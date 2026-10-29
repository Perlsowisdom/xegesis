package lists

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

type copyParams struct {
	ItemID string `json:"item_id"`
	FileID string `json:"file_id"`
	Text   string `json:"text"`
}

type env struct {
	t   *testing.T
	r   *command.Registry
	env *command.Env
	st  *store.Store
}

// setup makes a registry with a stand-in for book.copy, which refuses text over 20 characters.
func setup(t *testing.T) *env {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	r := command.NewRegistry()
	core.Register(r)
	r.Add(command.Define("book.copy", "stand-in", true, func(_ context.Context, _ *command.Env, p copyParams) (map[string]any, error) {
		if len(p.Text) > 20 {
			return map[string]any{"allowed": false, "reason": "too long"}, nil
		}
		return map[string]any{"allowed": true, "attribution": "Smith, A Book (CC-BY)"}, nil
	}))
	Register(r)
	return &env{t: t, r: r, env: &command.Env{Store: st}, st: st}
}

func (e *env) run(id string, params any, out any) error {
	e.t.Helper()
	raw, _ := json.Marshal(params)
	res, err := e.r.Run(context.Background(), e.env, id, raw)
	if err != nil {
		return err
	}
	if out != nil {
		b, _ := json.Marshal(res)
		if err := json.Unmarshal(b, out); err != nil {
			e.t.Fatal(err)
		}
	}
	return nil
}

func (e *env) must(id string, params any, out any) {
	e.t.Helper()
	if err := e.run(id, params, out); err != nil {
		e.t.Fatalf("%s: %v", id, err)
	}
}

func code(err error) string {
	var ce *command.Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func TestVerseListFillReorderOpen(t *testing.T) {
	e := setup(t)
	var l List
	e.must("list.create", map[string]any{"kind": "verses", "name": "  Love   verses "}, &l)
	if l.Name != "Love verses" || l.Count != 0 {
		t.Fatalf("list = %+v", l)
	}
	var a, b, c, again Entry
	e.must("list.add", map[string]any{"list": l.ID, "ref": "John 3:16"}, &a)
	e.must("list.add", map[string]any{"list": l.ID, "verse_start": 46013004, "verse_end": 46013007, "note": "what love does"}, &b)
	e.must("list.add", map[string]any{"list": l.ID, "ref": "1 John 4:8"}, &c)
	e.must("list.add", map[string]any{"list": l.ID, "ref": "John 3:16"}, &again)
	if !again.Already || again.ID != a.ID {
		t.Errorf("adding the same verses again: %+v", again)
	}
	if a.Ref.Display != "John 3:16" || b.Ref.Display != "1 Corinthians 13:4-7" || b.Note != "what love does" {
		t.Errorf("entries: %+v %+v", a.Ref, b)
	}

	e.must("list.reorder", map[string]any{"list": l.ID, "ids": []string{c.ID, a.ID}}, nil)
	var items Items
	e.must("list.items", map[string]any{"id": l.ID}, &items)
	var order []string
	for _, x := range items.Entries {
		order = append(order, x.ID)
	}
	if strings.Join(order, ",") != strings.Join([]string{c.ID, a.ID, b.ID}, ",") || items.List.Count != 3 {
		t.Errorf("order = %v, count %d", order, items.List.Count)
	}
	if err := e.run("list.reorder", map[string]any{"list": l.ID, "ids": []string{"nope"}}, nil); code(err) != "bad_params" {
		t.Errorf("reorder with a stranger: %v", err)
	}

	e.must("list.remove", map[string]any{"id": a.ID}, nil)
	e.must("list.items", map[string]any{"id": l.ID}, &items)
	if len(items.Entries) != 2 {
		t.Errorf("after remove: %d", len(items.Entries))
	}
	// Taking an entry out is logged, so it can be brought back from history.
	h, _ := e.st.History(context.Background(), "list_entries", a.ID)
	if len(h) != 3 || h[2].Op != "delete" {
		t.Fatalf("history = %+v", h)
	}
	if err := e.st.Restore(context.Background(), "history.restore", "list_entries", store.ByID(a.ID), h[0].ID); err != nil {
		t.Fatal(err)
	}
	e.must("list.items", map[string]any{"id": l.ID}, &items)
	if len(items.Entries) != 3 {
		t.Errorf("after restore: %d", len(items.Entries))
	}

	if err := e.run("list.add", map[string]any{"list": l.ID, "strong": "G26"}, nil); code(err) != "bad_params" {
		t.Errorf("a word in a verse list: %v", err)
	}
	e.must("list.rename", map[string]any{"id": l.ID, "name": "Love"}, &l)
	e.must("list.delete", map[string]any{"id": l.ID}, nil)
	var all []List
	e.must("list.list", map[string]any{}, &all)
	if len(all) != 0 {
		t.Errorf("deleted list still listed: %v", all)
	}
	if err := e.run("list.add", map[string]any{"list": l.ID, "ref": "John 1:1"}, nil); code(err) != "not_found" {
		t.Errorf("adding to a deleted list: %v", err)
	}
}

func TestVersesAreKeptInEnglishNumbering(t *testing.T) {
	e := setup(t)
	var l List
	e.must("list.create", map[string]any{"kind": "verses", "name": "Joel"}, &l)
	var x Entry
	// Hebrew Joel 3:1 is English Joel 2:28.
	e.must("list.add", map[string]any{"list": l.ID, "ref": "Joel 3:1", "versification": "hebrew"}, &x)
	if x.Ref.Display != "Joel 2:28" {
		t.Errorf("stored as %s", x.Ref.Display)
	}
}

func TestWordList(t *testing.T) {
	e := setup(t)
	var l List
	e.must("list.create", map[string]any{"kind": "words", "name": "Greek"}, &l)
	var a, again, b Entry
	e.must("list.add", map[string]any{"list": l.ID, "word": "love", "strong": " g26", "lemma": "ἀγάπη", "language": "grc"}, &a)
	e.must("list.add", map[string]any{"list": l.ID, "word": "loved", "strong": "G26"}, &again)
	e.must("list.add", map[string]any{"list": l.ID, "strong": "H2617"}, &b)
	if a.Strong != "G26" || a.Lemma != "ἀγάπη" || a.Word != "love" || a.Language != "grc" || !again.Already || b.Word != "H2617" {
		t.Errorf("words: %+v %+v %+v", a, again, b)
	}
	if err := e.run("list.add", map[string]any{"list": l.ID, "ref": "John 1:1"}, nil); code(err) != "bad_params" {
		t.Errorf("verses in a word list: %v", err)
	}
	var all []List
	e.must("list.list", map[string]any{"kind": "words"}, &all)
	if len(all) != 1 || all[0].Count != 2 {
		t.Errorf("lists = %+v", all)
	}
}

func TestExcerptListRespectsTheLicence(t *testing.T) {
	e := setup(t)
	now := int64(1)
	if err := e.st.Write(context.Background(), "test.item", func(tx *store.Tx) error {
		return tx.Create("items", store.Row{"id": "book1", "kind": "monograph", "title": "A Book", "source": "import",
			"created_at": now, "updated_at": now})
	}); err != nil {
		t.Fatal(err)
	}
	var l List
	e.must("list.create", map[string]any{"kind": "excerpts", "name": "Quotes"}, &l)
	loc := map[string]any{"section": 2, "para": 3, "offset": 0, "end_para": 3, "end_offset": 9}
	var x Entry
	e.must("list.add", map[string]any{"list": l.ID, "item_id": "book1", "locator": loc, "text": "Short text"}, &x)
	if x.Text != "Short text" || x.Title != "A Book" || x.Attribution != "Smith, A Book (CC-BY)" || x.Locator["para"].(float64) != 3 {
		t.Errorf("excerpt = %+v", x)
	}
	err := e.run("list.add", map[string]any{"list": l.ID, "item_id": "book1", "locator": loc, "text": strings.Repeat("long ", 10)}, nil)
	if code(err) != "not_allowed" || !strings.Contains(fmt.Sprint(err), "too long") {
		t.Errorf("text over the limit: %v", err)
	}
	var items Items
	e.must("list.items", map[string]any{"id": l.ID}, &items)
	if len(items.Entries) != 1 {
		t.Errorf("a refused excerpt was added: %d", len(items.Entries))
	}
}
