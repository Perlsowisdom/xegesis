package export

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/Perlsowisdom/xegesis/internal/reader"
)

// Ink written on the pages of a PDF (reader.PageInk: points as fractions of the page box) is exported
// with the pages' text as a drawing under "Ink · page N": SVG in HTML, an outline in PDF, and an ink
// block in Markdown. The page is drawn blank-backed at its own proportions, the same way a note's
// handwriting is, so the strokes use the stroke engine of inkoutline.go. Word has no place for strokes.

const maxInkPages = 200

// pageInk gives a drawing block for each page from..to that has ink; nothing for other formats.
func (bd *builder) pageInk(src Source, format string) []unit {
	if format != "pdf" || src.NoInk || src.From < 1 {
		return nil
	}
	to := max(src.To, src.From)
	var out []unit
	for p, n := src.From, 0; p <= to && n < maxInkPages; p, n = p+1, n+1 {
		var got reader.InkPage
		if err := bd.run("ink.page.get", map[string]any{"item_id": src.ItemID, "page": p}, &got); err != nil || len(got.Ink.Strokes) == 0 {
			continue
		}
		ink := fromPageInk(got.Ink)
		label := fmt.Sprintf("Ink · page %d", p)
		out = append(out,
			unit{block: Block{Kind: Para, Spans: []Span{{Text: label, Italic: true}}}},
			unit{block: Block{Kind: Handwriting, Ink: ink}})
	}
	return out
}

// fromPageInk puts a page's strokes on the export's drawing, which is inkW units wide.
func fromPageInk(p reader.PageInk) *Ink {
	h := math.Min(8000, math.Max(200, math.Round(inkW*p.Aspect)))
	ink := &Ink{H: h}
	raw := struct {
		V int `json:"v"`
		H int `json:"h"`
		S []struct {
			T string       `json:"t"`
			C string       `json:"c"`
			W int          `json:"w"`
			P [][3]float64 `json:"p"`
		} `json:"s"`
	}{V: 2, H: int(h)}
	for _, k := range p.Strokes {
		pts := make([][3]float64, len(k.P))
		for i, q := range k.P {
			pts[i] = [3]float64{math.Round(q[0]*inkW*100) / 100, math.Round(q[1]*inkW*p.Aspect*100) / 100, q[2]}
		}
		ink.Strokes = append(ink.Strokes, InkStroke{Tool: k.T, Colour: k.C, Width: k.W, Points: pts})
		raw.S = append(raw.S, struct {
			T string       `json:"t"`
			C string       `json:"c"`
			W int          `json:"w"`
			P [][3]float64 `json:"p"`
		}{k.T, k.C, k.W, pts})
	}
	b, _ := json.Marshal(raw)
	ink.Raw = "```ink\n" + string(b) + "\n```"
	return ink
}
