package assistant

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/integrations/secrets"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

type rig struct {
	t    *testing.T
	reg  *command.Registry
	env  *command.Env
	keys *secrets.Memory
	fake *fakeServer
	mu   sync.Mutex
	evs  []Event
	ctx  context.Context
}

// fakeLibrary stands in for the Bible and library commands: a free Bible, a limited one, and
// two books, one a personal copy.
func fakeLibrary(t *testing.T, r *command.Registry) {
	free, _ := licence.Defaults(licence.PublicDomain, "")
	limited, _ := licence.Defaults(licence.PersonalPurchase, "")
	mods := []moduleMeta{{ID: "KJV", Kind: "bible", Abbreviation: "KJV", Licence: free}, {ID: "NETfree", Kind: "bible", Abbreviation: "NETfree", Licence: limited}}
	type none struct {
		Kind string `json:"kind,omitempty"`
	}
	r.Add(
		command.Define("bible.modules", "test", false, func(_ context.Context, _ *command.Env, _ none) ([]moduleMeta, error) { return mods, nil }),
		command.Define("bible.read", "test", false, func(_ context.Context, _ *command.Env, p struct {
			Module string `json:"module"`
			Ref    string `json:"ref"`
		}) (map[string]any, error) {
			var vs []map[string]any
			for i := 1; i <= 39; i++ {
				vs = append(vs, map[string]any{"id": 45008000 + i, "text": strings.Repeat("Spirit life ", 12)})
			}
			return map[string]any{"ref": "Romans 8", "verses": vs}, nil
		}),
		command.Define("library.item", "test", false, func(_ context.Context, _ *command.Env, p struct {
			ID string `json:"id"`
		}) (map[string]any, error) {
			if p.ID == "own" {
				return map[string]any{"title": "Bought Commentary", "licence": nil}, nil
			}
			return map[string]any{"title": "Open Book", "licence": free}, nil
		}),
		command.Define("book.section", "test", false, func(_ context.Context, _ *command.Env, p struct {
			ItemID  string `json:"item_id"`
			Section int    `json:"section"`
		}) (map[string]any, error) {
			return map[string]any{"sections": 3, "html": "<p>Chapter <b>one</b> text.</p>"}, nil
		}),
	)
}

func newRig(t *testing.T, reply func(map[string]any) step) *rig {
	st, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	g := &rig{t: t, reg: command.NewRegistry(), env: &command.Env{Store: st}, keys: secrets.NewMemory(), fake: newFake(t, reply)}
	core.Register(g.reg)
	fakeLibrary(t, g.reg)
	Register(g.reg, g.keys, WithProvider(func(addr, key string, p Preset) Provider {
		return &OpenAI{BaseURL: addr, Key: key, Client: g.fake.Client()}
	}))
	g.ctx = WithEmitter(context.Background(), func(name string, data any) {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.evs = append(g.evs, data.(Event))
	})
	return g
}

func (g *rig) run(id string, params any, out any) error {
	raw, _ := json.Marshal(params)
	res, err := g.reg.Run(g.ctx, g.env, id, raw)
	if err != nil || out == nil {
		return err
	}
	b, _ := json.Marshal(res)
	return json.Unmarshal(b, out)
}

func (g *rig) must(id string, params any, out any) {
	g.t.Helper()
	if err := g.run(id, params, out); err != nil {
		g.t.Fatalf("%s: %v", id, err)
	}
}

// setUp chooses the OpenAI preset pointed at the fake server, with a key, a model, and confirms.
func (g *rig) setUp(provider string) {
	g.must("assistant.config.set", map[string]any{"provider": provider, "base_url": g.fake.URL + "/v1", "key": "sk-secret-123", "model": "model-a", "confirm": true}, nil)
}

func TestConfigKeepsKeyInKeychainOnly(t *testing.T) {
	g := newRig(t, nil)
	var c Config
	g.must("assistant.config.get", nil, &c)
	if c.Provider != "off" || len(c.Presets) == 0 {
		t.Fatalf("%+v", c)
	}
	g.setUp("openai")
	g.must("assistant.config.get", nil, &c)
	if !c.KeySaved || c.Model != "model-a" || c.Local || !c.Confirmed {
		t.Fatalf("%+v", c)
	}
	if v, _ := g.keys.Get("assistant.openai.key"); v != "sk-secret-123" {
		t.Fatal("key not in the keychain")
	}
	// Nothing in library.db holds the key, the change history included.
	for _, q := range []string{`SELECT group_concat(value_json) FROM settings`, `SELECT group_concat(CAST(rowid AS TEXT) || COALESCE(CAST(after_json AS TEXT), '')) FROM changes`} {
		var s *string
		if err := g.env.Store.QueryRow(context.Background(), q).Scan(&s); err != nil && !strings.Contains(err.Error(), "no such column") {
			t.Fatal(err)
		}
		if s != nil && strings.Contains(*s, "sk-secret") {
			t.Fatalf("key stored in library.db: %s", q)
		}
	}
	var ms []string
	g.must("assistant.models", nil, &ms)
	if len(ms) != 2 {
		t.Fatal(ms)
	}
	var tr TestResult
	g.must("assistant.test", nil, &tr)
	if !tr.OK {
		t.Fatal(tr.Message)
	}
	g.must("assistant.config.set", map[string]any{"clear_key": true}, &c)
	if c.KeySaved {
		t.Fatal("key not cleared")
	}
	g.must("assistant.test", nil, &tr)
	if tr.OK || !strings.Contains(tr.Message, "key") {
		t.Fatalf("%+v", tr)
	}
}

func TestChatNeedsChoiceAndConfirmation(t *testing.T) {
	g := newRig(t, func(map[string]any) step { return step{text: []string{"hi"}} })
	if err := g.run("assistant.chat", map[string]any{"message": "hi"}, nil); err == nil || !strings.Contains(err.Error(), "off") {
		t.Fatalf("off: %v", err)
	}
	g.must("assistant.config.set", map[string]any{"provider": "openai", "base_url": g.fake.URL + "/v1", "key": "k", "model": "m"}, nil)
	if err := g.run("assistant.chat", map[string]any{"message": "hi"}, nil); err == nil || !strings.Contains(err.Error(), "confirm") {
		t.Fatalf("unconfirmed: %v", err)
	}
	var pv Preview
	g.must("assistant.preview", map[string]any{"message": "What is grace?", "context": map[string]any{"ref": "Romans 8", "bible": "KJV"}}, &pv)
	if !contains(pv.Body, "What is grace?", "Romans 8", "read_passage") || pv.Model != "m" || !strings.HasSuffix(pv.Endpoint, "/v1/chat/completions") || strings.Contains(pv.Body, `"k"`) {
		t.Fatalf("%+v", pv)
	}
	if len(g.fake.requests()) != 0 {
		t.Fatal("something was sent before confirming")
	}
	// A new address asks again.
	g.must("assistant.config.set", map[string]any{"confirm": true}, nil)
	g.must("assistant.config.set", map[string]any{"base_url": g.fake.URL + "/v1/"}, nil)
	var c Config
	g.must("assistant.config.get", nil, &c)
	if c.Confirmed {
		t.Fatal("a changed address keeps its confirmation")
	}
}

// script answers each request in turn: first a tool call, then text that quotes the tool result.
func script(calls ...[2]string) func(map[string]any) step {
	return func(body map[string]any) step {
		role, content := lastMessage(body)
		if role == "tool" {
			return step{text: []string{"From the text: ", "Romans 8:28 says much. ", content[:min(40, len(content))]}}
		}
		return step{calls: calls}
	}
}

func TestChatStreamsWithToolsThroughTheGate(t *testing.T) {
	g := newRig(t, script([2]string{"read_passage", `{"ref":"Romans 8","bible":"NETfree"}`}))
	g.setUp("openai")
	var res ChatResult
	g.must("assistant.chat", map[string]any{"message": "Summarise Romans 8", "context": map[string]any{"ref": "Romans 8", "note_id": "n1", "note_title": "Grace"}}, &res)
	if !strings.HasPrefix(res.Reply, "From the text: Romans 8:28") || len(res.Tools) != 1 {
		t.Fatalf("%+v", res)
	}
	line := res.Tools[0]
	if line.Label != "Read Romans 8 (NETfree, shortened to the licence limit)" || !strings.Contains(line.Content, "Shortened to about 2000") {
		t.Fatalf("%+v", line)
	}
	reqs := g.fake.requests()
	if len(reqs) != 2 {
		t.Fatalf("%d requests", len(reqs))
	}
	_, sent := lastMessage(reqs[1])
	if sent != line.Content || len([]rune(sent)) > 2300 {
		t.Fatalf("the model got something other than the line shows (%d chars)", len(sent))
	}
	msgs := reqs[0]["messages"].([]any)
	if q := msgs[1].(map[string]any)["content"].(string); !contains(q, "passage Romans 8", `note "Grace" (id n1)`, "Summarise Romans 8") {
		t.Fatalf("context not sent: %q", q)
	}
	types := []string{}
	for _, e := range g.evs {
		types = append(types, e.Type)
	}
	if got := strings.Join(types, ","); got != "start,tool,tool_done,text,text,text,done" {
		t.Fatalf("events %s", got)
	}
	// The chat continues, and can be kept as a note.
	g.must("assistant.chat", map[string]any{"conversation_id": res.Conversation, "message": "And verse 28?"}, &res)
	if n := len(g.fake.requests()[2]["messages"].([]any)); n < 6 {
		t.Fatalf("history not sent: %d messages", n)
	}
	var note struct {
		ID, Title, Body string
	}
	g.must("assistant.save", map[string]any{"conversation_id": res.Conversation}, &note)
	if note.Title != "Summarise Romans 8" || !contains(note.Body, "## Summarise Romans 8", "- *Read Romans 8", "## And verse 28?") || strings.Contains(note.Body, "Shortened to about") {
		t.Fatalf("%+v", note)
	}
}

func TestPersonalCopyRefusedOnlineSentLocally(t *testing.T) {
	g := newRig(t, script([2]string{"read_book", `{"item_id":"own"}`}))
	g.setUp("openai")
	var res ChatResult
	g.must("assistant.chat", map[string]any{"message": "Read my commentary"}, &res)
	if l := res.Tools[0]; !strings.Contains(l.Label, "not sent: a personal copy") || strings.Contains(l.Content, "Chapter one") {
		t.Fatalf("online: %+v", l)
	}
	// The same book goes to a model on this computer.
	g.must("assistant.config.set", map[string]any{"provider": "custom", "base_url": g.fake.URL + "/v1", "model": "model-a", "confirm": true}, nil)
	var c Config
	g.must("assistant.config.get", nil, &c)
	if !c.Local {
		t.Fatal("a custom address on this computer is local")
	}
	g.must("assistant.chat", map[string]any{"message": "Read my commentary"}, &res)
	if l := res.Tools[0]; !strings.Contains(l.Content, "Chapter one text.") || strings.Contains(l.Label, "not sent") {
		t.Fatalf("local: %+v", l)
	}
}

func TestStopEndsTheReply(t *testing.T) {
	g := newRig(t, func(map[string]any) step { return step{text: []string{"Begin", " never"}} })
	g.fake.block = make(chan struct{})
	defer close(g.fake.block)
	g.setUp("openai")
	done := make(chan ChatResult, 1)
	go func() {
		var r ChatResult
		_ = g.run("assistant.chat", map[string]any{"message": "Long answer"}, &r)
		done <- r
	}()
	var id string
	for start := time.Now(); id == "" && time.Since(start) < 5*time.Second; time.Sleep(10 * time.Millisecond) {
		g.mu.Lock()
		for _, e := range g.evs {
			if e.Type == "text" {
				id = e.Conversation
			}
		}
		g.mu.Unlock()
	}
	var stopped map[string]bool
	g.must("assistant.cancel", map[string]any{"conversation_id": id}, &stopped)
	select {
	case r := <-done:
		if !r.Stopped || r.Reply != "Begin" || !stopped["stopped"] {
			t.Fatalf("%+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not end the reply")
	}
}

func TestToolsAreReadOnly(t *testing.T) {
	names := map[string]bool{}
	for _, tl := range Tools() {
		if names[tl.Def.Name] {
			t.Fatal("duplicate tool", tl.Def.Name)
		}
		names[tl.Def.Name] = true
		for _, w := range []string{"create", "update", "delete", "set", "write", "save"} {
			if strings.Contains(tl.Def.Name, w) {
				t.Fatalf("%s looks like a write", tl.Def.Name)
			}
		}
	}
	// Every command a tool runs is a read.
	g := newRig(t, nil)
	var ran []string
	d := &Deps{Gate: Gate{}, Run: func(ctx context.Context, id string, p, out any) error {
		ran = append(ran, id)
		return RegistryRunner(g.reg, g.env)(ctx, id, p, out)
	}}
	for _, tl := range Tools() {
		RunTool(context.Background(), d, tl.Def.Name, `{"ref":"Romans 8","query":"grace","id":"x","item_id":"own","word":"G26"}`)
	}
	for _, id := range ran {
		if desc, ok := g.reg.Describe(id); ok && desc.Writes {
			t.Fatalf("a tool runs %s, which writes", id)
		}
	}
}
