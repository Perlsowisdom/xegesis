package document

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

type env struct {
	t   *testing.T
	r   *command.Registry
	env *command.Env
	st  *store.Store
}

// setup makes a registry with stand-ins for bible.modules and bible.read: "kjv" is public domain,
// "live" may not be stored and "mine" is a personal copy limited to one page per export.
func setup(t *testing.T) *env {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	r := command.NewRegistry()
	pd, _ := licence.Defaults(licence.PublicDomain, "")
	live, _ := licence.Defaults(licence.CreativeCommons, "CC-BY-4.0")
	live.Store, live.RightsHolder = false, "A Society"
	mine, _ := licence.Defaults(licence.PersonalPurchase, "")
	mine.Export, mine.RightsHolder = &licence.Limit{Pages: 1}, "A Publisher"
	r.Add(command.Define("bible.modules", "stand-in", false, func(context.Context, *command.Env, struct{}) ([]bible.Meta, error) {
		return []bible.Meta{{ID: "kjv", Abbreviation: "KJV", Licence: pd}, {ID: "live", Name: "Live Bible", Licence: live},
			{ID: "mine", Abbreviation: "MINE", Licence: mine}}, nil
	}))
	type readParams struct {
		Module string `json:"module"`
		Ref    string `json:"ref"`
	}
	r.Add(command.Define("bible.read", "stand-in", false, func(_ context.Context, _ *command.Env, p readParams) (bible.Passage, error) {
		rg, err := verse.Parse(p.Ref)
		if err != nil {
			return bible.Passage{}, err
		}
		out := bible.Passage{Module: p.Module, Ref: rg.String()}
		for id := rg.Start; id <= rg.End; id++ {
			text := "Verse text " + id.String()
			if p.Module == "mine" {
				text = strings.Repeat("long ", 300)
			}
			out.Verses = append(out.Verses, bible.ReadVerse{ID: id, Ref: id.String(), Text: text})
		}
		return out, nil
	}))
	Register(r)
	core.Register(r)
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

func TestCreateUpdateOutline(t *testing.T) {
	e := setup(t)
	var d Document
	e.must("document.create", map[string]any{"kind": "lesson", "passage": "Romans 8:28-39"}, &d)
	if d.Title != "Untitled lesson" || d.Kind != "lesson" || d.Passage == nil || d.Passage.Display != "Romans 8:28-39" || len(d.Points) != 0 {
		t.Fatalf("created = %+v", d)
	}
	points := []Point{{Heading: "  God   works  ", Ref: "Rom 8:28"}, {Heading: "", Ref: ""}, {Heading: "Nothing separates", Ref: "rom 8:38-39", Level: 2}}
	e.must("document.update", map[string]any{"id": d.ID, "title": "More than conquerors", "points": points, "body": "Some **words** here."}, &d)
	if d.Title != "More than conquerors" || len(d.Points) != 2 || d.Points[0].Heading != "God works" || d.Points[0].Level != 1 ||
		d.Points[1].Ref != "Romans 8:38-39" || d.Points[1].Level != 2 || d.Words != 3 {
		t.Fatalf("updated = %+v", d)
	}
	if d.Passage == nil || d.Passage.Display != "Romans 8:28-39" {
		t.Fatalf("passage lost: %+v", d.Passage)
	}
	// Clearing the passage keeps the points; a bad reference changes nothing.
	var cleared Document
	e.must("document.update", map[string]any{"id": d.ID, "passage": ""}, &cleared)
	if cleared.Passage != nil || len(cleared.Points) != 2 {
		t.Fatalf("cleared = %+v", cleared)
	}
	if err := e.run("document.update", map[string]any{"id": d.ID, "passage": "Nowhere 99"}, nil); code(err) != "bad_params" {
		t.Fatalf("bad passage: %v", err)
	}
	if err := e.run("document.create", map[string]any{"kind": "poem"}, nil); code(err) != "bad_params" {
		t.Fatalf("bad kind: %v", err)
	}
	// Every change is in the history.
	var n int
	e.st.QueryRow(context.Background(), `SELECT COUNT(*) FROM changes WHERE object_id = ? AND command LIKE 'document.%'`, d.ID).Scan(&n)
	if n < 4 {
		t.Fatalf("changes logged = %d", n)
	}
}

func TestListFilters(t *testing.T) {
	e := setup(t)
	var a, b, c Document
	e.must("document.create", map[string]any{"title": "Grace alone", "passage": "Eph 2:1-10"}, &a)
	e.must("document.create", map[string]any{"kind": "study", "title": "Psalm study", "points": []Point{{Heading: "Shepherd", Ref: "Ps 23:1"}}}, &b)
	e.must("document.create", map[string]any{"kind": "lesson", "title": "Faith", "body": "Abraham believed God"}, &c)
	var got []Document
	e.must("document.list", map[string]any{}, &got)
	if len(got) != 3 || got[0].ID != c.ID {
		t.Fatalf("list = %d, first %s", len(got), got[0].Title)
	}
	e.must("document.list", map[string]any{"kind": "study"}, &got)
	if len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("kind filter = %+v", got)
	}
	e.must("document.list", map[string]any{"query": "abraham GOD"}, &got)
	if len(got) != 1 || got[0].ID != c.ID {
		t.Fatalf("query filter = %+v", got)
	}
	e.must("document.list", map[string]any{"ref": "Psalm 23"}, &got)
	if len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("ref filter by point = %+v", got)
	}
	e.must("document.list", map[string]any{"ref": "Eph 2:8"}, &got)
	if len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("ref filter by passage = %+v", got)
	}
}

func TestDeleteRestore(t *testing.T) {
	e := setup(t)
	var d Document
	e.must("document.create", map[string]any{"title": "Gone"}, &d)
	e.must("document.delete", map[string]any{"id": d.ID}, nil)
	if err := e.run("document.get", map[string]any{"id": d.ID}, nil); code(err) != "not_found" {
		t.Fatalf("get deleted: %v", err)
	}
	if err := e.run("document.update", map[string]any{"id": d.ID, "title": "x"}, nil); code(err) != "not_found" {
		t.Fatalf("update deleted: %v", err)
	}
	var del []Document
	e.must("document.deleted", map[string]any{}, &del)
	if len(del) != 1 || del[0].DeletedAt == 0 {
		t.Fatalf("deleted = %+v", del)
	}
	e.must("document.restore", map[string]any{"id": d.ID}, &d)
	if d.Title != "Gone" {
		t.Fatalf("restored = %+v", d)
	}
	if err := e.run("document.restore", map[string]any{"id": d.ID}, nil); code(err) != "bad_params" {
		t.Fatalf("restore live: %v", err)
	}
}

func TestQuoteFollowsLicence(t *testing.T) {
	e := setup(t)
	var d Document
	e.must("document.create", map[string]any{"title": "Love", "body": "Opening words."}, &d)
	var q QuoteResult
	e.must("document.quote", map[string]any{"id": d.ID, "ref": "John 3:16-17", "module": "kjv"}, &q)
	if !q.Allowed || q.Document == nil {
		t.Fatalf("quote = %+v", q)
	}
	body := q.Document.Body
	for _, want := range []string{"Opening words.\n\n> **16** Verse text", " **17** Verse text", "[John 3:16-17](#passage/John.3.16-John.3.17) (KJV)"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %q:\n%s", want, body)
		}
	}
	// A Bible whose licence forbids keeping its text is not copied.
	e.must("document.quote", map[string]any{"id": d.ID, "ref": "John 3:16", "module": "live"}, &q)
	if q.Allowed || !strings.Contains(q.Reason, "does not allow keeping") {
		t.Fatalf("live quote = %+v", q)
	}
	// A limited licence caps one quotation, and adds its credit.
	e.must("document.quote", map[string]any{"id": d.ID, "ref": "John 3:16-18", "module": "mine"}, &q)
	if q.Allowed || !strings.Contains(q.Reason, "2000 characters") {
		t.Fatalf("long quote = %+v", q)
	}
	e.must("document.quote", map[string]any{"id": d.ID, "ref": "John 3:16", "module": "mine"}, &q)
	if !q.Allowed || !strings.Contains(q.Quote, "(MINE). © A Publisher") {
		t.Fatalf("short quote = %+v", q)
	}
	var got Document
	e.must("document.get", map[string]any{"id": d.ID}, &got)
	if strings.Count(got.Body, "> —") != 2 {
		t.Fatalf("quotations kept = %d:\n%s", strings.Count(got.Body, "> —"), got.Body)
	}
	if err := e.run("document.quote", map[string]any{"id": d.ID, "ref": "John 3:16", "module": "nope"}, nil); code(err) != "not_found" {
		t.Fatalf("unknown module: %v", err)
	}
	// The credit is the quotation's last line, with no empty quote line before it.
	if !strings.Contains(got.Body, "John 3:17\n> — [John 3:16-17]") {
		t.Fatalf("credit line:\n%s", got.Body)
	}
}

func TestQuotePreview(t *testing.T) {
	e := setup(t)
	var q QuoteResult
	e.must("document.quote", map[string]any{"ref": "John 3:16", "module": "kjv", "preview": true}, &q)
	if !q.Allowed || q.Document != nil || !strings.HasPrefix(q.Quote, "> **16** Verse text") || !strings.Contains(q.Quote, "\n> — [John 3:16](#passage/John.3.16) (KJV)") {
		t.Fatalf("preview = %+v", q)
	}
	// The licence rules are the same: nothing to store means nothing to place.
	q = QuoteResult{}
	e.must("document.quote", map[string]any{"ref": "John 3:16", "module": "live", "preview": true}, &q)
	if q.Allowed || q.Quote != "" {
		t.Fatalf("live preview = %+v", q)
	}
	if err := e.run("document.quote", map[string]any{"ref": "John 3:16", "module": "kjv"}, nil); code(err) != "bad_params" {
		t.Fatalf("no id and no preview: %v", err)
	}
	var d Document
	e.must("document.create", map[string]any{"title": "Kept", "body": "Text."}, &d)
	e.must("document.quote", map[string]any{"id": d.ID, "ref": "John 3:16", "module": "kjv", "preview": true}, &q)
	e.must("document.get", map[string]any{"id": d.ID}, &d)
	if d.Body != "Text." {
		t.Fatalf("preview changed the text: %q", d.Body)
	}
}

func TestOldOutlineArray(t *testing.T) {
	e := setup(t)
	err := e.st.Write(context.Background(), "test", func(tx *store.Tx) error {
		return tx.Create("documents", store.Row{"id": "D1", "title": "Old", "outline_json": `[{"h":"One"},{"h":"Two"}]`,
			"created_at": int64(1), "updated_at": int64(2)})
	})
	if err != nil {
		t.Fatal(err)
	}
	var d Document
	e.must("document.get", map[string]any{"id": "D1"}, &d)
	if d.Kind != "sermon" || len(d.Points) != 2 || d.Points[1].Heading != "Two" {
		t.Fatalf("old = %+v", d)
	}
}

func TestQuoteOnlySomeWords(t *testing.T) {
	e := setup(t)
	var q QuoteResult
	// A highlight's exact words keep the place and the credit line; the licence's limit counts only them.
	e.must("document.quote", map[string]any{"preview": true, "ref": "John 3:16-17", "module": "mine", "words": "long  long"}, &q)
	if !q.Allowed || !strings.HasPrefix(q.Quote, "> long long\n> — [John 3:16-17](#passage/John.3.16-John.3.17) (MINE). © A Publisher") || strings.Contains(q.Quote, "**16**") {
		t.Fatalf("words quote = %+v", q)
	}
	// Words that are not in the passage are not quoted as if they were.
	e.must("document.quote", map[string]any{"preview": true, "ref": "John 3:16", "module": "kjv", "words": "God so loved"}, &q)
	if q.Allowed || !strings.Contains(q.Reason, "not in") {
		t.Fatalf("missing words = %+v", q)
	}
	// The licence still decides.
	e.must("document.quote", map[string]any{"preview": true, "ref": "John 3:16", "module": "live", "words": "Verse text"}, &q)
	if q.Allowed {
		t.Fatalf("live words = %+v", q)
	}
}

func TestSeveralPassages(t *testing.T) {
	e := setup(t)
	var d Document
	e.must("document.create", map[string]any{"passages": []string{"Matt 14:22-33", "Psalm 84"}}, &d)
	if len(d.Passages) != 2 || d.Passages[1].Display != "Psalms 84" || d.Passage == nil || d.Passage.Display != d.Passages[0].Display {
		t.Fatalf("created = %+v", d.Passages)
	}
	// Found by any of its passages, and an older single passage still works.
	for _, ref := range []string{"Matt 14:25", "Ps 84:1"} {
		var list []Document
		e.must("document.list", map[string]any{"ref": ref}, &list)
		if len(list) != 1 || list[0].ID != d.ID {
			t.Fatalf("ref %s found %d documents", ref, len(list))
		}
	}
	var one Document
	e.must("document.update", map[string]any{"id": d.ID, "passage": "John 3:16"}, &one)
	if len(one.Passages) != 1 || one.Passages[0].Display != "John 3:16" {
		t.Fatalf("single passage replaces the list: %+v", one.Passages)
	}
	var none Document
	e.must("document.update", map[string]any{"id": d.ID, "passages": []string{}}, &none)
	if len(none.Passages) != 0 || none.Passage != nil {
		t.Fatalf("cleared: %+v", none.Passages)
	}
}

func TestTurnNoteInto(t *testing.T) {
	e := setup(t)
	var f struct{ ID string }
	f.ID = store.NewID()
	if err := e.st.Write(context.Background(), "folder.create", func(tx *store.Tx) error {
		return tx.Create("folders", store.Row{"id": f.ID, "name": "Sunday", "sort": 0, "created_at": int64(1), "updated_at": int64(1)})
	}); err != nil {
		t.Fatal(err)
	}
	var n core.Note
	e.must("note.create", map[string]any{"title": "Grace", "body": "Grace is **free**. [John 3:16](#passage/John.3.16)",
		"ref": "Rom 5:1-5", "tags": []string{"grace", "romans"}, "folder_id": f.ID}, &n)
	var d Document
	e.must("note.turn_into", map[string]any{"id": n.ID, "kind": "study"}, &d)
	if d.Kind != "study" || d.Title != "Grace" || d.Body != n.Body || d.FolderID != f.ID || d.FromNote != n.ID ||
		len(d.Passages) != 1 || d.Passages[0].Display != "Romans 5:1-5" || len(d.Tags) != 2 {
		t.Fatalf("document = %+v", d)
	}
	var live []core.Note
	e.must("note.list", map[string]any{}, &live)
	if len(live) != 0 {
		t.Fatalf("the note should be in the bin: %d left", len(live))
	}
	// Undo: bring the note back and the document goes away.
	var back core.Note
	e.must("note.restore", map[string]any{"id": n.ID}, &back)
	e.must("document.delete", map[string]any{"id": d.ID}, nil)
	if back.Body != n.Body || back.Title != "Grace" {
		t.Fatalf("restored note = %+v", back)
	}
	if err := e.run("note.turn_into", map[string]any{"id": "nope", "kind": "sermon"}, nil); code(err) != "not_found" {
		t.Fatalf("missing note: %v", err)
	}
	// An edit keeps the tags.
	var n2 core.Note
	e.must("note.create", map[string]any{"title": "T", "body": "x", "tags": []string{"a"}}, &n2)
	var d2, d3 Document
	e.must("note.turn_into", map[string]any{"id": n2.ID, "kind": "sermon"}, &d2)
	e.must("document.update", map[string]any{"id": d2.ID, "title": "Renamed", "passages": []string{"Ps 1"}}, &d3)
	if len(d3.Tags) != 1 || d3.FromNote != n2.ID {
		t.Fatalf("tags lost on edit: %+v", d3)
	}
}
