package reader

import (
	"context"
	"testing"
)

func strokes(n int) []InkStroke {
	out := []InkStroke{}
	for i := 0; i < n; i++ {
		out = append(out, InkStroke{T: "pen", C: "blue", W: 2, P: [][3]float64{{0.1, 0.2, 0}, {0.3 + float64(i)/100, 0.25, 0.6}}})
	}
	return out
}

func TestInkPageRoundTripListAndHistory(t *testing.T) {
	e := setup(t)
	book := e.items["pdf"]
	var none InkPage
	e.must("ink.page.get", map[string]any{"item_id": book, "page": 2}, &none)
	if none.ID != "" || none.Ink.Count != 0 {
		t.Fatalf("empty page %+v", none)
	}

	var set InkPage
	e.must("ink.page.set", map[string]any{"item_id": book, "page": 2, "aspect": 1.414, "strokes": strokes(3)}, &set)
	if set.ID == "" || set.Ink.Count != 3 {
		t.Fatalf("set %+v", set)
	}
	var got InkPage
	e.must("ink.page.get", map[string]any{"item_id": book, "page": 2}, &got)
	if got.ID != set.ID || len(got.Ink.Strokes) != 3 || got.Ink.Aspect != 1.414 || got.Ink.Strokes[1].P[1][2] != 0.6 {
		t.Fatalf("get %+v", got)
	}
	e.must("ink.page.get", map[string]any{"item_id": book, "page": 1}, &none)
	if len(none.Ink.Strokes) != 0 {
		t.Fatalf("page 1 has ink %+v", none)
	}

	// A second set replaces the strokes in the same annotation.
	e.must("ink.page.set", map[string]any{"item_id": book, "page": 2, "aspect": 1.414, "strokes": strokes(2)}, &set)
	if set.ID != got.ID || set.Ink.Count != 2 {
		t.Fatalf("replaced %+v", set)
	}

	// It shows in the highlight list as "Ink · page 2", without the strokes, and in the page's own list.
	var list []Highlight
	e.must("highlight.list", map[string]any{"item_id": book}, &list)
	if len(list) != 1 || list[0].Style != "ink" || list[0].Text != "Ink · page 2" || list[0].Locator.Page != 2 ||
		list[0].Locator.Ink == nil || list[0].Locator.Ink.Count != 2 || len(list[0].Locator.Ink.Strokes) != 0 {
		t.Fatalf("list %+v", list)
	}
	e.must("highlight.list", map[string]any{"item_id": book, "section": 3}, &list)
	if len(list) != 0 {
		t.Fatalf("page 3 list %+v", list)
	}

	// Ink cannot be made or restyled as a plain highlight.
	e.fails("highlight.create", map[string]any{"item_id": book, "locator": Anchor{Section: 1, Page: 1, Rects: [][4]float64{{0, 0, 0.5, 0.1}}, Ink: &PageInk{Aspect: 1}}}, "bad_params")
	e.fails("highlight.update", map[string]any{"id": set.ID, "style": "green"}, "bad_params")

	// A cleared page leaves the list; drawing again brings it back in the same row.
	e.must("ink.page.set", map[string]any{"item_id": book, "page": 2, "aspect": 1.414, "strokes": []InkStroke{}}, &set)
	e.must("highlight.list", map[string]any{"item_id": book}, &list)
	if len(list) != 0 {
		t.Fatalf("cleared page listed %+v", list)
	}
	e.must("ink.page.set", map[string]any{"item_id": book, "page": 2, "aspect": 1.414, "strokes": strokes(1)}, &set)
	if set.ID != got.ID {
		t.Fatalf("new row %s, was %s", set.ID, got.ID)
	}

	// Deleting the annotation (the Highlights tool's delete) removes the ink; the next stroke starts afresh.
	e.must("highlight.delete", map[string]any{"id": set.ID}, nil)
	e.must("ink.page.get", map[string]any{"item_id": book, "page": 2}, &none)
	if none.ID != "" || len(none.Ink.Strokes) != 0 {
		t.Fatalf("deleted %+v", none)
	}
	e.must("ink.page.set", map[string]any{"item_id": book, "page": 2, "aspect": 1.414, "strokes": strokes(1)}, &set)
	if set.ID == got.ID || set.Ink.Count != 1 {
		t.Fatalf("after delete %+v", set)
	}

	// Every write is in the change history.
	var n int
	if err := e.st.QueryRow(context.Background(), `SELECT COUNT(*) FROM changes WHERE command = 'ink.page.set'`).Scan(&n); err != nil || n < 5 {
		t.Fatalf("changes %d %v", n, err)
	}
}

func TestInkPageValidation(t *testing.T) {
	e := setup(t)
	book := e.items["pdf"]
	bad := func(name string, p map[string]any) {
		t.Helper()
		p["item_id"] = book
		if _, ok := p["page"]; !ok {
			p["page"] = 1
		}
		if _, ok := p["aspect"]; !ok {
			p["aspect"] = 1.4
		}
		e.fails("ink.page.set", p, "bad_params")
		_ = name
	}
	bad("aspect", map[string]any{"aspect": 0, "strokes": strokes(1)})
	bad("tool", map[string]any{"strokes": []InkStroke{{T: "brush", C: "blue", W: 1, P: [][3]float64{{0, 0, 0}}}}})
	bad("colour", map[string]any{"strokes": []InkStroke{{T: "pen", C: "red", W: 1, P: [][3]float64{{0, 0, 0}}}}})
	bad("width", map[string]any{"strokes": []InkStroke{{T: "pen", C: "ink", W: 4, P: [][3]float64{{0, 0, 0}}}}})
	bad("empty", map[string]any{"strokes": []InkStroke{{T: "pen", C: "ink", W: 1}}})
	bad("off page", map[string]any{"strokes": []InkStroke{{T: "pen", C: "ink", W: 1, P: [][3]float64{{3, 0, 0}}}}})
	bad("pressure", map[string]any{"strokes": []InkStroke{{T: "pen", C: "ink", W: 1, P: [][3]float64{{0.5, 0.5, 2}}}}})
	bad("page", map[string]any{"page": 0})
	e.fails("ink.page.set", map[string]any{"item_id": "nope", "page": 1, "aspect": 1.4, "strokes": strokes(1)}, "not_found")
}
