package reader

import "testing"

func TestBuildBibleMap(t *testing.T) {
	toc := []TOCEntry{
		{Title: "Title page", Section: 1, Level: 1},
		{Title: "The Book of Psalms", Section: 2, Level: 1},
		{Title: "1", Section: 2, Anchor: "p1", Level: 2},
		{Title: "Psalm 84", Section: 3, Anchor: "p84", Level: 2},
		{Title: "Chapter 85", Section: 3, Anchor: "p85", Level: 2},
		{Title: "Genesis 1", Section: 4, Level: 1},
		{Title: "1 Samuel", Section: 5, Level: 1},
		{Title: "3", Section: 5, Anchor: "s3", Level: 2},
		{Title: "Notes", Section: 6, Level: 1},
	}
	got, unmapped := buildBibleMap(toc)
	type pos struct{ book, chapter, section int }
	var have []pos
	for _, e := range got {
		have = append(have, pos{e.Book, e.Chapter, e.Section})
	}
	want := []pos{{19, 0, 2}, {19, 1, 2}, {19, 84, 3}, {19, 85, 3}, {1, 1, 4}, {9, 0, 5}, {9, 3, 5}}
	if len(have) != len(want) {
		t.Fatalf("map = %+v, want %+v", have, want)
	}
	for i := range want {
		if have[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, have[i], want[i])
		}
	}
	if len(unmapped) != 2 || unmapped[0] != "Title page" || unmapped[1] != "Notes" {
		t.Errorf("unmapped = %v", unmapped)
	}
}

func TestAtReferenceNoBibleHeadings(t *testing.T) {
	e := setup(t)
	var m BibleMap
	e.must("book.bible_map", map[string]any{"item_id": e.items["epub"]}, &m)
	if len(m.Entries) != 0 || len(m.Unmapped) == 0 {
		t.Fatalf("a commentary with chapter headings maps nothing: %+v", m)
	}
	var at AtRef
	e.must("book.at_reference", map[string]any{"item_id": e.items["epub"], "ref": "Psalm 84"}, &at)
	if at.Found != "none" {
		t.Errorf("at = %+v", at)
	}
}
