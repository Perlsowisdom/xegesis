package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

type client struct {
	in  io.WriteCloser
	out *bufio.Reader
}

func start(t *testing.T) *client {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	reg := command.NewRegistry()
	core.Register(reg)
	s := &Server{Reg: reg, Env: &command.Env{Store: st}, Name: "xegesis", Version: "test"}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	go func() {
		s.Serve(context.Background(), inR, outW)
		outW.Close()
	}()
	t.Cleanup(func() { inW.Close() })
	return &client{in: inW, out: bufio.NewReader(outR)}
}

func (c *client) send(t *testing.T, msg string) {
	t.Helper()
	if _, err := io.WriteString(c.in, msg+"\n"); err != nil {
		t.Fatal(err)
	}
}

func (c *client) recv(t *testing.T) map[string]any {
	t.Helper()
	line, err := c.out.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("%v: %s", err, line)
	}
	return m
}

func (c *client) rpc(t *testing.T, msg string) map[string]any {
	t.Helper()
	c.send(t, msg)
	return c.recv(t)
}

func errCode(m map[string]any) float64 {
	e, _ := m["error"].(map[string]any)
	c, _ := e["code"].(float64)
	return c
}

func TestInitializeAndNotifications(t *testing.T) {
	c := start(t)
	r := c.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	res := r["result"].(map[string]any)
	if res["protocolVersion"] != "2024-11-05" || res["capabilities"].(map[string]any)["tools"] == nil {
		t.Fatalf("initialize: %v", r)
	}
	r = c.rpc(t, `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	if r["result"].(map[string]any)["protocolVersion"] != versions[0] {
		t.Fatalf("negotiation: %v", r)
	}
	// A notification, even an unknown one, gets no reply: the next response is for the ping.
	c.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	c.send(t, `{"jsonrpc":"2.0","method":"nonsense"}`)
	r = c.rpc(t, `{"jsonrpc":"2.0","id":"p","method":"ping"}`)
	if r["id"] != "p" || r["error"] != nil {
		t.Fatalf("ping: %v", r)
	}
}

func TestToolsList(t *testing.T) {
	c := start(t)
	r := c.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var create, del map[string]any
	for _, x := range r["result"].(map[string]any)["tools"].([]any) {
		tl := x.(map[string]any)
		switch tl["name"] {
		case "note_create":
			create = tl
		case "note_delete":
			del = tl
		}
	}
	if create == nil || del == nil {
		t.Fatalf("missing tools: %v", r)
	}
	schema := create["inputSchema"].(map[string]any)
	props := schema["properties"].(map[string]any)
	if schema["type"] != "object" || props["body"].(map[string]any)["type"] != "string" || props["ref"] == nil {
		t.Fatalf("schema: %v", schema)
	}
	if !strings.Contains(create["description"].(string), "recorded in history") {
		t.Fatalf("description: %v", create["description"])
	}
	a := create["annotations"].(map[string]any)
	if a["readOnlyHint"] != false || a["destructiveHint"] != false {
		t.Fatalf("annotations: %v", a)
	}
	if del["annotations"].(map[string]any)["destructiveHint"] != true {
		t.Fatalf("delete annotations: %v", del["annotations"])
	}
}

func TestToolsCall(t *testing.T) {
	c := start(t)
	r := c.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"verse_parse","arguments":{"ref":"John 3:16"}}}`)
	res := r["result"].(map[string]any)
	sc, _ := res["structuredContent"].(map[string]any)
	if res["isError"] != false || sc["start"] != float64(43003016) {
		t.Fatalf("verse_parse: %v", r)
	}
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "43003016") {
		t.Fatalf("text content: %s", text)
	}

	r = c.rpc(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"verse_parse","arguments":{}}}`)
	res = r["result"].(map[string]any)
	if res["isError"] != true || !strings.Contains(res["content"].([]any)[0].(map[string]any)["text"].(string), "bad_params") {
		t.Fatalf("failing call: %v", r)
	}

	r = c.rpc(t, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"nope"}}`)
	if errCode(r) != codeInvalidParams {
		t.Fatalf("unknown tool: %v", r)
	}
}

func TestProtocolErrors(t *testing.T) {
	c := start(t)
	if r := c.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`); errCode(r) != codeMethodNotFound {
		t.Fatalf("unknown method: %v", r)
	}
	r := c.rpc(t, `{not json`)
	if errCode(r) != codeParse || r["id"] != nil {
		t.Fatalf("malformed: %v", r)
	}
	if r := c.rpc(t, `{"jsonrpc":"1.0","id":1,"method":"ping"}`); errCode(r) != codeInvalidRequest {
		t.Fatalf("bad version: %v", r)
	}
}

func TestToolName(t *testing.T) {
	if got := ToolName("note.create"); got != "note_create" {
		t.Fatal(got)
	}
}
