package report

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/lists"
	"github.com/Perlsowisdom/xegesis/internal/plan"
	"github.com/Perlsowisdom/xegesis/internal/reader"
	"github.com/Perlsowisdom/xegesis/internal/search"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// Every test must stop the goroutines it starts and close what it opens (ADR 0008).
func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

// Fixtures: a few public-domain KJV words typed by hand, and tiny samples in the formats of
// STEPBible's TAGNT and TBESG (CC BY 4.0).
const osisKJV = `<?xml version="1.0" encoding="UTF-8"?>
<osis xmlns="http://www.bibletechnologies.net/2003/OSIS/namespace">
<osisText osisIDWork="KJVtest" xml:lang="en">
<header><work osisWork="KJVtest"><title>King James Version (test)</title></work></header>
<div type="book" osisID="John"><chapter osisID="John.3">
<verse osisID="John.3.16">For God so <w lemma="strong:G25">loved</w> the world, that he gave his only begotten Son.</verse>
</chapter></div>
<div type="book" osisID="Rom"><chapter osisID="Rom.8">
<verse osisID="Rom.8.28">And we know that all things work together for good to them that <w lemma="strong:G25">love</w> God.<note type="crossReference"><reference osisRef="Gen.50.20">Gen 50:20</reference></note></verse>
<verse osisID="Rom.8.29">For whom he did foreknow, he also did predestinate.</verse>
</chapter></div>
<div type="book" osisID="Eph"><chapter osisID="Eph.2">
<verse osisID="Eph.2.8">For by grace are ye saved through faith; and that not of yourselves: it is the gift of God.</verse>
</chapter></div>
</osisText></osis>`

const tagnt = "TAGNT test\n" +
	"Word & Type\tGreek\tEnglish translation\tdStrongs = Grammar\n" +
	"Jhn.3.16#01=NKO\tοὕτως (houtōs)\tThus\tG3779=ADV\tοὕτω=thus\n" +
	"Jhn.3.16#02=NKO\tἠγάπησεν (ēgapēsen)\tloved\tG0025=V-AAI-3S\tἀγαπάω=to love\n" +
	"Rom.8.28#01=NKO\tΟἴδαμεν (Oidamen)\tWe know\tG6063=V-RAI-1P\tοἶδα=to know\n" +
	"Rom.8.28#02=NKO\tἀγαπῶσιν (agapōsin)\tloving\tG0025=V-PAP-DPM\tἀγαπάω=to love\n" +
	"Rom.8.28#03=NKO\tθεὸν (theon)\tGod\tG2316G=N-ASM-T\tθεός=God\n"

const tbesg = "TBESG test header\n" +
	"eStrong#\tdStrong\tuStrong\tGreek\tTransliteration\tMorph\tGloss\tMeaning\n" +
	"G0025\tG0025 =\tG0025\tἀγαπάω\tagapaō\tG:V\tto love\t<b>ἀγαπάω</b>, to love: <ref='Jhn.3.16'>Jhn.3:16</ref>; cf. G0026.\n" +
	"G0026\tG0026 =\tG0026\tἀγάπη\tagapē\tG:N-F\tlove\t<b>ἀγάπη</b>, love\n"

type env struct {
	t   *testing.T
	r   *command.Registry
	env *command.Env
}

// setup makes a library with Bibles, a lexicon, a commentary, a topical index and a dictionary,
// and stands in for the library search, which needs indexed books.
func setup(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	lib := bible.NewLibrary(filepath.Join(dir, "modules"))
	t.Cleanup(lib.Close)
	install(t, lib, dir)

	r := command.NewRegistry()
	core.Register(r)
	bible.RegisterLibrary(r, lib)
	lists.Register(r)
	plan.Register(r)
	reader.Register(r)
	hit := search.Hit{ItemID: "item1", Title: "Lectures on Romans", Authors: []string{"A. Writer"}, FileID: "f1", Online: true,
		Heading: "Chapter Two", Locator: search.Locator{Spine: 2, Para: 1}, Snippet: "All things work together for <mark>good</mark>"}
	r.Add(
		command.Define("search.refs", "stand-in", false, func(_ context.Context, _ *command.Env, p search.RefsParams) (search.QueryResult, error) {
			h := hit
			h.Cites = "Romans 8:28-30"
			return search.QueryResult{Hits: []search.Hit{h}, More: p.Offset == 0, Incomplete: []string{}}, nil
		}),
		command.Define("search.query", "stand-in", false, func(_ context.Context, _ *command.Env, p search.QueryParams) (search.QueryResult, error) {
			if strings.Contains(p.Query, "ἀγαπάω") || strings.EqualFold(p.Query, "grace") {
				return search.QueryResult{Hits: []search.Hit{hit}, Incomplete: []string{"folder2"}}, nil
			}
			return search.QueryResult{Hits: []search.Hit{}, Incomplete: []string{}}, nil
		}),
	)
	Register(r)
	return &env{t: t, r: r, env: &command.Env{Store: st}}
}

func install(t *testing.T, lib *bible.Library, dir string) {
	ctx := context.Background()
	pd, _ := licence.Defaults(licence.PublicDomain, "")
	cc, _ := licence.Defaults(licence.CreativeCommons, "CC-BY-4.0")
	cc.Attribution = "STEPBible.org, CC BY 4.0"
	path := filepath.Join(dir, "kjv.xml")
	if err := os.WriteFile(path, []byte(osisKJV), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := lib.Install(ctx, bible.InstallOptions{Path: path, ID: "kjv", Name: "King James Version", Abbreviation: "KJV", Licence: pd}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := lib.InstallSTEP(ctx, bible.InstallOptions{ID: "tagnt", Name: "Greek NT", Language: "grc", Licence: cc},
		[]io.Reader{strings.NewReader(tagnt)}); err != nil {
		t.Fatal(err)
	}
	heads, err := bible.ReadSTEPLexicon(strings.NewReader(tbesg), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.InstallHeadwords(ctx, bible.InstallOptions{ID: "tbesg", Name: "Greek lexicon", Kind: bible.KindLexicon, Licence: cc}, heads); err != nil {
		t.Fatal(err)
	}
	_, _, err = lib.InstallEntries(ctx, bible.InstallOptions{ID: "notes", Name: "Short notes", Kind: bible.KindCommentary, Licence: pd}, "osis",
		func(emit func(bible.Entry) error) error {
			return emit(bible.Entry{ID: 45008028, Markup: `All things serve the good of those who love God; compare <reference osisRef="Gen.50.20">Gen 50:20</reference>.`})
		})
	if err != nil {
		t.Fatal(err)
	}
	topical := []bible.Headword{
		{Key: "GRACE", Text: "- Given by Christ 1Co 1:4; Joh 1:17, 18; 5:2", Markup: "- Given by Christ<p/><r>1Co 1:4; Joh 1:17, 18; Ro 5:2</r>"},
		{Key: "GRACE OF GOD", Text: "Ge 15:6", Markup: `<r o="Gen.15.6">Ge 15:6</r>`},
		{Key: "HOPE", Text: "Ro 8:24", Markup: "<r>Ro 8:24</r>"},
	}
	if _, err := lib.InstallHeadwords(ctx, bible.InstallOptions{ID: "topics", Name: "Topical Textbook (test)", Licence: pd}, topical); err != nil {
		t.Fatal(err)
	}
	dict := []bible.Headword{{Key: "GRACE", Text: "Grace. Favour, kindness, friendship; God's forgiving mercy (Rom. 11:6).",
		Markup: `Grace. Favour, kindness, friendship; God's forgiving mercy (<r o="Rom.11.6">Rom. 11:6</r>).`}}
	if _, err := lib.InstallHeadwords(ctx, bible.InstallOptions{ID: "dict", Name: "Bible Dictionary (test)", Licence: pd}, dict); err != nil {
		t.Fatal(err)
	}
}

func (e *env) run(id string, params any, out any) error {
	e.t.Helper()
	raw, _ := json.Marshal(params)
	res, err := e.r.Run(context.Background(), e.env, id, raw)
	if err != nil {
		return err
	}
	if out != nil {
		reflect.ValueOf(out).Elem().SetZero()
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

func section(rep Report, id string) *Section {
	for i := range rep.Sections {
		if rep.Sections[i].ID == id {
			return &rep.Sections[i]
		}
	}
	return nil
}

func ids(rep Report) []string {
	var out []string
	for _, s := range rep.Sections {
		out = append(out, s.ID)
	}
	return out
}

func TestPassageReport(t *testing.T) {
	e := setup(t)
	var note core.Note
	e.must("note.create", map[string]any{"title": "Providence", "body": "All things **work** together.", "ref": "Rom 8:28"}, &note)
	e.must("note.create", map[string]any{"title": "Elsewhere", "body": "Not on this verse.", "ref": "John 3:16"}, nil)
	e.must("highlight.create", map[string]any{"ref": "Rom 8:28", "style": "green"}, nil)
	var l lists.List
	e.must("list.create", map[string]any{"kind": "verses", "name": "Comfort"}, &l)
	e.must("list.add", map[string]any{"list": l.ID, "ref": "Rom 8:28-30"}, nil)
	var pl plan.Plan
	e.must("plan.create", map[string]any{"name": "Romans", "ranges": []string{"Rom 8"}, "days": 1}, &pl)

	var rep Report
	e.must("report.passage", map[string]any{"ref": "Romans 8:28"}, &rep)
	want := []string{"text:kjv", "text:tagnt", "commentary:notes", "crossrefs", "words", "citing", "notes", "highlights", "lists", "plans"}
	if strings.Join(ids(rep), " ") != strings.Join(want, " ") {
		t.Fatalf("sections %v, want %v", ids(rep), want)
	}
	if rep.Title != "Passage report: Romans 8:28" || rep.Ref != "Rom.8.28" || rep.Kind != "passage" {
		t.Errorf("report %+v", rep)
	}
	text := section(rep, "text:kjv")
	if len(text.Entries) != 1 || !strings.Contains(text.Entries[0].Text, "all things work together") ||
		text.Sources[0].Name != "King James Version" || text.Sources[0].Licence != "public_domain" || text.Group != "text" {
		t.Errorf("text %+v", text)
	}
	if g := section(rep, "text:tagnt"); g.Sources[0].Attribution != "STEPBible.org, CC BY 4.0" {
		t.Errorf("attribution %+v", g.Sources)
	}
	com := section(rep, "commentary:notes")
	if com.Title != "Short notes" || !strings.Contains(com.Entries[0].Markup, `<r o="Gen.50.20">`) || com.Entries[0].Ref != "Rom.8.28" {
		t.Errorf("commentary %+v", com)
	}
	xs := section(rep, "crossrefs")
	if len(xs.Entries) != 1 || xs.Entries[0].Links[0].Ref != "Gen.50.20" || xs.Sources[0].Module != "kjv" {
		t.Errorf("cross-references %+v", xs)
	}
	words := section(rep, "words")
	if words.Title != "Original-language words" || len(words.Entries) != 3 || words.Sources[0].Module != "tagnt" {
		t.Fatalf("words %+v", words)
	}
	if w := words.Entries[1]; w.Heading != "ἀγαπῶσιν" || w.Strong != "G25" || w.Lemma != "ἀγαπάω" || w.Gloss != "loving" || w.Ref != "Rom.8.28" {
		t.Errorf("word %+v", w)
	}
	citing := section(rep, "citing")
	if b := citing.Entries[0].Book; b == nil || b.ItemID != "item1" || citing.Entries[0].Ref != "Rom.8.28-Rom.8.30" || citing.More != "citing@1" {
		t.Errorf("citing %+v", citing)
	}
	notes := section(rep, "notes")
	if len(notes.Entries) != 1 || notes.Entries[0].NoteID != note.ID || notes.Entries[0].Text != "All things work together." {
		t.Errorf("notes %+v", notes)
	}
	if h := section(rep, "highlights"); h.Entries[0].Extra[0] != "green" {
		t.Errorf("highlights %+v", h)
	}
	if ls := section(rep, "lists"); ls.Entries[0].ListID != l.ID || ls.Entries[0].RefLabel != "Romans 8:28-30" {
		t.Errorf("lists %+v", ls)
	}
	if ps := section(rep, "plans"); ps.Entries[0].PlanID != pl.ID || ps.Entries[0].Day != 1 {
		t.Errorf("plans %+v", ps)
	}

	// Only some sections, then the next page of one.
	e.must("report.passage", map[string]any{"ref": "Rom 8:28", "sections": []string{"text", "notes"}}, &rep)
	if strings.Join(ids(rep), " ") != "text:kjv text:tagnt notes" {
		t.Errorf("chosen sections %v", ids(rep))
	}
	e.must("report.passage", map[string]any{"ref": "Rom 8:28", "bibles": []string{"tagnt"}, "sections": []string{"text"}}, &rep)
	if strings.Join(ids(rep), " ") != "text:tagnt" {
		t.Errorf("chosen Bible %v", ids(rep))
	}
	e.must("report.passage", map[string]any{"ref": "Rom 8:28", "cursor": "citing@1"}, &rep)
	if strings.Join(ids(rep), " ") != "citing" || section(rep, "citing").More != "" {
		t.Errorf("next page %+v", rep.Sections)
	}
	e.must("report.passage", map[string]any{"ref": "Rom 8", "cursor": "text:kjv@1", "limit": 1}, &rep)
	if s := section(rep, "text:kjv"); len(rep.Sections) != 1 || s.Total != 2 || s.Entries[0].Ref != "Rom.8.29" || s.More != "" {
		t.Errorf("text page %+v", rep.Sections)
	}
	for _, bad := range []map[string]any{{"ref": "Nowhere 1:1"}, {"ref": "Rom 8:28", "cursor": "citing"}} {
		var ce *command.Error
		if err := e.run("report.passage", bad, nil); !errors.As(err, &ce) || ce.Code != "bad_params" {
			t.Errorf("%v: %v", bad, err)
		}
	}
}

func TestPassageReportWithoutLibrary(t *testing.T) {
	e := setup(t)
	e.env = &command.Env{}
	var rep Report
	e.must("report.passage", map[string]any{"ref": "Rom 8:28"}, &rep)
	for _, s := range rep.Sections {
		if s.Error != "" || s.Group == "notes" {
			t.Errorf("section %+v", s)
		}
	}
	if section(rep, "text:kjv") == nil {
		t.Errorf("sections %v", ids(rep))
	}
}

func TestWordReport(t *testing.T) {
	e := setup(t)
	var l lists.List
	e.must("list.create", map[string]any{"kind": "words", "name": "Love"}, &l)
	e.must("list.add", map[string]any{"list": l.ID, "word": "loved", "strong": "G25", "lemma": "ἀγαπάω"}, nil)

	var rep Report
	e.must("report.word", map[string]any{"word": "g0025", "bibles": []string{"kjv"}}, &rep)
	if rep.Strong != "G25" || rep.Subject != "G25 ἀγαπάω" || rep.Title != "Word report: G25 ἀγαπάω" {
		t.Errorf("report %+v", rep)
	}
	want := "lexicon:tbesg occurrences renderings:kjv renderings:tagnt related books lists"
	if strings.Join(ids(rep), " ") != want {
		t.Fatalf("sections %v, want %s", ids(rep), want)
	}
	lex := section(rep, "lexicon:tbesg")
	if lex.Entries[0].Lemma != "ἀγαπάω" || lex.Entries[0].Gloss != "to love" || lex.Sources[0].Attribution == "" {
		t.Errorf("lexicon %+v", lex)
	}
	occ := section(rep, "occurrences")
	if occ.Total != 2 || len(occ.Chart) != 2 || occ.Chart[0] != (Bar{Label: "John", Book: "John", Count: 1}) || occ.Chart[1].Book != "Rom" {
		t.Errorf("occurrences %+v", occ)
	}
	if occ.Sources[0].Module != "tagnt" || occ.Entries[0].Module != "kjv" || !strings.Contains(occ.Entries[0].Text, "so loved the world") {
		t.Errorf("occurrence text from the Bible asked for: %+v", occ.Entries)
	}
	kjv := section(rep, "renderings:kjv")
	if kjv.Title != "How King James Version translates it" || len(kjv.Entries) != 2 || kjv.Entries[0].Count != 1 {
		t.Errorf("renderings %+v", kjv)
	}
	if g := section(rep, "renderings:tagnt"); g.Title != "Forms in Greek NT" {
		t.Errorf("forms %+v", g)
	}
	rel := section(rep, "related")
	if len(rel.Entries) != 1 || rel.Entries[0].Strong != "G26" || rel.Entries[0].Lemma != "ἀγάπη" {
		t.Errorf("related %+v", rel)
	}
	if b := section(rep, "books"); b.Hint == "" || b.Entries[0].Book.Title != "Lectures on Romans" {
		t.Errorf("books %+v", b)
	}
	if ls := section(rep, "lists"); ls.Entries[0].ListID != l.ID {
		t.Errorf("lists %+v", ls)
	}

	// By the Greek word, and by an English word the tagged Bible gives the number.
	e.must("report.word", map[string]any{"word": "αγαπαω", "sections": []string{"lexicon"}}, &rep)
	if rep.Strong != "G25" || len(rep.Sections) != 1 {
		t.Errorf("by lemma %+v", rep)
	}
	e.must("report.word", map[string]any{"word": "loved", "sections": []string{"occurrences"}}, &rep)
	if rep.Strong != "G25" || len(rep.Candidates) != 1 || rep.Candidates[0].Lemma != "ἀγαπάω" {
		t.Errorf("by English word %+v", rep)
	}
	e.must("report.word", map[string]any{"word": "zzz"}, &rep)
	if rep.Strong != "" || len(rep.Sections) != 0 {
		t.Errorf("unknown word %+v", rep)
	}
}

func TestTopicReport(t *testing.T) {
	e := setup(t)
	e.must("note.create", map[string]any{"title": "Grace notes", "body": "Grace is favour."}, nil)
	var rep Report
	e.must("report.topic", map[string]any{"topic": "  Grace "}, &rep)
	want := "topical:topics dictionary:dict verses books notes"
	if strings.Join(ids(rep), " ") != want || rep.Title != "Topic report: Grace" {
		t.Fatalf("sections %v, want %s", ids(rep), want)
	}
	top := section(rep, "topical:topics")
	if len(top.Entries) != 2 || top.Entries[0].Heading != "GRACE" || top.Entries[1].Heading != "GRACE OF GOD" || top.Sources[0].Licence != "public_domain" {
		t.Fatalf("topical %+v", top)
	}
	links := top.Entries[0].Links
	got := []string{}
	for _, l := range links {
		got = append(got, l.Ref)
	}
	if strings.Join(got, " ") != "1Cor.1.4 John.1.17 John.1.18 Rom.5.2" {
		t.Errorf("topical references %v", got)
	}
	if m := top.Entries[0].Markup; !strings.Contains(m, `<r o="1Cor.1.4">1Co 1:4</r>; <r o="John.1.17">Joh 1:17</r>, <r o="John.1.18">18</r>`) {
		t.Errorf("markup %s", m)
	}
	if d := section(rep, "dictionary:dict"); d.Entries[0].Topic != "grace" {
		t.Errorf("dictionary %+v", d)
	}
	if v := section(rep, "verses"); len(v.Entries) != 0 && v.Entries[0].Ref == "" {
		t.Errorf("verses %+v", v)
	}
	if n := section(rep, "notes"); n.Entries[0].Heading != "Grace notes" {
		t.Errorf("notes %+v", n)
	}
}

func TestTopicVerses(t *testing.T) {
	e := setup(t)
	var rep Report
	e.must("report.topic", map[string]any{"topic": "god", "limit": 1, "sections": []string{"verses"}}, &rep)
	v := section(rep, "verses")
	if v == nil || len(v.Entries) != 1 || v.More != "verses@1" {
		t.Fatalf("verses %+v", rep.Sections)
	}
	e.must("report.topic", map[string]any{"topic": "god", "limit": 1, "cursor": v.More}, &rep)
	if v2 := section(rep, "verses"); v2 == nil || v2.Entries[0].Ref == v.Entries[0].Ref || v2.More != "verses@2" {
		t.Errorf("next page %+v", rep.Sections)
	}
}

func TestSaveAsNote(t *testing.T) {
	e := setup(t)
	var n core.Note
	e.must("report.save", map[string]any{"kind": "passage", "query": "Rom 8:28"}, &n)
	if n.Title != "Passage report: Romans 8:28" || n.Ref == nil || n.Ref.OSIS != "Rom.8.28" || len(n.Tags) != 1 || n.Tags[0] != "report" {
		t.Errorf("note %+v", n)
	}
	for _, want := range []string{"[Romans 8:28](#report/passage/Rom.8.28)", "## Cross-references", "[Gen 50:20](#passage/Gen.50.20)",
		"[ἀγαπῶσιν](#word/G25)", "[Lectures on Romans, Chapter Two](#book/item1?file=f1&para=1&spine=2)", "*From Greek NT (STEPBible.org, CC BY 4.0)*"} {
		if !strings.Contains(n.Body, want) {
			t.Errorf("note lacks %q:\n%s", want, n.Body)
		}
	}
	// No text of the works is copied: not the Bible verse, not the commentary.
	for _, text := range []string{"work together", "serve the good", "All things work together for"} {
		if strings.Contains(n.Body, text) {
			t.Errorf("note copies %q:\n%s", text, n.Body)
		}
	}
	e.must("report.save", map[string]any{"kind": "topic", "query": "grace", "title": "Grace study"}, &n)
	if n.Title != "Grace study" || n.Ref != nil || !strings.Contains(n.Body, "[GRACE](#topic/grace)") || strings.Contains(n.Body, "Favour") {
		t.Errorf("topic note %+v", n)
	}
	e.must("report.save", map[string]any{"kind": "word", "query": "G25"}, &n)
	if !strings.Contains(n.Body, "[G26 ἀγάπη](#word/G26)") {
		t.Errorf("word note %s", n.Body)
	}
}

// A report stops when its caller gives up, and leaves no goroutines behind (TestMain checks).
func TestCancel(t *testing.T) {
	e := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.r.Run(ctx, e.env, "report.passage", json.RawMessage(`{"ref":"Rom 8:28"}`))
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Errorf("cancelled report: %v", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	if _, err := e.r.Run(ctx, e.env, "report.word", json.RawMessage(`{"word":"G25"}`)); err == nil {
		t.Error("report after its deadline succeeded")
	}
}

func TestPager(t *testing.T) {
	pg, _ := newPager(Paging{Sections: []string{"commentary", "text:kjv"}})
	for id, want := range map[string]bool{"commentary:mhcc": true, "text:kjv": true, "text:web": false, "notes": false} {
		if pg.wants(id) != want {
			t.Errorf("wants(%s) = %v", id, !want)
		}
	}
	if !pg.wantsGroup("text") || pg.wantsGroup("notes") {
		t.Error("wantsGroup")
	}
	pg, _ = newPager(Paging{Cursor: "commentary:mhcc@40", Limit: 10})
	s := Section{ID: "commentary:mhcc"}
	pg.page(&s, make([]Entry, 55), 20)
	if len(s.Entries) != 10 || s.More != "commentary:mhcc@50" || s.Total != 55 || pg.wants("commentary:other") {
		t.Errorf("page %+v", s)
	}
}

func TestLinkRefs(t *testing.T) {
	m, links := linkRefs(`See <r>De 7:6-9</r>; <r>9:4-6</r>, <r o="Job.10.12">Job 10:12</r>; <r>22:2,3</r>; <r>Nowhere</r>`)
	got := []string{}
	for _, l := range links {
		got = append(got, l.Ref+"="+l.Label)
	}
	want := "Deut.7.6-Deut.7.9=De 7:6-9 Deut.9.4-Deut.9.6=9:4-6 Job.10.12=Job 10:12 Job.22.2=22:2 Job.22.3=3"
	if strings.Join(got, " ") != want {
		t.Errorf("links %v", got)
	}
	if !strings.Contains(m, `<r o="Job.22.2">22:2</r>,<r o="Job.22.3">3</r>`) || !strings.Contains(m, "; Nowhere") {
		t.Errorf("markup %s", m)
	}
	if n := relatedNumbers("from 25; see GREEK for 5368 see HEBREW for 0157 G25 G26b", "G25"); strings.Join(n, " ") != "G5368 H157 G26" {
		t.Errorf("related %v", n)
	}
}
