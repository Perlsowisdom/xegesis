package reader

import (
	"testing"
)

const rom828, rom829 = 45008028, 45008029

func rng(v1 int64, o1 int, v2 int64, o2 int) map[string]any {
	return map[string]any{"start": map[string]any{"verse": v1, "offset": o1}, "end": map[string]any{"verse": v2, "offset": o2}}
}

func TestHighlightRangeValidation(t *testing.T) {
	e := setup(t)
	ok := map[string]any{"range": rng(rom828, 4, rom828, 11), "module": "KJV", "parts": []QuotePart{{Verse: rom828, Text: "we know"}}}
	var h Highlight
	e.must("highlight.create", ok, &h)
	if h.Range == nil || h.Module != "KJV" || h.Text != "we know" || h.Start != rom828 || h.End != rom828 || h.Ref != "Romans 8:28" {
		t.Fatalf("ranged highlight = %+v", h)
	}
	bad := []map[string]any{
		{"range": rng(rom828, 11, rom828, 4), "module": "KJV"},                   // backwards
		{"range": rng(rom828, 4, rom828, 4), "module": "KJV"},                    // empty
		{"range": rng(rom828, -1, rom828, 4), "module": "KJV"},                   // negative
		{"range": rng(rom828, 0, rom828, 4)},                                     // no module
		{"range": rng(1, 0, rom828, 4), "module": "KJV"},                         // not a verse
		{"range": rng(rom828, 0, 46001001, 4), "module": "KJV"},                  // two books
		{"range": rng(rom828, 0, rom828, 4), "module": "KJV", "ref": "Rom 8:29"}, // verses differ
		{"range": rng(rom828, 0, rom828, 4), "module": "KJV", "parts": []QuotePart{{Verse: rom829, Text: "x"}}},
		{"range": rng(rom828, 0, rom828, 4), "module": "KJV", "versification": "hebrew"},
		{"ref": "Rom 8:28", "module": "KJV"}, // module without a range
	}
	for _, p := range bad {
		e.fails("highlight.create", p, "bad_params")
	}
	// The matching verses may be given with the range.
	e.must("highlight.create", map[string]any{"range": rng(rom829, 0, rom829, 3), "module": "KJV", "ref": "Rom 8:29", "quote": "For"}, &h)
	if h.Text != "For" {
		t.Errorf("quote = %q", h.Text)
	}
	e.fails("highlight.update", map[string]any{"id": h.ID}, "bad_params")
	e.fails("highlight.update", map[string]any{"id": h.ID, "range": rng(rom829, 3, rom829, 0), "module": "KJV"}, "bad_params")
}

// listed gives the listed highlights of one Bible verse, in order.
func listed(e *env, ref string) []Highlight {
	var list []Highlight
	e.must("highlight.list", map[string]any{"ref": ref}, &list)
	return list
}

func TestHighlightOverlapSplits(t *testing.T) {
	e := setup(t)
	// "And we know that all things work together for good" in one verse, offsets as drawn.
	words := "And we know that all things work together for good"
	part := func(a, b int) []QuotePart { return []QuotePart{{Verse: rom828, Text: words[a:b]}} }
	var old, mid Highlight
	e.must("highlight.create", map[string]any{"range": rng(rom828, 0, rom828, 27), "module": "KJV", "parts": part(0, 27), "style": "green"}, &old)
	// A new highlight inside the old one leaves two green pieces either side of it.
	e.must("highlight.create", map[string]any{"range": rng(rom828, 7, rom828, 16), "module": "KJV", "parts": part(7, 16), "style": "pink"}, &mid)
	list := listed(e, "Rom 8:28")
	if len(list) != 3 {
		t.Fatalf("after splitting: %+v", list)
	}
	want := []struct {
		style, text string
		a, b        int
	}{{"green", "And we ", 0, 7}, {"pink", "know that", 7, 16}, {"green", " all things", 16, 27}}
	for i, w := range want {
		h := list[i]
		if h.Style != w.style || h.Range.Start.Offset != w.a || h.Range.End.Offset != w.b || h.Text != joinParts([]QuotePart{{Text: w.text}}) {
			t.Errorf("piece %d = %s %+v %q, want %+v", i, h.Style, *h.Range, h.Text, w)
		}
	}
	if list[0].ID != old.ID || list[0].CreatedAt != list[2].CreatedAt {
		t.Errorf("the old highlight keeps its first piece and its age: %+v", list)
	}
	// Overlapping the end of a piece shortens it; covering a piece wholly removes it.
	e.must("highlight.create", map[string]any{"range": rng(rom828, 3, rom828, 30), "module": "KJV", "parts": part(3, 30), "style": "blue"}, nil)
	list = listed(e, "Rom 8:28")
	if len(list) != 2 || list[0].Style != "green" || list[0].Range.End.Offset != 3 || list[0].Text != "And" || list[1].Style != "blue" {
		t.Fatalf("after covering: %+v", list)
	}
	// A highlight in another Bible is left alone: its offsets count in other words.
	e.must("highlight.create", map[string]any{"range": rng(rom828, 0, rom828, 10), "module": "WEB", "quote": "We know t", "style": "purple"}, nil)
	if n := len(listed(e, "Rom 8:28")); n != 3 {
		t.Errorf("another Bible's highlight split this one: %d", n)
	}
}

func TestHighlightOverlapAcrossVerses(t *testing.T) {
	e := setup(t)
	parts := []QuotePart{{Verse: rom828, Text: "all things work together for good"}, {Verse: rom829, Text: "For whom he did foreknow"}}
	var old Highlight
	e.must("highlight.create", map[string]any{"range": rng(rom828, 17, rom829, 24), "module": "KJV", "parts": parts, "style": "yellow"}, &old)
	if old.Ref != "Romans 8:28-29" || old.Text != "all things work together for good For whom he did foreknow" {
		t.Fatalf("spanning highlight = %+v", old)
	}
	// Cut from the middle of verse 28 to the middle of verse 29.
	e.must("highlight.create", map[string]any{"range": rng(rom828, 33, rom829, 8), "module": "KJV",
		"parts": []QuotePart{{Verse: rom828, Text: "together for good"}, {Verse: rom829, Text: "For whom"}}, "style": "pink"}, nil)
	list := listed(e, "Rom 8:28-29")
	if len(list) != 3 {
		t.Fatalf("pieces: %+v", list)
	}
	if list[0].Text != "all things work" || list[0].Ref != "Romans 8:28" || list[2].Text != "he did foreknow" || list[2].Ref != "Romans 8:29" {
		t.Errorf("pieces' words: %q (%s), %q (%s)", list[0].Text, list[0].Ref, list[2].Text, list[2].Ref)
	}
	// Moving a highlight with highlight.update splits the others too.
	e.must("highlight.update", map[string]any{"id": list[1].ID, "range": rng(rom828, 0, rom829, 24), "module": "KJV", "quote": "moved", "style": "green"}, nil)
	list = listed(e, "Rom 8:28-29")
	if len(list) != 1 || list[0].Style != "green" || list[0].Text != "moved" || list[0].Start != rom828 || list[0].End != rom829 {
		t.Errorf("after moving: %+v", list)
	}
}

func TestHighlightRangeLeavesWholeVersesAlone(t *testing.T) {
	e := setup(t)
	var whole, part Highlight
	e.must("highlight.create", map[string]any{"ref": "Rom 8:28-29", "style": "blue"}, &whole)
	e.must("highlight.create", map[string]any{"range": rng(rom828, 4, rom828, 11), "module": "KJV", "quote": "we know", "style": "pink"}, &part)
	list := listed(e, "Rom 8:28")
	if len(list) != 2 {
		t.Fatalf("highlights: %+v", list)
	}
	for _, h := range list {
		if h.ID == whole.ID && (h.Range != nil || h.Start != rom828 || h.End != rom829 || h.Style != "blue") {
			t.Errorf("the whole-verse highlight changed: %+v", h)
		}
	}
	// It still restyles, and a whole-verse highlight takes no range of its own words.
	e.must("highlight.update", map[string]any{"id": whole.ID, "style": "green"}, &whole)
	if whole.Style != "green" || whole.Range != nil {
		t.Errorf("restyled = %+v", whole)
	}
	var books []Highlight
	e.must("highlight.list", map[string]any{"on": "verses"}, &books)
	if len(books) != 2 {
		t.Errorf("on verses: %d", len(books))
	}
	// A book highlight cannot be given a Bible range.
	var inBook Highlight
	e.must("highlight.create", map[string]any{"item_id": e.items["epub"], "locator": Anchor{Section: 1, Para: 1, Offset: 0, EndPara: 1, EndOffset: 4, Text: "Some"}}, &inBook)
	e.fails("highlight.update", map[string]any{"id": inBook.ID, "range": rng(rom828, 0, rom828, 4), "module": "KJV"}, "bad_params")
	e.fails("highlight.create", map[string]any{"item_id": e.items["epub"], "locator": Anchor{Section: 1, Para: 1}, "range": rng(rom828, 0, rom828, 4), "module": "KJV"}, "bad_params")
}
