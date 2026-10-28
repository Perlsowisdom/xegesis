package reader

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/search"
)

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

// The paragraph markers must number paragraphs exactly as the search index does, or search hits
// and highlights would land on the wrong paragraph.
func TestMarkersMatchSearchParagraphs(t *testing.T) {
	e := setup(t)
	for _, format := range []string{"epub", "html", "docx", "md"} {
		path := filepath.Join(e.books, map[string]string{"epub": "luther.epub", "html": "page.html",
			"docx": "sermon.docx", "md": "notes.md"}[format])
		secs, err := search.ReadSections(path, format)
		if err != nil {
			t.Fatal(err)
		}
		for i, sec := range secs {
			var got Section
			e.must("book.section", map[string]any{"item_id": e.items[format], "section": i + 1}, &got)
			paras := paraTexts(t, got.HTML)
			if len(paras) != len(sec.Blocks) || got.Paras != len(sec.Blocks) {
				t.Fatalf("%s section %d: %d markers (%d), search has %d blocks\n%s", format, i+1, len(paras), got.Paras, len(sec.Blocks), got.HTML)
			}
			for j, b := range sec.Blocks {
				if paras[j] != norm(b.Text) {
					t.Errorf("%s section %d para %d: %q, search has %q", format, i+1, j+1, paras[j], b.Text)
				}
			}
		}
	}
}

func TestSanitisedContent(t *testing.T) {
	e := setup(t)
	var ch Section
	e.must("book.section", map[string]any{"item_id": e.items["epub"], "section": 1}, &ch)
	for _, bad := range []string{"<script", "alert", "<style", "color:red", "onclick", "onload", "style=", "<link",
		"example.com/x.css", "track.png", "<iframe", "<svg", "svg text", "javascript:", "href=\"https:"} {
		if strings.Contains(ch.HTML, bad) {
			t.Errorf("chapter keeps %q:\n%s", bad, ch.HTML)
		}
	}
	for _, want := range []string{
		`<img alt="dot" src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(png) + `">`,
		`<h1 id="bk-start">`,
		`<a href="#" data-section="2" data-anchor="bk-grace">the next chapter</a>`,
		`<a class="external" data-url="https://example.com/">a web link</a>`,
		`<a href="#" class="vref" data-ref="Rom.1.17">Rom 1:17</a>`,
		`<em>paragraph</em>`,
	} {
		if !strings.Contains(ch.HTML, want) {
			t.Errorf("chapter lacks %s:\n%s", want, ch.HTML)
		}
	}
	if ch.Title != "Chapter One" || ch.CFI != "epubcfi(/6/2[c1]!)" || ch.Sections != 2 {
		t.Errorf("section %+v", ch)
	}

	var page Section
	e.must("book.section", map[string]any{"item_id": e.items["html"], "section": 1}, &page)
	if strings.Contains(page.HTML, "win.ini") || strings.Contains(page.HTML, "hidden") || strings.Contains(page.HTML, "onclick") ||
		!strings.Contains(page.HTML, `src="data:image/png;base64,`) || !strings.Contains(page.HTML, `data-anchor="bk-top"`) ||
		!strings.Contains(page.HTML, `data-ref="John.1.16"`) {
		t.Errorf("html page:\n%s", page.HTML)
	}
	var doc Section
	e.must("book.section", map[string]any{"item_id": e.items["docx"], "section": 1}, &doc)
	if !strings.Contains(doc.HTML, "&lt;script&gt;") || !strings.Contains(doc.HTML, `data-ref="John.3.16"`) {
		t.Errorf("docx:\n%s", doc.HTML)
	}
}

func TestOpenEachFormat(t *testing.T) {
	e := setup(t)
	var b Book
	e.must("book.open", map[string]any{"item_id": e.items["epub"]}, &b)
	want := []TOCEntry{{Title: "Chapter One", Section: 1, Level: 1}, {Title: "On grace", Section: 2, Anchor: "bk-grace", Level: 2}}
	if b.Format != "epub" || !b.Reflowable || b.Sections != 2 || len(b.TOC) != 2 || b.TOC[0] != want[0] || b.TOC[1] != want[1] {
		t.Errorf("epub %+v", b)
	}
	if b.Licence != "personal_purchase" || b.Position != nil || len(b.Authors) != 1 {
		t.Errorf("epub licence/position/authors %+v", b)
	}

	e.must("book.open", map[string]any{"item_id": e.items["pdf"]}, &b)
	if b.Format != "pdf" || b.Reflowable || b.Sections != 2 {
		t.Errorf("pdf %+v", b)
	}
	var page Section
	e.must("book.section", map[string]any{"item_id": e.items["pdf"], "section": 2}, &page)
	if page.Width != 612 || page.Height != 792 || page.Sections != 2 || page.HTML != "" {
		t.Errorf("pdf page %+v", page)
	}
	e.fails("book.section", map[string]any{"item_id": e.items["pdf"], "section": 3}, "not_found")

	e.must("book.open", map[string]any{"item_id": e.items["docx"]}, &b)
	if b.Format != "docx" || b.Sections != 1 || len(b.TOC) != 1 || b.TOC[0].Anchor != "bp-1" || b.TOC[0].Title != "Hope in Suffering" {
		t.Errorf("docx %+v", b)
	}
	e.fails("book.section", map[string]any{"item_id": e.items["docx"], "section": 2}, "not_found")
	e.fails("book.open", map[string]any{"item_id": "nope"}, "not_found")
	e.fails("book.open", map[string]any{"item_id": e.items["epub"], "file_id": "nope"}, "not_found")
}

func TestPDFBytes(t *testing.T) {
	e := setup(t)
	var got []byte
	offset := 0
	for {
		var c FileChunk
		e.must("book.file", map[string]any{"item_id": e.items["pdf"], "offset": offset, "length": 100}, &c)
		b, _ := base64.StdEncoding.DecodeString(c.Data)
		got = append(got, b...)
		offset += len(b)
		if c.EOF {
			break
		}
	}
	if !bytes.Equal(got, testPDF) {
		t.Fatalf("got %d bytes, want %d", len(got), len(testPDF))
	}
	e.fails("book.file", map[string]any{"item_id": e.items["epub"]}, "bad_params")
	e.fails("book.file", map[string]any{"item_id": e.items["pdf"], "offset": len(testPDF) + 1}, "bad_params")
}

func TestSearchHitOpensAtItsPlace(t *testing.T) {
	e := setup(t)
	cases := []struct {
		query, format string
		section       int
	}{
		{"sanctification", "epub", 2},
		{"propitiation", "pdf", 2},
		{"endurance", "docx", 1},
	}
	for _, c := range cases {
		var res search.QueryResult
		e.must("search.query", map[string]any{"query": c.query}, &res)
		if len(res.Hits) != 1 {
			t.Fatalf("%s: hits %+v", c.query, res.Hits)
		}
		h := res.Hits[0]
		var loc Location
		e.must("book.locate", map[string]any{"item_id": h.ItemID, "file_id": h.FileID, "locator": h.Locator}, &loc)
		if loc.Section != c.section || loc.FileID != h.FileID {
			t.Fatalf("%s: located %+v from %+v", c.query, loc, h.Locator)
		}
		if c.format == "pdf" {
			continue
		}
		var sec Section
		e.must("book.section", map[string]any{"item_id": h.ItemID, "section": loc.Section}, &sec)
		// A hit's locator is where its passage starts; a passage is a paragraph or two.
		at := strings.Index(sec.HTML, `id="`+loc.Anchor+`"`)
		if loc.Para != h.Locator.Para || at < 0 || !strings.Contains(sec.HTML[at:], c.query) {
			t.Errorf("%s: %s is not where the hit's passage starts:\n%s", c.query, loc.Anchor, sec.HTML)
		}
		if after := strings.Index(sec.HTML, fmt.Sprintf(`id="bp-%d"`, loc.Para+2)); after >= 0 && after < strings.Index(sec.HTML, c.query) {
			t.Errorf("%s: the hit is more than a passage after %s", c.query, loc.Anchor)
		}
	}
	// A locator whose spine number is stale is found again by its href.
	var loc Location
	e.must("book.locate", map[string]any{"item_id": e.items["epub"], "locator": search.Locator{Spine: 9, Href: "text/ch2.xhtml", Para: 2}}, &loc)
	if loc.Section != 2 || loc.Anchor != "bp-2" {
		t.Errorf("by href: %+v", loc)
	}
	e.fails("book.locate", map[string]any{"item_id": e.items["epub"], "locator": search.Locator{Spine: 9}}, "not_found")
	e.fails("book.locate", map[string]any{"item_id": e.items["pdf"], "locator": search.Locator{Page: 7}}, "not_found")
}

func TestHighlightsNotesAndPositionSurviveRestart(t *testing.T) {
	e := setup(t)
	book := e.items["epub"]
	var h Highlight
	e.must("highlight.create", map[string]any{"item_id": book, "style": "green", "locator": Anchor{Section: 2, CFI: "epubcfi(/6/4[c2]!)",
		Para: 2, Offset: 4, EndPara: 2, EndOffset: 20, Text: "things work together"}}, &h)
	var pdfH Highlight
	e.must("highlight.create", map[string]any{"item_id": e.items["pdf"], "locator": Anchor{Section: 1, Page: 1,
		Rects: [][4]float64{{0.1, 0.1, 0.5, 0.02}}, Text: "Justification"}}, &pdfH)
	var verseH Highlight
	e.must("highlight.create", map[string]any{"ref": "Rom 8:28"}, &verseH)
	var n core.Note
	e.must("note.create", map[string]any{"title": "On grace", "body": "Compare Eph 2.", "item_id": book,
		"locator": map[string]any{"section": 2, "para": 1, "offset": 0, "text": "All things"}}, &n)
	e.must("book.set_position", map[string]any{"item_id": book, "section": 2, "para": 1}, nil)
	e.must("book.set_position", map[string]any{"item_id": book, "section": 2, "para": 2}, nil)

	e.fails("highlight.create", map[string]any{"item_id": book}, "bad_params")
	e.fails("highlight.create", map[string]any{"item_id": book, "locator": Anchor{Section: 1}}, "bad_params")
	e.fails("highlight.create", map[string]any{"item_id": "nope", "locator": Anchor{Section: 1, Para: 1}}, "not_found")
	e.fails("highlight.create", map[string]any{"item_id": book, "locator": Anchor{Section: 1, Para: 1}, "style": "plaid"}, "bad_params")
	e.fails("note.create", map[string]any{"body": "x", "item_id": book}, "bad_params")
	e.fails("note.create", map[string]any{"body": "x", "item_id": book, "locator": map[string]any{"para": 1}}, "bad_params")

	e.st.Close()
	e.openStore()

	var list []Highlight
	e.must("highlight.list", map[string]any{"item_id": book, "section": 2}, &list)
	if len(list) != 1 || list[0].ID != h.ID || list[0].Style != "green" || list[0].Locator.Text != "things work together" ||
		list[0].Locator.EndOffset != 20 {
		t.Fatalf("after restart %+v", list)
	}
	e.must("highlight.list", map[string]any{"item_id": book, "section": 1}, &list)
	if len(list) != 0 {
		t.Errorf("section 1: %+v", list)
	}
	e.must("highlight.list", map[string]any{"item_id": e.items["pdf"]}, &list)
	if len(list) != 1 || list[0].Locator.Page != 1 || list[0].Locator.Rects[0][2] != 0.5 {
		t.Errorf("pdf highlights %+v", list)
	}
	e.must("highlight.list", map[string]any{"ref": "Rom 8"}, &list)
	if len(list) != 1 || list[0].ID != verseH.ID || list[0].Ref == "" {
		t.Errorf("verse highlights %+v", list)
	}
	var notes []core.Note
	e.must("note.list", map[string]any{"item_id": book, "section": 2}, &notes)
	if len(notes) != 1 || notes[0].ID != n.ID || notes[0].ItemID != book || notes[0].Locator["text"] != "All things" {
		t.Fatalf("notes after restart %+v", notes)
	}
	var b Book
	e.must("book.open", map[string]any{"item_id": book}, &b)
	if b.Position == nil || b.Position.Section != 2 || b.Position.Para != 2 {
		t.Errorf("position %+v", b.Position)
	}

	e.must("highlight.delete", map[string]any{"id": h.ID}, nil)
	e.fails("highlight.delete", map[string]any{"id": h.ID}, "not_found")
	e.must("highlight.list", map[string]any{"item_id": book}, &list)
	if len(list) != 0 {
		t.Errorf("after delete %+v", list)
	}
}

func TestCopyFollowsTheLicence(t *testing.T) {
	e := setup(t)
	book := e.items["epub"]
	e.must("library.set_licence", map[string]any{"item_ids": []string{book}, "type": "personal_purchase",
		"rights_holder": "Example Press", "export_limit": map[string]any{"pages": 1}}, nil)
	day := "2026-10-27"
	e.svc.today = func() string { return day }
	text := func(n int) string { return strings.Repeat("a", n) }

	var r CopyResult
	e.must("book.copy", map[string]any{"item_id": book, "text": text(1500)}, &r)
	if !r.Allowed || r.Text != text(1500)+"\n\n© Example Press" || r.CopiedToday != 1500 || r.DailyLimit != charsPerPage {
		t.Fatalf("first copy %+v", r)
	}
	e.must("book.copy", map[string]any{"item_id": book, "text": text(600)}, &r)
	if r.Allowed || r.Text != "" || !strings.Contains(r.Reason, "a day") || r.CopiedToday != 1500 {
		t.Fatalf("over the daily limit %+v", r)
	}
	e.must("book.copy", map[string]any{"item_id": book, "text": text(500)}, &r)
	if !r.Allowed || r.CopiedToday != 2000 {
		t.Fatalf("up to the limit %+v", r)
	}
	e.must("book.copy", map[string]any{"item_id": book, "text": text(2500)}, &r)
	if r.Allowed || !strings.Contains(r.Reason, "too long") {
		t.Fatalf("one copy over the limit %+v", r)
	}
	day = "2026-10-28"
	e.must("book.copy", map[string]any{"item_id": book, "text": text(10)}, &r)
	if !r.Allowed || r.CopiedToday != 10 {
		t.Fatalf("next day %+v", r)
	}

	// Copies are recorded writes, so the day's total survives a restart.
	e.st.Close()
	e.openStore()
	e.svc.today = func() string { return day }
	e.must("book.copy", map[string]any{"item_id": book, "text": text(1995)}, &r)
	if r.Allowed {
		t.Fatalf("after restart %+v", r)
	}
	var changes int
	e.st.QueryRow(t.Context(), `SELECT COUNT(*) FROM changes WHERE command = 'book.copy'`).Scan(&changes)
	if changes != 3 {
		t.Errorf("%d book.copy changes, want 3", changes)
	}

	// A percentage limit uses the book's length: the PDF has 2 pages, so 10% is 1 page.
	pdfBook := e.items["pdf"]
	e.must("book.copy", map[string]any{"item_id": pdfBook, "text": text(2001)}, &r)
	if r.Allowed || r.DailyLimit != charsPerPage {
		t.Fatalf("undecided licence %+v", r)
	}

	// Public domain is not limited and needs no credit.
	e.must("library.set_licence", map[string]any{"item_ids": []string{pdfBook}, "type": "public_domain"}, nil)
	e.must("book.copy", map[string]any{"item_id": pdfBook, "text": text(50000)}, &r)
	if !r.Allowed || r.Text != text(50000) || r.DailyLimit != 0 {
		t.Fatalf("public domain %+v", r.Allowed)
	}
}

func TestOfflineBook(t *testing.T) {
	e := setup(t)
	if err := os.Rename(e.books, e.books+"-moved"); err != nil {
		t.Skip("cannot move the folder:", err)
	}
	defer os.Rename(e.books+"-moved", e.books)
	e.fails("book.open", map[string]any{"item_id": e.items["epub"]}, "unavailable")
}
