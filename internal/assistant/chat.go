package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Perlsowisdom/xegesis/internal/command"
)

// Event is one step of a reply, sent as the "assistant:event" event while assistant.chat runs.
type Event struct {
	Conversation string `json:"conversation_id"`
	Type         string `json:"type" desc:"start, text, tool, tool_done, done or error"`
	Text         string `json:"text,omitempty" desc:"text: the next piece of the reply"`
	Tool         *Line  `json:"tool,omitempty"`
	Error        string `json:"error,omitempty"`
}

// EventName is the name events go out under.
const EventName = "assistant:event"

type emitKey struct{}

// WithEmitter gives assistant.chat a way to send events, such as the desktop app's Wails events.
func WithEmitter(ctx context.Context, emit func(name string, data any)) context.Context {
	return context.WithValue(ctx, emitKey{}, emit)
}

func emitter(ctx context.Context) func(Event) {
	fn, _ := ctx.Value(emitKey{}).(func(string, any))
	return func(e Event) {
		if fn != nil {
			fn(EventName, e)
		}
	}
}

// Line is a tool call as the person sees it: a short line that unfolds to what was sent.
type Line struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Args    string `json:"args"`
	Label   string `json:"label"`
	Content string `json:"content,omitempty" desc:"Exactly what the model received"`
	Notice  string `json:"notice,omitempty"`
}

// Entry is one item of a conversation as shown: a question, a reply or a tool line.
type Entry struct {
	Role string `json:"role" desc:"user, assistant or tool"`
	Text string `json:"text,omitempty"`
	Tool *Line  `json:"tool,omitempty"`
}

type conversation struct {
	id       string
	provider string // provider|address it was started with
	mu       sync.Mutex
	running  context.CancelFunc
	messages []Message
	entries  []Entry
}

// Context is what is open as the question is asked.
type Context struct {
	Ref       string `json:"ref,omitempty" desc:"The passage open in the Reader, e.g. \"Romans 8\""`
	Bible     string `json:"bible,omitempty" desc:"The Bible it is open in (module ID)"`
	NoteID    string `json:"note_id,omitempty" desc:"The note or sermon open now"`
	NoteKind  string `json:"note_kind,omitempty" enum:"note,sermon"`
	NoteTitle string `json:"note_title,omitempty"`
}

func (c *Context) line() string {
	if c == nil {
		return ""
	}
	var parts []string
	if c.Ref != "" {
		p := "passage " + c.Ref
		if c.Bible != "" {
			p += " in " + c.Bible
		}
		parts = append(parts, p)
	}
	if c.NoteID != "" {
		kind := c.NoteKind
		if kind == "" {
			kind = "note"
		}
		parts = append(parts, fmt.Sprintf("%s %q (id %s)", kind, c.NoteTitle, c.NoteID))
	}
	if len(parts) == 0 {
		return ""
	}
	return "[Open in Xegesis now: " + strings.Join(parts, "; ") + "]"
}

// SystemPrompt tells the model what it is and how to answer.
const SystemPrompt = `You are the study assistant in Xegesis, a Bible study app. You help the person study with the Bibles, books, notes, sermons and highlights in their own library, which you read with the tools. You cannot change anything in the library.

- Use the tools to look things up rather than relying on memory, and say which work a quotation comes from.
- Write Bible references in full, such as Romans 8:28 or John 3:16-18, so the app can link them.
- When you write something for the person to keep, such as a note, an outline or a sermon section, put it in a fenced block that starts with ` + "```draft" + ` and ends with ` + "```" + `, in Markdown. The person inserts it themselves.
- Some tool results are shortened or withheld because of a work's licence. Say so when it matters and do not fill in the missing text.
- Be accurate and brief.`

// maxRounds bounds how many times one question may call tools.
const maxRounds = 8

type ChatParams struct {
	Conversation string   `json:"conversation_id,omitempty" desc:"Continue this chat; a new one when omitted"`
	Message      string   `json:"message" required:"true" desc:"The question"`
	Context      *Context `json:"context,omitempty"`
}

type ChatResult struct {
	Conversation string  `json:"conversation_id"`
	Reply        string  `json:"reply"`
	Tools        []*Line `json:"tools"`
	Stopped      bool    `json:"stopped,omitempty" desc:"The person pressed Stop"`
}

func (s *service) conv(id string) (*conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" {
		c := &conversation{id: newID()}
		s.convs[c.id] = c
		return c, nil
	}
	c, ok := s.convs[id]
	if !ok {
		return nil, command.Errorf("not_found", "no chat %q in this session", id)
	}
	return c, nil
}

func userText(p ChatParams) string {
	if l := p.Context.line(); l != "" {
		return l + "\n\n" + p.Message
	}
	return p.Message
}

func (s *service) chat(ctx context.Context, env *command.Env, p ChatParams) (ChatResult, error) {
	if strings.TrimSpace(p.Message) == "" {
		return ChatResult{}, command.Errorf("bad_params", "ask something")
	}
	cur, err := s.current(ctx, env)
	if err != nil {
		return ChatResult{}, err
	}
	if cur.model == "" {
		return ChatResult{}, command.Errorf("unavailable", "Choose a model in Settings → Assistant → Model.")
	}
	if !cur.conf.Confirmed {
		return ChatResult{}, command.Errorf("unavailable", "First confirm what is sent to %s (assistant.preview shows it; assistant.config.set with confirm agrees).", cur.preset.Label)
	}
	c, err := s.conv(p.Conversation)
	if err != nil {
		return ChatResult{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	pk := confirmKey(cur.preset.ID, cur.addr)
	if c.provider != "" && c.provider != pk {
		return ChatResult{}, command.Errorf("bad_params", "this chat was started with another provider; start a new chat")
	}
	c.provider = pk
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	s.mu.Lock()
	c.running = stop
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		c.running = nil
		s.mu.Unlock()
	}()

	emit := emitter(ctx)
	deps := &Deps{Run: RegistryRunner(s.reg, env), Gate: Gate{Local: cur.local}}
	if p.Context != nil {
		deps.Bible = p.Context.Bible
	}
	res := ChatResult{Conversation: c.id, Tools: []*Line{}}
	emit(Event{Conversation: c.id, Type: "start"})
	start := len(c.messages)
	question := Message{Role: "user", Content: userText(p)}
	c.messages = append(c.messages, question)
	c.entries = append(c.entries, Entry{Role: "user", Text: p.Message})
	var reply strings.Builder
	finish := func() {
		if t := reply.String(); t != "" {
			c.entries = append(c.entries, Entry{Role: "assistant", Text: t})
		}
		res.Reply = reply.String()
	}
	// stopped keeps the question and what was written so far, without half-finished tool calls,
	// so the chat can go on.
	stopped := func() (ChatResult, error) {
		finish()
		res.Stopped = true
		c.messages = append(c.messages[:start], question)
		if res.Reply != "" {
			c.messages = append(c.messages, Message{Role: "assistant", Content: res.Reply})
		}
		emit(Event{Conversation: c.id, Type: "done"})
		return res, nil
	}
	for round := 0; ; round++ {
		req := Request{Model: cur.model, Messages: append([]Message{{Role: "system", Content: SystemPrompt}}, c.messages...), Tools: ToolDefs()}
		if round == maxRounds {
			req.Tools = nil // answer with what has been read
		}
		turn, err := cur.p.Stream(ctx, req, func(t string) {
			reply.WriteString(t)
			emit(Event{Conversation: c.id, Type: "text", Text: t})
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return stopped()
			}
			finish()
			// The question leaves the history, so asking again does not send it twice.
			c.messages = c.messages[:start]
			perr := plainErr(err)
			emit(Event{Conversation: c.id, Type: "error", Error: perr.Error()})
			return res, perr
		}
		c.messages = append(c.messages, Message{Role: "assistant", Content: turn.Text, ToolCalls: turn.ToolCalls})
		if len(turn.ToolCalls) == 0 {
			break
		}
		if turn.Text != "" {
			reply.WriteString("\n\n")
		}
		for _, call := range turn.ToolCalls {
			line := &Line{ID: call.ID, Name: call.Name, Args: call.Args, Label: plainName(call.Name) + "…"}
			emit(Event{Conversation: c.id, Type: "tool", Tool: line})
			r := RunTool(ctx, deps, call.Name, call.Args)
			if ctx.Err() != nil {
				break
			}
			line.Label, line.Content, line.Notice = r.Label, r.Content, r.Notice
			res.Tools = append(res.Tools, line)
			c.entries = append(c.entries, Entry{Role: "tool", Tool: line})
			c.messages = append(c.messages, Message{Role: "tool", ToolCallID: call.ID, Content: r.Content})
			emit(Event{Conversation: c.id, Type: "tool_done", Tool: line})
		}
		if ctx.Err() != nil {
			return stopped()
		}
	}
	finish()
	emit(Event{Conversation: c.id, Type: "done"})
	return res, nil
}

type ConvParams struct {
	Conversation string `json:"conversation_id" required:"true"`
}

func (s *service) cancel(_ context.Context, _ *command.Env, p ConvParams) (map[string]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.convs[p.Conversation]
	if !ok {
		return nil, command.Errorf("not_found", "no chat %q in this session", p.Conversation)
	}
	stopped := c.running != nil
	if stopped {
		c.running()
	}
	return map[string]bool{"stopped": stopped}, nil
}

func (s *service) conversation(_ context.Context, _ *command.Env, p ConvParams) ([]Entry, error) {
	c, err := s.conv(p.Conversation)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Entry{}, c.entries...), nil
}

type SaveParams struct {
	Conversation string `json:"conversation_id" required:"true"`
	Title        string `json:"title,omitempty" desc:"The note's title; the first question when omitted"`
}

// Transcript is a conversation as Markdown: questions as headings, replies as text, tool calls
// as a list of what was read (without the text sent).
func Transcript(entries []Entry) string {
	var b strings.Builder
	for _, e := range entries {
		switch e.Role {
		case "user":
			fmt.Fprintf(&b, "## %s\n\n", strings.Join(strings.Fields(e.Text), " "))
		case "tool":
			fmt.Fprintf(&b, "- *%s*\n\n", e.Tool.Label)
		default:
			b.WriteString(strings.TrimSpace(e.Text) + "\n\n")
		}
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func (s *service) save(ctx context.Context, env *command.Env, p SaveParams) (json.RawMessage, error) {
	entries, err := s.conversation(ctx, env, ConvParams{p.Conversation})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, command.Errorf("bad_params", "the chat is empty")
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title, _ = Budget(strings.Join(strings.Fields(entries[0].Text), " "), 60)
	}
	raw, _ := json.Marshal(map[string]any{"title": title, "body": Transcript(entries)})
	res, err := s.reg.Run(ctx, env, "note.create", raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(res)
}

type PreviewParams struct {
	Message string   `json:"message" required:"true"`
	Context *Context `json:"context,omitempty"`
}

type Preview struct {
	Provider string   `json:"provider"`
	Endpoint string   `json:"endpoint" desc:"Where the request goes"`
	Model    string   `json:"model"`
	Local    bool     `json:"local"`
	Question string   `json:"question" desc:"The question as sent, with what is open"`
	Tools    []string `json:"tools" desc:"The read-only tools the model may call"`
	Body     string   `json:"body" desc:"The first request, exactly as sent, without the key"`
}

func (s *service) preview(ctx context.Context, env *command.Env, p PreviewParams) (Preview, error) {
	cur, err := s.current(ctx, env)
	if err != nil {
		return Preview{}, err
	}
	q := userText(ChatParams{Message: p.Message, Context: p.Context})
	req := Request{Model: cur.model, Messages: []Message{{Role: "system", Content: SystemPrompt}, {Role: "user", Content: q}}, Tools: ToolDefs()}
	body, _ := json.MarshalIndent(Body(req, true), "", "  ")
	out := Preview{Provider: cur.preset.Label, Endpoint: strings.TrimRight(cur.addr, "/") + "/chat/completions", Model: cur.model,
		Local: cur.local, Question: q, Body: string(body)}
	for _, t := range Tools() {
		out.Tools = append(out.Tools, t.Def.Name)
	}
	return out, nil
}
