package search

import (
	"fmt"
	"math"
	"strings"

	"github.com/ledongthuc/pdf"
)

// pdfText returns one section per page. Text is laid out from the positioned glyphs that
// github.com/ledongthuc/pdf (BSD-3-Clause) decodes from each page's content stream: a new line
// where the baseline moves, a space where glyphs are apart, and a new paragraph at a wide gap
// between lines. Pages without text (scans) yield nothing.
func pdfText(file string) (out []section, err error) {
	defer func() {
		if x := recover(); x != nil {
			out, err = nil, fmt.Errorf("pdf: %v", x)
		}
	}()
	f, r, err := pdf.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	for n := 1; n <= r.NumPage(); n++ {
		p := r.Page(n)
		if p.V.IsNull() {
			continue
		}
		var blocks []block
		for _, para := range pageParagraphs(pageText(p)) {
			blocks = append(blocks, block{text: para})
		}
		if len(blocks) > 0 {
			out = append(out, section{loc: Locator{Page: n}, blocks: blocks})
		}
	}
	return out, nil
}

// pageText reads one page's glyphs, recovering from content the library cannot parse.
func pageText(p pdf.Page) (text []pdf.Text) {
	defer func() {
		if recover() != nil {
			text = nil
		}
	}()
	return p.Content().Text
}

func pageParagraphs(glyphs []pdf.Text) []string {
	var paras []string
	var buf strings.Builder
	var prev *pdf.Text
	lineGap := 0.0
	flush := func() {
		if s := clean(buf.String()); s != "" {
			paras = append(paras, s)
		}
		buf.Reset()
	}
	for i := range glyphs {
		g := &glyphs[i]
		size := math.Max(g.FontSize, 1)
		if prev != nil {
			dy := prev.Y - g.Y
			switch {
			case math.Abs(dy) > size*0.5:
				if lineGap > 0 && dy > lineGap*1.4 || dy < 0 && -dy > size*2 {
					flush()
				} else {
					buf.WriteByte(' ')
				}
				if dy > 0 && (lineGap == 0 || dy < lineGap) {
					lineGap = dy
				}
			case g.X-(prev.X+prev.W) > size*0.15 || g.X < prev.X-size:
				buf.WriteByte(' ')
			}
		}
		buf.WriteString(g.S)
		prev = g
	}
	flush()
	return paras
}
