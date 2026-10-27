package core

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

func setup(t *testing.T) (*command.Registry, *command.Env) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	r := command.NewRegistry()
	Register(r)
	return r, &command.Env{Store: s}
}

// run calls a command with JSON params and decodes the result into out.
func run(t *testing.T, r *command.Registry, env *command.Env, id, params string, out any) error {
	t.Helper()
	res, err := r.Run(context.Background(), env, id, json.RawMessage(params))
	if err != nil {
		return err
	}
	if out != nil {
		b, _ := json.Marshal(res)
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatal(err)
		}
	}
	return nil
}

func code(err error) string {
	var ce *command.Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func TestEveryCommandDescribesItself(t *testing.T) {
	r, _ := setup(t)
	list := r.List()
	if len(list) < 10 {
		t.Fatalf("only %d commands", len(list))
	}
	for _, d := range list {
		if d.Summary == "" || d.Params == nil || d.Params.Type != "object" {
			t.Errorf("%s: incomplete description %+v", d.ID, d)
		}
	}
	d, _ := r.Describe("verse.convert")
	if len(d.Params.Required) != 3 || len(d.Params.Properties["from"].Enum) != 2 {
		t.Errorf("verse.convert schema: %+v", d.Params)
	}
	d, _ = r.Describe("history.restore")
	if len(d.Params.Required) != 3 {
		t.Errorf("embedded params not flattened: %+v", d.Params)
	}
}

func TestParamErrors(t *testing.T) {
	r, env := setup(t)
	cases := map[string][2]string{
		"unknown command": {"nope", `{}`},
		"unknown field":   {"verse.parse", `{"ref":"Rom 1:1","extra":1}`},
		"missing field":   {"verse.parse", `{}`},
		"bad enum":        {"verse.convert", `{"ref":"Rom 1:1","from":"english","to":"klingon"}`},
		"bad reference":   {"verse.parse", `{"ref":"Nowhere 1:1"}`},
	}
	for name, c := range cases {
		err := run(t, r, env, c[0], c[1], nil)
		if want := map[bool]string{true: "unknown_command", false: "bad_params"}[c[0] == "nope"]; code(err) != want {
			t.Errorf("%s: got %v, want %s", name, err, want)
		}
	}
	if err := run(t, r, &command.Env{}, "note.create", `{"body":"x"}`, nil); code(err) != "unavailable" {
		t.Errorf("write without a library: %v", err)
	}
}

func TestVerseAndLicenceCommands(t *testing.T) {
	r, env := setup(t)
	var ref RefResult
	if err := run(t, r, env, "verse.convert", `{"ref":"Mal 4:5-6","from":"english","to":"hebrew"}`, &ref); err != nil {
		t.Fatal(err)
	}
	if ref.Display != "Malachi 3:23-24" || ref.OSIS != "Mal.3.23-Mal.3.24" {
		t.Fatalf("convert = %+v", ref)
	}
	var d struct {
		Allowed  bool `json:"allowed"`
		MaxPages int  `json:"max_pages"`
	}
	err := run(t, r, env, "licence.check", `{"licence":{"type":"personal_purchase","print_limit":{"pages":50}},
		"request":{"action":"print","pages":60}}`, &d)
	if err != nil || d.Allowed || d.MaxPages != 50 {
		t.Fatalf("licence.check = %+v, %v", d, err)
	}
}

func TestNotesWithHistory(t *testing.T) {
	r, env := setup(t)
	var n Note
	if err := run(t, r, env, "note.create", `{"title":"Hope","body":"first","ref":"Rom 8:28"}`, &n); err != nil {
		t.Fatal(err)
	}
	if n.Ref == nil || n.Ref.Start != 45008028 {
		t.Fatalf("note ref = %+v", n.Ref)
	}
	if err := run(t, r, env, "note.update", `{"id":"`+n.ID+`","body":"second"}`, &n); err != nil || n.Body != "second" {
		t.Fatalf("update = %+v, %v", n, err)
	}
	var list []Note
	run(t, r, env, "note.list", `{"ref":"Rom 8"}`, &list)
	if len(list) != 1 {
		t.Fatalf("notes on Rom 8 = %d", len(list))
	}
	run(t, r, env, "note.list", `{"ref":"Rom 9"}`, &list)
	if len(list) != 0 {
		t.Fatalf("notes on Rom 9 = %d", len(list))
	}

	var h []store.Change
	if err := run(t, r, env, "history.list", `{"object_type":"notes","object_id":"`+n.ID+`"}`, &h); err != nil {
		t.Fatal(err)
	}
	if len(h) != 3 { // create, body_md, updated_at
		t.Fatalf("history = %d changes", len(h))
	}
	if err := run(t, r, env, "note.delete", `{"id":"`+n.ID+`"}`, nil); err != nil {
		t.Fatal(err)
	}
	if err := run(t, r, env, "note.get", `{"id":"`+n.ID+`"}`, nil); code(err) != "not_found" {
		t.Fatalf("deleted note still readable: %v", err)
	}
	if err := run(t, r, env, "note.update", `{"id":"`+n.ID+`","body":"x"}`, nil); code(err) != "not_found" {
		t.Fatalf("deleted note updatable: %v", err)
	}
	err := run(t, r, env, "history.restore", `{"object_type":"notes","object_id":"`+n.ID+`","change_id":"`+h[0].ID+`"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := run(t, r, env, "note.get", `{"id":"`+n.ID+`"}`, &n); err != nil || n.Body != "first" {
		t.Fatalf("restored note = %+v, %v", n, err)
	}
}
