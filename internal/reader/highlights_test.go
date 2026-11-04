package reader

import "testing"

func TestHighlightListFiltersAndRestyle(t *testing.T) {
	e := setup(t)
	book := e.items["epub"]
	var john, rom, under, inBook Highlight
	e.must("highlight.create", map[string]any{"ref": "John 3:16", "style": "blue"}, &john)
	e.must("highlight.create", map[string]any{"ref": "Rom 8:28"}, &rom)
	e.must("highlight.create", map[string]any{"ref": "John 1:1", "style": "underline"}, &under)
	e.must("highlight.create", map[string]any{"item_id": book, "style": "green-underline", "locator": Anchor{Section: 2, Para: 2, Offset: 4,
		EndPara: 2, EndOffset: 20, Text: "things work together"}}, &inBook)

	ids := func(list []Highlight) []string {
		out := []string{}
		for _, h := range list {
			out = append(out, h.ID)
		}
		return out
	}
	same := func(name string, got []Highlight, want ...Highlight) {
		t.Helper()
		g := ids(got)
		if len(g) != len(want) {
			t.Fatalf("%s: got %v, want %d", name, g, len(want))
		}
		for i, w := range want {
			if g[i] != w.ID {
				t.Fatalf("%s: got %v, want %s at %d", name, g, w.ID, i)
			}
		}
	}

	var list []Highlight
	// Every highlight, Bible verses first in verse order, then books.
	e.must("highlight.list", map[string]any{}, &list)
	same("all", list, under, john, rom, inBook)
	e.must("highlight.list", map[string]any{"newest": true}, &list)
	same("newest", list, inBook, under, rom, john)
	e.must("highlight.list", map[string]any{"book": "John"}, &list)
	same("John", list, under, john)
	e.must("highlight.list", map[string]any{"on": "books"}, &list)
	same("books", list, inBook)
	if list[0].ItemTitle == "" || list[0].Text != "things work together" {
		t.Errorf("book highlight %+v", list[0])
	}
	// Yellow includes the older plain underline.
	e.must("highlight.list", map[string]any{"colour": "yellow"}, &list)
	same("yellow", list, under, rom)
	e.must("highlight.list", map[string]any{"colour": "green"}, &list)
	same("green", list, inBook)
	e.must("highlight.list", map[string]any{"query": "work together"}, &list)
	same("words", list, inBook)
	e.must("highlight.list", map[string]any{"query": "8:28"}, &list)
	same("place", list, rom)
	e.must("highlight.list", map[string]any{"limit": 1}, &list)
	same("limit", list, under)
	e.fails("highlight.list", map[string]any{"book": "Hezekiah"}, "bad_params")
	e.fails("highlight.list", map[string]any{"colour": "plaid"}, "bad_params")

	var h Highlight
	e.must("highlight.update", map[string]any{"id": john.ID, "style": "pink-underline"}, &h)
	if h.Style != "pink-underline" || h.Ref == "" {
		t.Fatalf("restyled %+v", h)
	}
	e.must("highlight.list", map[string]any{"colour": "pink"}, &list)
	same("pink", list, john)
	e.fails("highlight.update", map[string]any{"id": john.ID, "style": "plaid"}, "bad_params")
	e.must("highlight.delete", map[string]any{"id": john.ID}, nil)
	e.fails("highlight.update", map[string]any{"id": john.ID, "style": "green"}, "not_found")
}
