package search

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

func passagesOf(t *testing.T, path, format string) []passage {
	t.Helper()
	ps, err := readBook(path, format)
	if err != nil {
		t.Fatalf("%s: %v", filepath.Base(path), err)
	}
	return ps
}

func find(ps []passage, word string) *passage {
	for i := range ps {
		if strings.Contains(ps[i].text, word) {
			return &ps[i]
		}
	}
	return nil
}

func TestExtractFormats(t *testing.T) {
	dir := t.TempDir()
	books(t, dir)
	cases := []struct {
		file, format, word, heading string
		loc                         Locator
	}{
		{"luther.epub", "epub", "righteous", "Chapter One",
			Locator{Spine: 1, Href: "text/ch%201.xhtml", CFI: "epubcfi(/6/2[c1]!)", Para: 2, Offset: 13}},
		{"luther.epub", "epub", "work together", "Chapter Two",
			Locator{Spine: 2, Href: "text/ch2.xhtml", CFI: "epubcfi(/6/4[c2]!)", Para: 2, Offset: 13}},
		{"sermon.docx", "docx", "suffering produces endurance", "Hope in Suffering", Locator{Para: 2, Offset: 19}},
		{"commentary.pdf", "pdf", "faith alone continues on this line.", "", Locator{Page: 1, Para: 1}},
		{"commentary.pdf", "pdf", "1 Cor. 13:4", "", Locator{Page: 2, Para: 1}},
		{"page.html", "html", "greater than all", "Amazing Grace", Locator{Para: 2, Offset: 15}},
		{"notes.md", "md", "pilgrim walks the narrow way, see Matthew 7:14.", "Pilgrim", Locator{Para: 2, Offset: 9}},
		{"notes.md", "md", "burden falls", "Burden", Locator{Para: 4, Offset: 70}},
		{"plain.txt", "txt", "Second paragraph", "", Locator{Para: 3, Offset: 61}},
	}
	for _, c := range cases {
		ps := passagesOf(t, filepath.Join(dir, c.file), c.format)
		p := find(ps, c.word)
		if p == nil {
			t.Errorf("%s: no passage with %q in %+v", c.file, c.word, ps)
			continue
		}
		if p.heading != c.heading || p.loc != c.loc {
			t.Errorf("%s %q: heading %q loc %+v, want %q %+v", c.file, c.word, p.heading, p.loc, c.heading, c.loc)
		}
	}
	html := passagesOf(t, filepath.Join(dir, "page.html"), "html")
	if find(html, "hidden") != nil || find(html, "Saved by grace") == nil {
		t.Errorf("html: %+v", html)
	}
	pdf := passagesOf(t, filepath.Join(dir, "commentary.pdf"), "pdf")
	if p := find(pdf, "new paragraph"); p == nil || !strings.Contains(p.text, "line.\n\nA new paragraph") {
		t.Errorf("pdf paragraphs not split: %+v", pdf)
	}
}

func TestChunking(t *testing.T) {
	long := strings.Repeat("A sentence of some length goes here. ", 100)
	short := []block{{text: "Heading", heading: true}, {text: "one"}, {text: "two"}, {text: "three"}, {text: long}}
	ps := chunk([]section{{blocks: short}})
	if len(ps) != 4 || ps[0].text != "one\n\ntwo" || !strings.HasPrefix(ps[1].text, "three\n\nA sentence") {
		t.Fatalf("passages: %d, first %q, second %.20q", len(ps), ps[0].text, ps[1].text)
	}
	for _, p := range ps[1:] {
		if len(p.text) > maxPassage+10 || len(p.text) < minPassage || p.heading != "Heading" {
			t.Errorf("long paragraph piece of %d chars, heading %q", len(p.text), p.heading)
		}
	}
	if ps[1].loc.Para != 4 || ps[2].loc.Para != 5 || ps[3].loc.Offset <= ps[2].loc.Offset {
		t.Errorf("locators %+v %+v %+v", ps[1].loc, ps[2].loc, ps[3].loc)
	}
}

func TestFindRefs(t *testing.T) {
	text := "See Rom 8:28, 30 and Romans 8:28-30; also 1 Cor. 13:4–7, First John 4:8, John 3:16; 4:2, " +
		"the Epistle of James 2:14, Song of Songs 2:1. Not refs: Chapter 10:30, Smith 3:4, Rom 99:1."
	want := []string{"Romans 8:28", "Romans 8:30", "Romans 8:28-30", "1 Corinthians 13:4-7", "1 John 4:8",
		"John 3:16", "John 4:2", "James 2:14", "Song of Songs 2:1"}
	got := []string{}
	for _, c := range findRefs(text) {
		got = append(got, c.r.String())
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("refs\n got %v\nwant %v", got, want)
	}
	r, _ := verse.Parse("Rom 8:29")
	if s := citeSnippet("Text citing Romans 8:28-30 here.", r); s != "Text citing <mark>Romans 8:28-30</mark> here." {
		t.Errorf("snippet %q", s)
	}
}
