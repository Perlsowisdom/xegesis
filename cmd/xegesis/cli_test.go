package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunKeyValue(t *testing.T) {
	lib := filepath.Join(t.TempDir(), "library.db")
	var out bytes.Buffer
	if err := run([]string{"-library", lib, "run", "note.create", "body=hello there", "ref=Ps 23"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"hello there"`) {
		t.Fatalf("note.create output: %s", out.String())
	}
	out.Reset()
	if err := run([]string{"-library", lib, "run", "verse.parse", "ref=John 3:16"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "43003016") {
		t.Fatalf("verse.parse output: %s", out.String())
	}
	if err := run([]string{"-library", lib, "run", "verse.parse", "a=1", "b"}, nil, &out); err == nil {
		t.Fatal("expected an error for a bare word")
	}
}

func TestKeyValueParams(t *testing.T) {
	b, err := keyValueParams([]string{"n=3", "ok=true", "s=abc", `a.b="x"`, "a.c=[1,2]"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	a, _ := got["a"].(map[string]any)
	if got["n"] != 3.0 || got["ok"] != true || got["s"] != "abc" || a["b"] != "x" || len(a["c"].([]any)) != 2 {
		t.Fatalf("params: %s", b)
	}
}

func TestDescribeAndHelpText(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-format", "text", "describe", "note.create"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"note.create", "body", "string", "Changes the library"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("describe text lacks %q:\n%s", want, out.String())
		}
	}
	out.Reset()
	if err := run([]string{"-format", "text", "describe", "verse.parse"}, nil, &out); err != nil || !strings.Contains(out.String(), "(required)") {
		t.Fatalf("verse.parse text: %v\n%s", err, out.String())
	}
	out.Reset()
	if err := run([]string{"-format", "text", "help"}, nil, &out); err != nil || !strings.Contains(out.String(), "verse.parse") {
		t.Fatalf("help text: %v\n%s", err, out.String())
	}
	out.Reset()
	if err := run([]string{"help"}, nil, &out); err != nil || !strings.HasPrefix(out.String(), "[") {
		t.Fatalf("help should be JSON when piped: %v\n%s", err, out.String())
	}
	if err := run([]string{"-format", "xml", "help"}, nil, &out); err == nil {
		t.Fatal("expected bad format error")
	}
}

func TestMCPSubcommand(t *testing.T) {
	lib := filepath.Join(t.TempDir(), "library.db")
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	var out bytes.Buffer
	if err := run([]string{"-library", lib, "mcp"}, in, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"result":{}`) {
		t.Fatalf("mcp output: %s", out.String())
	}
}
