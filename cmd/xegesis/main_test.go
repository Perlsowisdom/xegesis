package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAndServe(t *testing.T) {
	lib := filepath.Join(t.TempDir(), "library.db")
	var out bytes.Buffer
	if err := run([]string{"-library", lib, "run", "verse.parse", `{"ref":"John 3:16"}`}, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"start": 43003016`) {
		t.Fatalf("run output: %s", out.String())
	}

	in := strings.NewReader(`{"id":1,"command":"note.create","params":{"body":"hi","ref":"Ps 23"}}
{"id":2,"command":"note.list","params":{"ref":"Ps 23:1"}}
{"id":3,"command":"nope"}
not json
`)
	out.Reset()
	if err := run([]string{"-library", lib, "serve"}, in, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d responses: %s", len(lines), out.String())
	}
	var list Response
	json.Unmarshal([]byte(lines[1]), &list)
	if notes, _ := list.Result.([]any); len(notes) != 1 {
		t.Fatalf("note.list response: %s", lines[1])
	}
	if !strings.Contains(lines[2], "unknown_command") || !strings.Contains(lines[3], "bad_request") {
		t.Fatalf("error responses: %s / %s", lines[2], lines[3])
	}
}

func TestCommandsList(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"commands"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id": "note.create"`) {
		t.Fatal("note.create not listed")
	}
}
