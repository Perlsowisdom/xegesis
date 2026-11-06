package reader

import "testing"

func bookList(e *env, item string) []Highlight {
	var list []Highlight
	e.must("highlight.list", map[string]any{"item_id": item}, &list)
	return list
}

func TestBookHighlightOverlapSplits(t *testing.T) {
	e := setup(t)
	book := e.items["epub"]
	words := "things work together for good"
	var old, mid, over Highlight
	e.must("highlight.create", map[string]any{"item_id": book, "style": "green", "locator": Anchor{Section: 2, Para: 2, Offset: 0, EndPara: 2, EndOffset: len(words), Text: words}}, &old)
	// A new highlight inside the old one leaves two green pieces either side of it.
	e.must("highlight.create", map[string]any{"item_id": book, "style": "pink", "locator": Anchor{Section: 2, Para: 2, Offset: 7, EndPara: 2, EndOffset: 11, Text: "work"}}, &mid)
	list := bookList(e, book)
	if len(list) != 3 {
		t.Fatalf("after splitting: %+v", list)
	}
	want := []struct {
		style, text string
		a, b        int
	}{{"green", "things", 0, 6}, {"pink", "work", 7, 11}, {"green", "together for good", 12, len(words)}}
	for i, w := range want {
		l := list[i].Locator
		if list[i].Style != w.style || l.Text != w.text || l.Offset != w.a || l.EndOffset != w.b {
			t.Errorf("piece %d: %s %+v, want %+v", i, list[i].Style, l, w)
		}
	}
	// A highlight over the left piece's end and the pink one takes them over; only the cut is left.
	e.must("highlight.create", map[string]any{"item_id": book, "style": "blue", "locator": Anchor{Section: 2, Para: 2, Offset: 3, EndPara: 2, EndOffset: 11, Text: "ngs work"}}, &over)
	list = bookList(e, book)
	if len(list) != 3 || list[0].Style != "green" || list[0].Locator.EndOffset != 3 || list[0].Locator.Text != "thi" || list[1].Style != "blue" {
		t.Fatalf("after covering: %+v", list)
	}
	// Covering everything in the paragraph leaves only the new highlight.
	e.must("highlight.create", map[string]any{"item_id": book, "style": "yellow", "locator": Anchor{Section: 2, Para: 2, Offset: 0, EndPara: 2, EndOffset: 40, Text: words}}, nil)
	if list = bookList(e, book); len(list) != 1 || list[0].Style != "yellow" {
		t.Fatalf("after covering all: %+v", list)
	}
}

func TestBookHighlightOverlapAcrossParagraphsAndSections(t *testing.T) {
	e := setup(t)
	book := e.items["epub"]
	e.must("highlight.create", map[string]any{"item_id": book, "style": "green", "locator": Anchor{Section: 2, Para: 1, Offset: 5, EndPara: 3, EndOffset: 4, Text: "long words"}}, nil)
	// Another section is never touched.
	e.must("highlight.create", map[string]any{"item_id": book, "style": "pink", "locator": Anchor{Section: 1, Para: 1, Offset: 0, EndPara: 3, EndOffset: 9, Text: "elsewhere"}}, nil)
	e.must("highlight.create", map[string]any{"item_id": book, "style": "blue", "locator": Anchor{Section: 2, Para: 2, Offset: 0, EndPara: 2, EndOffset: 3, Text: "mid"}}, nil)
	var greens, pinks, blues int
	for _, h := range bookList(e, book) {
		switch h.Style {
		case "green":
			greens++
		case "pink":
			pinks++
		case "blue":
			blues++
		}
	}
	if greens != 2 || pinks != 1 || blues != 1 {
		t.Fatalf("green %d, pink %d, blue %d: %+v", greens, pinks, blues, bookList(e, book))
	}
	// Touching without sharing a character changes nothing.
	e.must("highlight.create", map[string]any{"item_id": book, "style": "yellow", "locator": Anchor{Section: 2, Para: 3, Offset: 4, EndPara: 3, EndOffset: 8, Text: "next"}}, nil)
	if n := len(bookList(e, book)); n != 5 {
		t.Fatalf("%d highlights, want 5", n)
	}
}
