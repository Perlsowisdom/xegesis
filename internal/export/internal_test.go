package export

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"
)

func TestNoteMarkdownReadsAndWritesBack(t *testing.T) {
	md := "# Title\n\nSome **bold** and *italic* and `code` and [a link](https://example.com).\nSecond line\n\n- one\n- two\n\n1. first\n2. second\n\n> quoted\n\nnot *closed and 2 * 3"
	bs := parseNote(md)
	kinds := []Kind{Heading, Para, Bullet, Bullet, Numbered, Numbered, Quote, Para}
	var got []Kind
	for _, b := range bs {
		got = append(got, b.Kind)
	}
	if !reflect.DeepEqual(got, kinds) {
		t.Fatalf("kinds %v, want %v", got, kinds)
	}
	if s := bs[1].Spans; !s[1].Bold || !s[3].Italic || !s[5].Code || s[7].Link != "https://example.com" || !s[9].Break {
		t.Errorf("spans %+v", s)
	}
	if spansText(bs[7].Spans) != "not *closed and 2 * 3" {
		t.Errorf("unmatched marks must stay text: %q", spansText(bs[7].Spans))
	}
	out := string(Markdown(Doc{Pieces: []Piece{{Blocks: bs}}}))
	for _, w := range []string{"## Title", "Some **bold** and *italic* and `code` and [a link](https://example.com).\nSecond line",
		"- one\n- two", "1. first\n2. second", "> quoted"} {
		if !strings.Contains(out, w) {
			t.Errorf("written Markdown lacks %q:\n%s", w, out)
		}
	}
}

func TestUnderlineAndVerseQuotations(t *testing.T) {
	bs := parseNote("Some <u>underlined</u> text\n\n> **28** And we know.\n>\n> — [Romans 8:28](#passage/Rom.8.28) (KJV)")
	if len(bs) != 2 || !bs[0].Spans[1].Under || bs[0].Spans[1].Text != "underlined" {
		t.Fatalf("underline %+v", bs)
	}
	if bs[1].Kind != Quote || !strings.Contains(spansText(bs[1].Spans), "— Romans 8:28 (KJV)") {
		t.Fatalf("an older quotation with an empty line is still a quote: %+v", bs[1])
	}
	if out := string(Markdown(Doc{Pieces: []Piece{{Blocks: bs[:1]}}})); !strings.Contains(out, "Some <u>underlined</u> text") {
		t.Errorf("markdown %q", out)
	}
	if out := inlineHTML(bs[0].Spans); !strings.Contains(out, "<u>underlined</u>") {
		t.Errorf("html %q", out)
	}
}

func TestHTMLEscapesEverything(t *testing.T) {
	d := Doc{Title: `<script>alert(1)</script>`, Pieces: []Piece{{Title: "A & B", Blocks: []Block{
		{Kind: Para, Spans: []Span{{Text: `<img src=x onerror=1>`, Link: "javascript:alert(1)"}, {Text: "ok", Link: "https://x.org/?a=1&b=2"}}},
	}, Credits: []string{`"quoted" <credit>`}}}}
	h := HTML(d)
	for _, bad := range []string{"<script>", "<img", "javascript:"} {
		if strings.Contains(h, bad) {
			t.Errorf("HTML keeps %q", bad)
		}
	}
	if !strings.Contains(h, `<a href="https://x.org/?a=1&amp;b=2">ok</a>`) || !strings.Contains(h, "&#34;quoted&#34; &lt;credit&gt;") {
		t.Errorf("HTML:\n%s", h)
	}
	if css := CSS(Options{}, `Say "hi" \ <b>`); !strings.Contains(css, `content: "Say \"hi\" \\ \3c b\3e "`) {
		t.Errorf("header string not quoted: %s", css)
	}
}

func TestReverseClustersKeepsMarksWithLetters(t *testing.T) {
	in := []styledRune{{r: 'ב'}, {r: '\u05B0'}, {r: 'ר'}, {r: 'א'}}
	var got []rune
	for _, r := range reverseClusters(in) {
		got = append(got, r.r)
	}
	if string(got) != "ארב\u05B0" {
		t.Errorf("got %q", string(got))
	}
}

func TestPDFFontsAreSubsetAndHebrewIsShown(t *testing.T) {
	fs, err := systemFonts()
	if err != nil {
		t.Skip(err)
	}
	f := fs.styles[0]
	g, ok := f.glyph('A')
	if !ok {
		t.Fatal("no A in the regular font")
	}
	sub := f.subset(map[uint16]bool{g: true})
	if len(sub) >= len(f.data) {
		t.Errorf("subset is %d bytes, the font %d", len(sub), len(f.data))
	}
	back, err := parseTTF("sub", sub)
	if err != nil {
		t.Fatalf("subset does not parse: %v", err)
	}
	if len(back.glyphData(int(g))) == 0 || back.numGlyphs != f.numGlyphs {
		t.Errorf("subset lost the glyph")
	}

	d := Doc{Title: "Genesis 1:1", Pieces: []Piece{{Title: "Hebrew", Blocks: []Block{
		{Kind: Verse, Num: "1", Spans: plain("בְּרֵאשִׁית בָּרָא אֱלֹהִים")},
		{Kind: Para, Spans: plain("Ἐν ἀρχῇ ἦν ὁ λόγος")},
	}}}}
	data, warnings, err := PDF(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) > 0 {
		t.Logf("warnings: %v", warnings)
	}
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	text, _ := r.Page(1).GetPlainText(nil)
	if !strings.Contains(strings.ReplaceAll(text, " ", ""), "Ἐνἀρχῇἦνὁλόγος") {
		t.Errorf("Greek missing from the PDF text: %q", text)
	}
	if !strings.ContainsRune(text, 'א') {
		t.Errorf("Hebrew missing from the PDF text: %q", text)
	}
}

func TestLongDocumentsBreakPages(t *testing.T) {
	if _, err := systemFonts(); err != nil {
		t.Skip(err)
	}
	var rows []GridRow
	var verses []Block
	for i := 1; i <= 120; i++ {
		txt := strings.Repeat("word ", 40)
		rows = append(rows, GridRow{Num: "1", Cells: [][]Span{plain(txt), plain(txt + txt)}})
		verses = append(verses, Block{Kind: Verse, Num: "2", Spans: plain(txt)})
	}
	verses = append(verses, Block{Kind: Para, Spans: plain(strings.Repeat("x", 3000))}) // one unbreakable word
	d := Doc{Title: "Long", Pieces: []Piece{{Title: "Grid", Blocks: []Block{{Kind: Table, Table: &Grid{Head: []string{"A", "B"}, Rows: rows}}}},
		{Title: "Verses", Blocks: verses}}}
	data, _, err := PDF(d)
	if err != nil {
		t.Fatal(err)
	}
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if r.NumPage() < 10 {
		t.Errorf("%d pages", r.NumPage())
	}
}

// Sections as report.* returns them: entries in "entries", credits under "sources", a chart, and
// book quotes whose item ID sits in a book link.
func TestReportShape(t *testing.T) {
	sec := map[string]any{
		"title":   "Citing books",
		"sources": []any{map[string]any{"name": "KJV", "attribution": "Public domain"}, map[string]any{"name": "WEB", "attribution": "WEB, CC BY 4.0"}},
		"chart":   []any{map[string]any{"label": "Matt", "count": 3}},
		"entries": []any{
			map[string]any{"ref": "Rom.8.28", "ref_label": "Romans 8:28", "snippet": "all things <mark>work</mark> together"},
			map[string]any{"heading": "A book", "text": "quoted", "book": map[string]any{"item_id": "B1"}},
		},
	}
	es := entriesOf(sec)
	if len(es) != 2 {
		t.Fatalf("entries = %d, want 2 (sources and chart are not entries)", len(es))
	}
	if got := firstString(es[0], entryTitleKeys); got != "Romans 8:28" {
		t.Errorf("entry title = %q, want the readable reference", got)
	}
	if got := noMarks.Replace(firstString(es[0], entryTextKeys)); got != "all things work together" {
		t.Errorf("snippet = %q", got)
	}
	if itemIDOf(es[1]) != "B1" {
		t.Error("a book quote's item ID was not found, so its licence would not be charged")
	}
	if c := sectionCredits(sec); len(c) != 2 || c[1] != "WEB, CC BY 4.0" {
		t.Errorf("credits = %v", c)
	}
}

func TestCheckListsAndDividers(t *testing.T) {
	bs := parseNote("- [ ] read\n- [x] pray\n\n---\n\nAfter")
	if len(bs) != 4 || bs[0].Check != "todo" || bs[1].Check != "done" || bs[2].Kind != Rule || spansText(bs[0].Spans) != "read" {
		t.Fatalf("blocks %+v", bs)
	}
	d := Doc{Pieces: []Piece{{Blocks: bs}}}
	if out := string(Markdown(d)); !strings.Contains(out, "- [ ] read\n- [x] pray\n\n---\n\nAfter") {
		t.Errorf("markdown %q", out)
	}
	if h := HTML(d); !strings.Contains(h, "☐ read") || !strings.Contains(h, "☑ pray") || !strings.Contains(h, "<hr>") || strings.Contains(h, ">---<") {
		t.Errorf("html %s", h)
	}
}

// Reference chips, note links and highlight quotations are links and quotes in a note's Markdown.
// Exported, a chip keeps its words (an app address is not a web link) and the Markdown keeps the link.
func TestChipsAndQuotationsExport(t *testing.T) {
	md := "See [Romans 8:28](#passage/Rom.8.28) and [My study](#notes/note/01AB).\n\n> Words from a book\n> — [A book, page 3](#book/01CD?page=3)"
	bs := parseNote(md)
	d := Doc{Pieces: []Piece{{Blocks: bs}}}
	if out := string(Markdown(d)); !strings.Contains(out, md) {
		t.Errorf("markdown %q", out)
	}
	h := HTML(d)
	for _, w := range []string{"See Romans 8:28 and My study.", "— A book, page 3"} {
		if !strings.Contains(h, w) {
			t.Errorf("html lacks %q:\n%s", w, h)
		}
	}
	if strings.Contains(h, "#passage") || strings.Contains(h, "#notes") {
		t.Errorf("app addresses leaked into HTML:\n%s", h)
	}
}

// Tables in notes are GitHub-style Markdown with an optional comment for widths and a missing
// header row; they round-trip through export and show in HTML, Word and PDF.
func TestNoteTablesRoundTripAndExport(t *testing.T) {
	md := "<!-- table widths=120,60 -->\n| Point | Verse |\n| --- | --- |\n| **Grace** | [Romans 8:28](#passage/Rom.8.28) |\n| a \\| b | one<br>two |"
	bs := parseNote(md)
	if len(bs) != 1 || bs[0].Kind != Table || !bs[0].Table.Plain || len(bs[0].Table.Rows) != 2 || len(bs[0].Table.Widths) != 2 {
		t.Fatalf("blocks %+v", bs)
	}
	if c := bs[0].Table.Rows[1].Cells[0]; spansText(c) != "a | b" {
		t.Errorf("escaped pipe: %+v", c)
	}
	d := Doc{Pieces: []Piece{{Blocks: bs}}}
	if out := string(Markdown(d)); !strings.Contains(out, md) {
		t.Errorf("markdown %q", out)
	}
	h := HTML(d)
	for _, w := range []string{`<table class="xd-par xd-table">`, `<col style="width:66.7%">`, "<th dir=\"auto\">Point</th>", "<strong>Grace</strong>", "one<br>two"} {
		if !strings.Contains(h, w) {
			t.Errorf("html lacks %q:\n%s", w, h)
		}
	}
	if strings.Contains(h, `class="xd-n"`) {
		t.Errorf("a note table has no verse-number column:\n%s", h)
	}
	if _, err := DOCX(d); err != nil {
		t.Fatal(err)
	}
	if _, _, err := PDF(d); err != nil {
		t.Fatal(err)
	}
	noHead := "<!-- table header=no -->\n| a | b |\n| --- | --- |\n| c | d |"
	d = Doc{Pieces: []Piece{{Blocks: parseNote(noHead)}}}
	if out := string(Markdown(d)); !strings.Contains(out, noHead) {
		t.Errorf("markdown %q", out)
	}
	if h := HTML(d); strings.Contains(h, "<thead>") {
		t.Errorf("no header row expected:\n%s", h)
	}
}
