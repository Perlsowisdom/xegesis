package export_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/all"
	"github.com/Perlsowisdom/xegesis/internal/export"
	"github.com/Perlsowisdom/xegesis/internal/integrations/secrets"
	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// Fixtures: a few public-domain verses typed by hand, installed once as a public-domain Bible and
// once as a free-to-share one with a credit line, and a long "purchased" book.

const osisKJV = `<?xml version="1.0" encoding="UTF-8"?>
<osis xmlns="http://www.bibletechnologies.net/2003/OSIS/namespace">
<osisText osisIDWork="KJVtest" xml:lang="en">
<header><work osisWork="KJVtest"><title>King James Version (test)</title></work></header>
<div type="book" osisID="John"><chapter osisID="John.3">
<verse osisID="John.3.16">For God so loved the world, that he gave his only begotten Son.</verse>
<verse osisID="John.3.17">For God sent not his Son into the world to condemn the world.</verse>
<verse osisID="John.3.18">He that believeth on him is not condemned.</verse>
</chapter></div>
</osisText></osis>`

const usfmWEB = `\id JHN World English Bible test
\c 3
\p
\v 16 For God so loved the world, that he gave his one and only Son.
\v 17 For God didn't send his Son into the world to judge the world.
\v 18 He who believes in him is not judged.
`

const webCredit = "World English Bible test, CC BY 4.0, eBible.org"

type env struct {
	t     *testing.T
	data  string
	reg   *command.Registry
	env   *command.Env
	book  string // the purchased book's item ID
	paras []string
}

func setup(t *testing.T) *env {
	t.Helper()
	data, err := os.MkdirTemp("", "xegesis-export-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(data) })
	st, err := store.Open(filepath.Join(data, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &env{t: t, data: data, reg: all.Registry(data, secrets.NewMemory()), env: &command.Env{Store: st}}

	kjv := filepath.Join(data, "kjv.xml")
	web := filepath.Join(data, "web.usfm")
	os.WriteFile(kjv, []byte(osisKJV), 0o644)
	os.WriteFile(web, []byte(usfmWEB), 0o644)
	e.must("bible.install", map[string]any{"path": kjv, "id": "kjv", "abbreviation": "KJV", "licence_type": "public_domain"}, nil)
	e.must("bible.install", map[string]any{"path": web, "id": "web", "abbreviation": "WEB", "name": "World English Bible",
		"licence_type": "creative_commons", "spdx": "CC-BY-4.0", "attribution": webCredit}, nil)

	books := filepath.Join(data, "books")
	os.MkdirAll(books, 0o755)
	var md strings.Builder
	md.WriteString("# The Long Book\n\n")
	for i := 1; i <= 60; i++ {
		p := fmt.Sprintf("Paragraph %02d. ", i) + strings.Repeat("Grace and peace be multiplied to you. ", 26)
		p = p[:1000]
		e.paras = append(e.paras, p)
		md.WriteString(p + "\n\n")
	}
	os.WriteFile(filepath.Join(books, "long.md"), []byte(md.String()), 0o644)
	var f library.Folder
	e.must("library.add_folder", map[string]any{"path": books}, &f)
	e.must("library.scan", map[string]any{"folder_id": f.ID}, nil)
	var items struct {
		Items []library.ItemSummary `json:"items"`
	}
	e.must("library.items", map[string]any{}, &items)
	e.book = items.Items[0].ID
	e.must("library.set_licence", map[string]any{"item_ids": []string{e.book}, "type": "personal_purchase", "rights_holder": "Example Press"}, nil)
	return e
}

func (e *env) run(id string, params, out any) error {
	e.t.Helper()
	raw, _ := json.Marshal(params)
	res, err := e.reg.Run(context.Background(), e.env, id, raw)
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

func (e *env) must(id string, params, out any) {
	e.t.Helper()
	if err := e.run(id, params, out); err != nil {
		e.t.Fatalf("%s: %v", id, err)
	}
}

func (e *env) make(format string, sources []map[string]any, opts map[string]any) (export.MakeResult, []byte) {
	e.t.Helper()
	var res export.MakeResult
	e.must("export.make", map[string]any{"sources": sources, "format": format, "options": opts}, &res)
	data, err := os.ReadFile(res.Path)
	if err != nil {
		e.t.Fatal(err)
	}
	return res, data
}

var passage = []map[string]any{{"kind": "passage", "ref": "John 3:16-18", "modules": []string{"kjv", "web"}}}

func TestPassageInTwoBiblesEveryFormat(t *testing.T) {
	e := setup(t)
	want := []string{"loved the world, that he gave his only begotten Son", "his one and only Son", webCredit}

	_, md := e.make("md", passage, nil)
	for _, w := range append(want, "| KJV | WEB |", "## Credits") {
		if w == "## Credits" {
			continue
		}
		if !bytes.Contains(md, []byte(w)) {
			t.Errorf("Markdown lacks %q:\n%s", w, md)
		}
	}
	if !strings.HasSuffix(string(md), "\n") || strings.HasSuffix(string(md), "\n\n") {
		t.Errorf("Markdown must end with exactly one newline")
	}

	_, page := e.make("html", passage, map[string]any{"credits": "end"})
	for _, w := range append(want, `<table class="xd-par">`, "<h2>Credits</h2>") {
		if !bytes.Contains(page, []byte(w)) {
			t.Errorf("HTML lacks %q", w)
		}
	}

	res, docx := e.make("docx", passage, nil)
	if !strings.HasSuffix(res.Path, ".docx") {
		t.Errorf("path %s", res.Path)
	}
	doc := checkDOCX(t, docx)
	for _, w := range want {
		if !strings.Contains(doc, w) {
			t.Errorf("Word document lacks %q", w)
		}
	}

	// One after another: each Bible is its own piece with its own credit.
	_, md = e.make("md", passage, map[string]any{"layout": "one_after_another", "verse_numbers": false})
	if !bytes.Contains(md, []byte("## John 3:16-18 · WEB")) || bytes.Contains(md, []byte("**16**")) {
		t.Errorf("one after another:\n%s", md)
	}

	var pv export.Preview
	e.must("export.preview", map[string]any{"sources": passage, "format": "pdf"}, &pv)
	if len(pv.Warnings) > 0 && strings.Contains(pv.Warnings[0], "no font for PDF") {
		t.Skip("no TrueType fonts on this system for the PDF test")
	}
	// Side by side, the columns' lines interleave in the PDF's text, so check whole sentences one
	// Bible after another, and the side-by-side file for its heads and credit.
	_, pdfData := e.make("pdf", passage, map[string]any{"layout": "one_after_another"})
	text := pdfText(t, pdfData)
	for _, w := range want {
		if !strings.Contains(squash(text), squash(w)) {
			t.Errorf("PDF text lacks %q:\n%s", w, text)
		}
	}
	_, pdfData = e.make("pdf", passage, nil)
	text = squash(pdfText(t, pdfData))
	for _, w := range []string{"KJVWEB", squash(webCredit), "believethonhim"} {
		if !strings.Contains(text, w) {
			t.Errorf("side-by-side PDF text lacks %q:\n%s", w, text)
		}
	}
}

func squash(s string) string { return strings.Join(strings.Fields(s), "") }

// checkDOCX opens a .docx as Word would: a zip whose parts are all well-formed XML, with the parts
// the content types and relationships name. It returns the document's text.
func checkDOCX(t *testing.T, data []byte) string {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]bool{}
	var text strings.Builder
	for _, f := range z.File {
		parts[f.Name] = true
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		d := xml.NewDecoder(r)
		for {
			tok, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s is not well-formed XML: %v", f.Name, err)
			}
			if c, ok := tok.(xml.CharData); ok && f.Name == "word/document.xml" {
				text.Write(c)
			}
		}
		r.Close()
	}
	for _, p := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/styles.xml",
		"word/_rels/document.xml.rels", "word/header1.xml", "word/footer1.xml", "docProps/core.xml"} {
		if !parts[p] {
			t.Errorf("the .docx lacks %s", p)
		}
	}
	return text.String()
}

func pdfText(t *testing.T, data []byte) string {
	t.Helper()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("the PDF does not parse: %v", err)
	}
	if r.NumPage() < 1 {
		t.Fatal("the PDF has no pages")
	}
	var out strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		s, err := p.GetPlainText(nil)
		if err != nil {
			t.Fatalf("page %d: %v", i, err)
		}
		out.WriteString(s)
	}
	return out.String()
}

func TestPurchasedBookIsCutAndCountedPerDay(t *testing.T) {
	e := setup(t)
	book := []map[string]any{{"kind": "book", "item_id": e.book, "from": 1}}

	// 60 paragraphs of 1,000 characters (and a heading) are 30 pages; 10% is 3 pages, 6,000 characters, a day.
	var pv export.Preview
	e.must("export.preview", map[string]any{"sources": book, "format": "pdf"}, &pv)
	if len(pv.Pieces) != 1 || pv.Pieces[0].Cut == nil {
		t.Fatalf("no cut: %+v", pv)
	}
	cut := pv.Pieces[0].Cut
	// The heading and five paragraphs (5,013 characters) fit in 6,000; the sixth would not.
	want := "Personal copy: up to 10% of this book per day; 2.5 of 30 pages fit. The rest can be exported tomorrow."
	if cut.Reason != want || len(pv.Cuts) != 1 {
		t.Errorf("reason %q, want it to start %q", cut.Reason, want)
	}
	if cut.Rest == nil || cut.Rest.Skip != 6 || cut.Rest.ItemID != e.book {
		t.Errorf("rest %+v", cut.Rest)
	}
	if !strings.Contains(pv.Body, "Shortened.") || strings.Contains(pv.Body, "Paragraph 06.") {
		t.Errorf("the preview page must show the cut and stop before paragraph 6")
	}
	if !strings.Contains(pv.Body, "Example Press") {
		t.Errorf("the credit line is missing")
	}

	// Preview counts nothing; the export does, and the next export the same day gets nothing.
	_, md := e.make("md", book, nil)
	if !bytes.Contains(md, []byte("Paragraph 05.")) || bytes.Contains(md, []byte("Paragraph 06.")) || !bytes.Contains(md, []byte("> Shortened.")) {
		t.Errorf("first export:\n%s", md)
	}
	res, md := e.make("docx", book, nil)
	if res.Pieces[0].Cut == nil || !strings.Contains(res.Pieces[0].Cut.Reason, "less than 0.1 of its 30 pages fit") ||
		!strings.Contains(res.Pieces[0].Cut.Reason, "already exported or copied today") {
		t.Errorf("second export: %+v", res.Pieces[0].Cut)
	}
	if doc := checkDOCX(t, md); strings.Contains(doc, "Paragraph 01.") {
		t.Errorf("the second export must not include text past the daily limit")
	}

	// Copies share the export total.
	var cp struct {
		Allowed bool   `json:"allowed"`
		Reason  string `json:"reason"`
	}
	e.must("book.copy", map[string]any{"item_id": e.book, "text": e.paras[10]}, &cp)
	if cp.Allowed {
		t.Errorf("a copy after the day's exports should be refused")
	}

	// Printing has its own limit, counted separately.
	var pr export.Preview
	e.must("export.print", map[string]any{"sources": book}, &pr)
	if !strings.Contains(pr.HTML, "Paragraph 05.") || !strings.Contains(pr.HTML, "<!DOCTYPE html>") {
		t.Errorf("print should still allow today's print share")
	}
	e.must("export.print", map[string]any{"sources": book}, &pr)
	if pr.Pieces[0].Cut == nil || !strings.Contains(pr.Pieces[0].Cut.Reason, "already printed today") {
		t.Errorf("second print: %+v", pr.Pieces[0].Cut)
	}
	for _, a := range []string{"export", "print"} {
		var c int
		e.env.Store.QueryRow(context.Background(), `SELECT COUNT(*) FROM copy_log WHERE item_id = ? AND action = ?`, e.book, a).Scan(&c)
		if c != 2 {
			t.Errorf("copy_log has %d %s rows for the book, want 2", c, a)
		}
	}

	// The rest is ready for another day.
	e.must("export.preview", map[string]any{"sources": []any{cut.Rest}, "format": "print"}, &pv)
	if !strings.Contains(pv.Body, "already printed today") {
		t.Errorf("rest preview: %v", pv.Cuts)
	}
}

func TestSelectionWithinLimitIsWhole(t *testing.T) {
	e := setup(t)
	var pv export.Preview
	e.must("export.preview", map[string]any{"sources": []map[string]any{{"kind": "book", "item_id": e.book, "text": e.paras[0] + "\n" + e.paras[1]}}}, &pv)
	if pv.Pieces[0].Cut != nil || len(pv.Cuts) != 0 || !strings.Contains(pv.Body, "Paragraph 02.") {
		t.Errorf("a short selection must not be cut: %+v", pv.Pieces[0])
	}
	if pv.Pieces[0].Licence != "Personal copy" {
		t.Errorf("licence %q", pv.Pieces[0].Licence)
	}
}

func TestNotesListsAndPlansExport(t *testing.T) {
	e := setup(t)
	var note struct{ ID string }
	e.must("note.create", map[string]any{"title": "Love", "ref": "John 3:16",
		"body": "# Big idea\n\nGod **loves** the *world*.\n\n- given\n- believed\n\n> a quote", "tags": []string{"love"}}, &note)
	var list struct{ ID string }
	e.must("list.create", map[string]any{"kind": "verses", "name": "Gospel verses"}, &list)
	e.must("list.add", map[string]any{"list": list.ID, "ref": "John 3:16", "note": "memorise"}, nil)
	var pl struct{ ID string }
	e.must("plan.create", map[string]any{"name": "John in a week", "ranges": []string{"John"}, "days": 7, "start": "2026-11-02"}, &pl)

	sources := []map[string]any{
		{"kind": "note", "id": note.ID},
		{"kind": "list", "id": list.ID, "module": "web"},
		{"kind": "plan", "id": pl.ID},
	}
	_, md := e.make("md", sources, nil)
	for _, w := range []string{"## Love", "### Big idea", "God **loves** the *world*.", "- given\n- believed", "> a quote",
		"*Tags: love*", "## Gospel verses", "**John 3:16** For God so loved", "*memorise*", "## John in a week",
		"**Day 1 · Mon 2 Nov 2026**", webCredit} {
		if !bytes.Contains(md, []byte(w)) {
			t.Errorf("Markdown lacks %q:\n%s", w, md)
		}
	}
	_, docx := e.make("docx", sources, nil)
	doc := checkDOCX(t, docx)
	for _, w := range []string{"Big idea", "Gospel verses", "John in a week", "Day 7"} {
		if !strings.Contains(doc, w) {
			t.Errorf("Word document lacks %q", w)
		}
	}
	// Notes on a passage come with it when asked.
	_, md = e.make("md", passage, map[string]any{"notes": true})
	if !bytes.Contains(md, []byte("## Your notes on John 3:16-18")) || !bytes.Contains(md, []byte("God **loves**")) {
		t.Errorf("passage with notes:\n%s", md)
	}
}

func TestReports(t *testing.T) {
	e := setup(t)
	err := e.run("export.preview", map[string]any{"sources": []map[string]any{{"kind": "report", "report": "passage"}}}, nil)
	if ce, ok := err.(*command.Error); !ok || ce.Code != "unavailable" {
		t.Fatalf("without reports: %v", err)
	}
	// A stand-in with the shape reports are read by.
	type sec struct {
		Title       string           `json:"title"`
		Attribution string           `json:"attribution,omitempty"`
		Items       []map[string]any `json:"items"`
	}
	e.reg.Add(command.Define("report.passage", "stand-in", false, func(context.Context, *command.Env, struct {
		Ref string `json:"ref"`
	}) (map[string]any, error) {
		return map[string]any{"title": "Report on John 3:16", "sections": []sec{
			{Title: "Commentary", Attribution: "Matthew Henry (public domain)", Items: []map[string]any{{"ref": "John 3:16", "text": "The love of God."}}},
			{Title: "Citing books", Items: []map[string]any{{"title": "The Long Book", "item_id": e.book, "text": strings.Repeat("x", 7000)}}},
		}}, nil
	}))
	var pv export.Preview
	e.must("export.preview", map[string]any{"sources": []map[string]any{{"kind": "report", "report": "passage", "params": map[string]any{"ref": "John 3:16"}}}}, &pv)
	for _, w := range []string{"Report on John 3:16", "Commentary", "The love of God.", "Matthew Henry (public domain)", "Shortened."} {
		if !strings.Contains(pv.Body, w) {
			t.Errorf("report preview lacks %q", w)
		}
	}
}

func TestBadSources(t *testing.T) {
	e := setup(t)
	for _, src := range []map[string]any{{"kind": "passage", "ref": "John 3:16"}, {"kind": "nope"}, {"kind": "note"}, {"kind": "book", "item_id": e.book}} {
		err := e.run("export.preview", map[string]any{"sources": []any{src}}, nil)
		if ce, ok := err.(*command.Error); !ok || ce.Code != "bad_params" {
			t.Errorf("%v: %v", src, err)
		}
	}
	path := filepath.Join(e.data, "out.md")
	os.WriteFile(path, []byte("x"), 0o644)
	if err := e.run("export.make", map[string]any{"sources": passage, "format": "md", "path": path}, nil); err == nil {
		t.Errorf("an existing file must not be overwritten without replace")
	}
	e.must("export.make", map[string]any{"sources": passage, "format": "md", "path": path, "replace": true}, nil)
}
