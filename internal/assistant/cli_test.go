package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/mcp"
)

// fakeCLI is what the test program prints when it stands in for claude or codex. The tests never
// run the real tools.
func fakeCLI() {
	pid := func() {
		if f := os.Getenv("XEGESIS_FAKE_PIDS"); f != "" {
			old, _ := os.ReadFile(f)
			_ = os.WriteFile(f, append(old, []byte(strconv.Itoa(os.Getpid())+"\n")...), 0o600)
		}
	}
	switch os.Getenv("XEGESIS_FAKE_CLI") {
	case "claude":
		pid()
		in := new(bytes.Buffer)
		_, _ = in.ReadFrom(os.Stdin)
		for _, l := range []string{
			`{"type":"system","subtype":"init","session_id":"s1"}`,
			`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Let me look. "}}}`,
			`{"type":"assistant","message":{"content":[{"type":"text","text":"Let me look. "},{"type":"tool_use","id":"t1","name":"mcp__xegesis__read_passage","input":{"ref":"Romans 8","bible":"KJV"}}]}}`,
			`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"For to be carnally minded"}]}]}}`,
			`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"See Romans 8:6."}}}`,
			`{"type":"result","subtype":"success","is_error":false,"result":"Let me look. See Romans 8:6."}`,
		} {
			fmt.Println(l)
		}
		fmt.Fprintln(os.Stderr, "prompt had", strings.Count(in.String(), "Romans"), "mentions")
	case "hang":
		// Starts a child that outlives it, as a launcher script does, then waits.
		pid()
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "XEGESIS_FAKE_CLI=sleep")
		_ = child.Start()
		fmt.Println(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Thinking"}}}`)
		time.Sleep(time.Minute)
	case "sleep":
		pid()
		time.Sleep(time.Minute)
	case "fail":
		fmt.Println(`{"type":"result","subtype":"success","is_error":true,"result":"Not logged in. Run claude login."}`)
		os.Exit(1)
	case "version":
		fmt.Println("9.9.9 (Fake Tool)")
	}
}

type got struct {
	text  strings.Builder
	start []Line
	done  []Line
}

func (g *got) hooks() AgentHooks {
	return AgentHooks{Text: func(s string) { g.text.WriteString(s) }, Tool: func(l Line) { g.start = append(g.start, l) }, ToolDone: func(l Line) { g.done = append(g.done, l) }}
}

func fakeAgent(id string) *CLIAgent {
	return &CLIAgent{Info: CLIInfo{ID: id, Label: "Fake", Found: true, Path: os.Args[0]}, Exe: "xegesis", Library: "library.db"}
}

func TestClaudeStreamBecomesTextAndToolLines(t *testing.T) {
	t.Setenv("XEGESIS_FAKE_CLI", "claude")
	var g got
	if err := fakeAgent("claude-code").Run(context.Background(), AgentRequest{Prompt: "Romans?"}, g.hooks()); err != nil {
		t.Fatal(err)
	}
	if g.text.String() != "Let me look. \n\nSee Romans 8:6." {
		t.Fatalf("text %q", g.text.String())
	}
	if len(g.start) != 1 || len(g.done) != 1 {
		t.Fatalf("tool lines %v %v", g.start, g.done)
	}
	l := g.done[0]
	if l.Name != "read_passage" || l.Label != "Read Romans 8 (KJV)" || l.Content != "For to be carnally minded" {
		t.Fatalf("%+v", l)
	}
}

func TestCodexLinesParse(t *testing.T) {
	var g got
	p := &streamParser{agent: "codex", h: g.hooks(), names: map[string]*Line{}}
	for _, l := range []string{
		`{"type":"thread.started","thread_id":"x"}`,
		`{"type":"item.started","item":{"id":"i1","type":"mcp_tool_call","server":"xegesis","tool":"look_up_word","arguments":{"word":"G26"},"status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"i1","type":"mcp_tool_call","server":"xegesis","tool":"look_up_word","arguments":{"word":"G26"},"result":{"content":[{"type":"text","text":"agape"}]},"status":"completed"}}`,
		`{"type":"item.completed","item":{"id":"i2","type":"agent_message","text":"Love."}}`,
		`{"type":"turn.completed","usage":{}}`,
	} {
		p.line([]byte(l))
	}
	if g.text.String() != "Love." || len(g.done) != 1 || g.done[0].Label != "Looked up G26" || g.done[0].Content != "agape" || !p.done {
		t.Fatalf("%q %+v", g.text.String(), g.done)
	}
	p = &streamParser{agent: "codex", h: g.hooks(), names: map[string]*Line{}}
	p.line([]byte(`{"type":"turn.failed","error":{"message":"not signed in"}}`))
	if p.err != "not signed in" {
		t.Fatal(p.err)
	}
}

func TestToolErrorsComeBackInPlainWords(t *testing.T) {
	t.Setenv("XEGESIS_FAKE_CLI", "fail")
	err := fakeAgent("claude-code").Run(context.Background(), AgentRequest{Prompt: "x"}, AgentHooks{})
	if err == nil || !strings.Contains(err.Error(), "Not logged in") {
		t.Fatal(err)
	}
}

func TestStopKillsTheWholeProcessTree(t *testing.T) {
	pids := t.TempDir() + "/pids"
	t.Setenv("XEGESIS_FAKE_CLI", "hang")
	t.Setenv("XEGESIS_FAKE_PIDS", pids)
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var g got
	go func() { done <- fakeAgent("claude-code").Run(ctx, AgentRequest{Prompt: "x"}, g.hooks()) }()
	var list []int
	for end := time.Now().Add(20 * time.Second); time.Now().Before(end) && len(list) < 2; time.Sleep(50 * time.Millisecond) {
		b, _ := os.ReadFile(pids)
		list = list[:0]
		for _, f := range strings.Fields(string(b)) {
			n, _ := strconv.Atoi(f)
			list = append(list, n)
		}
	}
	if len(list) < 2 {
		t.Fatalf("the tool and its child did not start: %v", list)
	}
	stop()
	select {
	case err := <-done:
		if err == nil || ctx.Err() == nil {
			t.Fatal("expected the run to end by cancel, got", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Stop did not end the run")
	}
	for _, p := range list {
		for end := time.Now().Add(5 * time.Second); alive(p) && time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		}
		if alive(p) {
			t.Fatalf("process %d is still running after Stop", p)
		}
	}
}

func TestDetectFindsToolOnPath(t *testing.T) {
	info := Detect(context.Background(), Preset{ID: "codex", Label: "Codex", CLI: true})
	if info.Found && info.Path == "" {
		t.Fatal("found without a path")
	}
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	t.Setenv("Path", empty)
	if info := Detect(context.Background(), Preset{ID: "claude-code", Label: "Claude Code", CLI: true}); info.Found || info.InstallURL == "" {
		t.Fatalf("%+v", info)
	}
}

func TestCommandLinesHoldNoPromptOrQuote(t *testing.T) {
	for _, id := range []string{"claude-code", "codex"} {
		a := fakeAgent(id)
		a.Exe = `C:\Program Files\Xegesis\xegesis.exe`
		args := strings.Join(a.Args("cfg.json", "m1"), " ")
		if strings.ContainsAny(args, "\"\n") {
			t.Fatalf("%s: %q", id, args)
		}
		if strings.Contains(args, "Bash") && id == "codex" {
			t.Fatal("codex args name claude tools")
		}
	}
}

func TestMCPAssistantModeOffersOnlyTheReadOnlyTools(t *testing.T) {
	g := newRig(t, nil)
	d := &Deps{Run: RegistryRunner(g.reg, g.env), Gate: Gate{Local: false}}
	s := &mcp.Server{Name: "t", Only: MCPTools(d)}
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read_book","arguments":{"item_id":"own"}}}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"note_create","arguments":{"title":"x"}}}` + "\n" +
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_bibles","arguments":{}}}` + "\n")
	var out bytes.Buffer
	if err := s.Serve(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var list struct {
		Result struct{ Tools []struct{ Name string } }
	}
	_ = json.Unmarshal([]byte(lines[0]), &list)
	var names []string
	for _, tl := range list.Result.Tools {
		names = append(names, tl.Name)
	}
	var want []string
	for _, tl := range Tools() {
		want = append(want, tl.Def.Name)
	}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tools %v, want exactly %v", names, want)
	}
	if !strings.Contains(lines[1], "personal copy") && !strings.Contains(lines[1], "Not sent") {
		t.Fatalf("a personal-copy book reached an online reader: %s", lines[1])
	}
	if strings.Contains(lines[1], "Chapter") {
		t.Fatalf("book text leaked: %s", lines[1])
	}
	if !strings.Contains(lines[2], "error") || strings.Contains(lines[2], `"result":{"content":[{"type":"text","text":"{`) {
		t.Fatalf("a write command was reachable: %s", lines[2])
	}
	if !strings.Contains(lines[3], "KJV") {
		t.Fatalf("list_bibles: %s", lines[3])
	}
}

func TestChatThroughACommandLineTool(t *testing.T) {
	t.Setenv("XEGESIS_FAKE_CLI", "claude")
	detect := func(_ context.Context, p Preset) CLIInfo {
		if p.ID == "codex" {
			return CLIInfo{ID: p.ID, Label: p.Label, InstallURL: "https://example.test/codex"}
		}
		return CLIInfo{ID: p.ID, Label: p.Label, Found: true, Path: os.Args[0], Version: "9.9.9", InstallURL: "https://example.test/c"}
	}
	g := newRig(t, nil, WithDetect(detect))
	var infos []CLIInfo
	g.must("assistant.cli.detect", struct{}{}, &infos)
	if len(infos) != 2 || !infos[0].Found || infos[1].Found {
		t.Fatalf("%+v", infos)
	}
	g.must("assistant.config.set", map[string]any{"provider": "codex"}, nil)
	if err := g.run("assistant.chat", map[string]any{"message": "hi"}, nil); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("a missing tool must say so: %v", err)
	}
	var c Config
	g.must("assistant.config.set", map[string]any{"provider": "claude-code"}, &c)
	if c.Local || c.Confirmed || c.NeedsKey {
		t.Fatalf("a command-line tool is online and unconfirmed at first: %+v", c)
	}
	if err := g.run("assistant.chat", map[string]any{"message": "hi"}, nil); err == nil || !strings.Contains(err.Error(), "confirm") {
		t.Fatalf("the privacy panel comes first: %v", err)
	}
	var pv Preview
	g.must("assistant.preview", map[string]any{"message": "Romans 8?"}, &pv)
	if !strings.Contains(pv.Endpoint, "signs in and connects") || !strings.Contains(pv.Body, "Romans 8?") || pv.Local {
		t.Fatalf("%+v", pv)
	}
	g.must("assistant.config.set", map[string]any{"confirm": true}, nil)
	var test TestResult
	g.must("assistant.test", struct{}{}, &test)
	if !test.OK || !strings.Contains(test.Message, "9.9.9") {
		t.Fatalf("%+v", test)
	}
	var res ChatResult
	g.must("assistant.chat", map[string]any{"message": "What is Romans 8?"}, &res)
	if res.Reply != "Let me look. \n\nSee Romans 8:6." || len(res.Tools) != 1 || res.Tools[0].Label != "Read Romans 8 (KJV)" {
		t.Fatalf("%+v", res)
	}
	var types []string
	g.mu.Lock()
	for _, e := range g.evs {
		types = append(types, e.Type)
	}
	g.mu.Unlock()
	if strings.Join(types, " ") != "start text tool tool_done text done" {
		t.Fatalf("events %v", types)
	}
	// The chat goes on; the second prompt carries the first exchange.
	g.must("assistant.chat", map[string]any{"conversation_id": res.Conversation, "message": "And verse 7?"}, &res)
}
