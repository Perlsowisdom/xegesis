package assistant

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Claude Code and Codex are command-line tools the person has already signed in to. Xegesis runs
// one non-interactively for each question and gives it its own MCP server in assistant mode, which
// offers only Tools, each result passing the licence gate as for an online model. The tool does its
// own sign-in and network calls (ADR 0011, docs/privacy.md).

// CLIInfo is a command-line tool found, or not, on this computer.
type CLIInfo struct {
	ID         string `json:"id" desc:"claude-code or codex"`
	Label      string `json:"label"`
	Found      bool   `json:"found"`
	Path       string `json:"path,omitempty"`
	Version    string `json:"version,omitempty"`
	InstallURL string `json:"install_url"`
}

type cliKind struct {
	exe, install string
}

var cliKinds = map[string]cliKind{
	"claude-code": {"claude", "https://docs.anthropic.com/en/docs/claude-code/setup"},
	"codex":       {"codex", "https://github.com/openai/codex#installing-and-running-codex-cli"},
}

// Detect looks for the tool of a CLI preset on PATH and asks it for its version.
func Detect(ctx context.Context, p Preset) CLIInfo {
	k := cliKinds[p.ID]
	info := CLIInfo{ID: p.ID, Label: p.Label, InstallURL: k.install}
	path, err := exec.LookPath(k.exe)
	if err != nil {
		return info
	}
	info.Found, info.Path = true, path
	ctx, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	cmd := exec.CommandContext(ctx, path, "--version")
	prepare(cmd)
	if out, err := cmd.Output(); err == nil {
		info.Version = firstLine(string(out))
	}
	return info
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

// AgentRequest is one question for a tool that runs its own loop.
type AgentRequest struct {
	Model  string
	Prompt string // sent to the tool on standard input
}

// AgentHooks receive a tool's progress as it streams. Tool is called when a call starts and
// ToolDone when its result is in, both with the same ID.
type AgentHooks struct {
	Text     func(string)
	Tool     func(Line)
	ToolDone func(Line)
}

// Agent is a provider that runs the whole tool loop itself (Claude Code, Codex). Cancelling ctx
// stops it and everything it started.
type Agent interface {
	Run(ctx context.Context, req AgentRequest, h AgentHooks) error
}

// CLIAgent runs a detected command-line tool.
type CLIAgent struct {
	Info    CLIInfo
	Exe     string // the program that serves "mcp-assistant" (the Xegesis app or CLI)
	Library string // library.db path
}

// Stream and Models exist only so a CLIAgent can stand where a Provider is kept; the chat uses Run.
func (a *CLIAgent) Stream(context.Context, Request, func(string)) (Turn, error) {
	return Turn{}, &Error{Kind: "failed", Message: a.Info.Label + " runs its own conversation."}
}
func (a *CLIAgent) Models(context.Context) ([]string, error) { return nil, nil }

// Args are the tool's command-line arguments for one question; mcpConfig is the MCP config file.
// The prompt itself goes on standard input, so no newline or quote of it reaches a command line.
func (a *CLIAgent) Args(mcpConfig, model string) []string {
	switch a.Info.ID {
	case "codex":
		args := []string{"exec", "--json", "--skip-git-repo-check", "--sandbox", "read-only",
			"-c", "mcp_servers.xegesis.command=" + tomlStr(a.Exe),
			"-c", "mcp_servers.xegesis.args=" + tomlList([]string{"-library", a.Library, "mcp-assistant"})}
		if model != "" {
			args = append(args, "-m", model)
		}
		return append(args, "-")
	default:
		args := []string{"-p", "--output-format", "stream-json", "--verbose", "--include-partial-messages",
			"--mcp-config", mcpConfig, "--strict-mcp-config",
			"--allowedTools", "mcp__xegesis",
			"--disallowedTools", "Bash,Edit,Write,MultiEdit,NotebookEdit,Read,Glob,Grep,WebFetch,WebSearch,Task"}
		if model != "" {
			args = append(args, "--model", model)
		}
		return args
	}
}

// Command is the command line shown in the privacy panel.
func (a *CLIAgent) Command(model string) string {
	return a.Info.Path + " " + strings.Join(a.Args("<mcp-config.json>", model), " ")
}

// tomlStr and tomlList write TOML literal strings (single quotes), so no double quote reaches a
// Windows command line.
func tomlStr(s string) string { return "'" + s + "'" }
func tomlList(l []string) string {
	q := make([]string, len(l))
	for i, s := range l {
		q[i] = tomlStr(s)
	}
	return "[" + strings.Join(q, ",") + "]"
}

// Run starts the tool, sends the prompt on standard input and streams what it prints.
func (a *CLIAgent) Run(ctx context.Context, req AgentRequest, h AgentHooks) error {
	dir, err := os.MkdirTemp("", "xegesis-assistant-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cfg := filepath.Join(dir, "mcp.json")
	conf, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{
		"xegesis": map[string]any{"command": a.Exe, "args": []string{"-library", a.Library, "mcp-assistant"}}}})
	if err := os.WriteFile(cfg, conf, 0o600); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, a.Info.Path, a.Args(cfg, req.Model)...)
	cmd.Dir = dir // not the app's folder, so the tool does not read whatever is there
	cmd.Stdin = strings.NewReader(req.Prompt)
	var stderr bytes.Buffer
	cmd.Stderr = &limitWriter{w: &stderr, n: 4000}
	prepare(cmd)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return &Error{Kind: "failed", Message: fmt.Sprintf("Could not start %s: %v", a.Info.Label, err)}
	}
	p := &streamParser{agent: a.Info.ID, h: h, names: map[string]*Line{}}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		p.line(sc.Bytes())
	}
	_, _ = io.Copy(io.Discard, out)
	werr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if p.err != "" {
		return &Error{Kind: "failed", Message: p.err}
	}
	if werr != nil && !p.done {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = werr.Error()
		}
		return &Error{Kind: "failed", Message: fmt.Sprintf("%s stopped: %s", a.Info.Label, firstLine(msg))}
	}
	return nil
}

type limitWriter struct {
	w io.Writer
	n int
}

func (l *limitWriter) Write(b []byte) (int, error) {
	if l.n > 0 {
		c := min(len(b), l.n)
		_, _ = l.w.Write(b[:c])
		l.n -= c
	}
	return len(b), nil
}

// streamParser turns a tool's JSON lines into hooks. Both tools print one JSON object per line.
type streamParser struct {
	agent   string
	h       AgentHooks
	names   map[string]*Line
	partial bool // text arrives as deltas, so whole messages repeat it
	wrote   bool
	gap     bool // a tool call came since the last text
	done    bool
	err     string
}

func (p *streamParser) text(t string) {
	if t == "" {
		return
	}
	if p.gap && p.wrote {
		t = "\n\n" + t
	}
	p.gap, p.wrote = false, true
	if p.h.Text != nil {
		p.h.Text(t)
	}
}

func (p *streamParser) start(id, name string, args json.RawMessage) {
	name = strings.TrimPrefix(name, "mcp__xegesis__")
	a := string(args)
	if a == "" || a == "null" {
		a = "{}"
	}
	l := &Line{ID: id, Name: name, Args: a, Label: agentLabel(name, a)}
	p.names[id] = l
	p.gap = true
	if p.h.Tool != nil {
		p.h.Tool(*l)
	}
}

func (p *streamParser) finish(id, content string) {
	l, ok := p.names[id]
	if !ok {
		return
	}
	delete(p.names, id)
	l.Content = content
	if p.h.ToolDone != nil {
		p.h.ToolDone(*l)
	}
}

type content struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

// blocks reads MCP-style content: a string, or a list of text blocks.
func blocks(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var l []content
	if json.Unmarshal(raw, &l) != nil {
		return ""
	}
	var parts []string
	for _, c := range l {
		if c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

type itemError struct {
	Message string `json:"message"`
}

// event is a line of either tool's output; each tool fills the fields it prints.
type event struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
	Message struct {
		Content []content `json:"content"`
	} `json:"message"`
	Event struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	} `json:"event"`
	Item struct {
		ID        string          `json:"id"`
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
		Result    struct {
			Content json.RawMessage `json:"content"`
		} `json:"result"`
		Error *itemError `json:"error"`
	} `json:"item"`
	Error json.RawMessage `json:"error"`
}

func (p *streamParser) line(b []byte) {
	var m event
	if json.Unmarshal(b, &m) != nil {
		return
	}
	if p.agent == "codex" {
		p.codex(m)
		return
	}
	switch m.Type {
	case "stream_event":
		if m.Event.Type == "content_block_delta" && m.Event.Delta.Type == "text_delta" {
			p.partial = true
			p.text(m.Event.Delta.Text)
		}
	case "assistant":
		for _, c := range m.Message.Content {
			switch c.Type {
			case "text":
				if !p.partial {
					p.text(c.Text)
				}
			case "tool_use":
				p.start(c.ID, c.Name, c.Input)
			}
		}
	case "user":
		for _, c := range m.Message.Content {
			if c.Type == "tool_result" {
				p.finish(c.ToolUseID, blocks(c.Content))
			}
		}
	case "result":
		p.done = true
		if m.IsError || strings.HasPrefix(m.Subtype, "error") {
			p.err = m.Result
			if p.err == "" {
				p.err = "The tool reported an error."
			}
		} else if !p.wrote {
			p.text(m.Result)
		}
	}
}

func (p *streamParser) codex(m event) {
	it := m.Item
	switch m.Type {
	case "item.started":
		if it.Type == "mcp_tool_call" {
			p.start(it.ID, it.Tool, codexArgs(it.Arguments))
		}
	case "item.completed":
		switch it.Type {
		case "agent_message":
			p.gap = true // each message is a paragraph of its own
			p.text(it.Text)
		case "mcp_tool_call":
			c := blocks(it.Result.Content)
			if it.Error != nil {
				c = "Error: " + it.Error.Message
			}
			if _, started := p.names[it.ID]; !started {
				p.start(it.ID, it.Tool, codexArgs(it.Arguments))
			}
			p.finish(it.ID, c)
		}
	case "turn.completed":
		p.done = true
	case "turn.failed", "error":
		var e itemError
		if json.Unmarshal(m.Error, &e) == nil {
			p.err = e.Message
		}
		if p.err == "" {
			p.err = "The tool reported an error."
		}
	}
}

// codexArgs accepts arguments given as an object or as a string holding one.
func codexArgs(raw json.RawMessage) json.RawMessage {
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return json.RawMessage(s)
	}
	return raw
}

// agentLabel is a tool call in words, from its name and arguments.
func agentLabel(name, args string) string {
	var a struct {
		Ref   string `json:"ref"`
		Bible string `json:"bible"`
		Query string `json:"query"`
		Word  string `json:"word"`
	}
	_ = json.Unmarshal([]byte(args), &a)
	switch {
	case name == "read_passage" && a.Ref != "":
		if a.Bible != "" {
			return fmt.Sprintf("Read %s (%s)", a.Ref, a.Bible)
		}
		return "Read " + a.Ref
	case name == "search_library" && a.Query != "":
		return fmt.Sprintf("Searched the library for %q", a.Query)
	case name == "passage_report" && a.Ref != "":
		return "Gathered a report on " + a.Ref
	case (name == "word_report" || name == "look_up_word") && a.Word != "":
		return "Looked up " + a.Word
	}
	return plainName(name)
}

// agentPrompt is what a tool is sent: Xegesis's instructions, the chat so far, and the question.
func agentPrompt(history []Message, question string) string {
	var b strings.Builder
	b.WriteString(SystemPrompt + "\n\n")
	if len(history) > 0 {
		b.WriteString("The conversation so far:\n\n")
		for _, m := range history {
			if m.Role == "user" {
				b.WriteString("User: " + m.Content + "\n\n")
			} else if m.Role == "assistant" && m.Content != "" {
				b.WriteString("Assistant: " + m.Content + "\n\n")
			}
		}
	}
	return b.String() + "The person's new question:\n\n" + question + "\n"
}
