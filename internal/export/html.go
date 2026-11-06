package export

import (
	"fmt"
	"html"
	"net/url"
	"strings"
)

// The HTML is the preview, the page the system print dialog prints, and the HTML export. Every
// piece of text is escaped; links keep only web and mail addresses.

// Body is the document as an HTML fragment inside <div class="xdoc">.
func Body(d Doc) string {
	var b strings.Builder
	b.WriteString(`<div class="xdoc">`)
	if d.Title != "" {
		fmt.Fprintf(&b, `<h1 class="xd-title" dir="auto">%s</h1>`, esc(d.Title))
	}
	for _, p := range d.Pieces {
		b.WriteString(`<section class="xd-piece">`)
		if d.showTitle(p) {
			fmt.Fprintf(&b, `<h2 dir="auto">%s</h2>`, esc(p.Title))
		}
		if p.Subtitle != "" {
			fmt.Fprintf(&b, `<p class="xd-sub" dir="auto">%s</p>`, esc(p.Subtitle))
		}
		htmlBlocks(&b, p.Blocks, d.Opt)
		if !d.creditsAtEnd() {
			for _, c := range p.Credits {
				fmt.Fprintf(&b, `<p class="xd-credit" dir="auto">%s</p>`, esc(c))
			}
		}
		b.WriteString(`</section>`)
	}
	if cs := d.credits(); d.creditsAtEnd() && len(cs) > 0 {
		b.WriteString(`<section class="xd-credits"><h2>Credits</h2><ul>`)
		for _, c := range cs {
			fmt.Fprintf(&b, `<li dir="auto">%s</li>`, esc(c))
		}
		b.WriteString(`</ul></section>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func htmlBlocks(b *strings.Builder, bs []Block, opt Options) {
	n := 0 // numbered items so far, counted on over bullets (an outline's sub-points)
	for i := 0; i < len(bs); i++ {
		bl := bs[i]
		if bl.Kind != Numbered && bl.Kind != Bullet {
			n = 0
		}
		switch bl.Kind {
		case Bullet, Numbered:
			switch {
			case bl.Kind == Bullet:
				b.WriteString("<ul>")
			case n > 0:
				fmt.Fprintf(b, `<ol start="%d">`, n+1)
			default:
				b.WriteString("<ol>")
			}
			for ; i < len(bs) && bs[i].Kind == bl.Kind; i++ {
				if bl.Kind == Numbered {
					n++
				}
				fmt.Fprintf(b, `<li dir="auto">%s%s</li>`, checkMark(bs[i].Check, "☐ ", "☑ "), inlineHTML(bs[i].Spans))
			}
			i--
			if bl.Kind == Numbered {
				b.WriteString("</ol>")
			} else {
				b.WriteString("</ul>")
			}
		case Verse:
			b.WriteString(`<p class="xd-verses" dir="auto">`)
			for ; i < len(bs) && bs[i].Kind == Verse; i++ {
				b.WriteString(`<span class="xd-v">`)
				if opt.verseNumbers() && bs[i].Num != "" {
					fmt.Fprintf(b, `<sup class="xd-n">%s</sup>`, esc(bs[i].Num))
				}
				fmt.Fprintf(b, "%s</span> ", inlineHTML(bs[i].Spans))
			}
			i--
			b.WriteString(`</p>`)
		case Heading:
			lv := min(max(bl.Level, 2), 3) + 1
			fmt.Fprintf(b, `<h%d dir="auto">%s</h%d>`, lv, inlineHTML(bl.Spans), lv)
		case Quote:
			fmt.Fprintf(b, `<blockquote dir="auto">%s</blockquote>`, inlineHTML(bl.Spans))
		case Notice:
			fmt.Fprintf(b, `<p class="xd-notice" role="note">%s</p>`, inlineHTML(bl.Spans))
		case Table:
			tableHTML(b, bl.Table, opt)
		case Rule:
			b.WriteString(`<hr>`)
		case Handwriting:
			fmt.Fprintf(b, `<div class="xd-par xd-ink">%s</div>`, bl.Ink.svg())
		default:
			fmt.Fprintf(b, `<p dir="auto">%s</p>`, inlineHTML(bl.Spans))
		}
	}
}

func tableHTML(b *strings.Builder, g *Grid, opt Options) {
	num := g.numbered(opt)
	if g.Plain {
		b.WriteString(`<table class="xd-par xd-table">`)
		if len(g.Widths) == g.Cols() {
			b.WriteString("<colgroup>")
			for _, f := range g.fractions() {
				fmt.Fprintf(b, `<col style="width:%.1f%%">`, f*100)
			}
			b.WriteString("</colgroup>")
		}
	} else {
		b.WriteString(`<table class="xd-par">`)
	}
	if g.HasHead() {
		b.WriteString(`<thead><tr>`)
		if num {
			b.WriteString(`<th class="xd-n"></th>`)
		}
		for _, h := range g.headSpans() {
			fmt.Fprintf(b, `<th dir="auto">%s</th>`, inlineHTML(h))
		}
		b.WriteString(`</tr></thead>`)
	}
	b.WriteString(`<tbody>`)
	for _, r := range g.Rows {
		b.WriteString(`<tr>`)
		if num {
			fmt.Fprintf(b, `<td class="xd-n">%s</td>`, esc(r.Num))
		}
		for _, c := range r.Cells {
			fmt.Fprintf(b, `<td dir="auto">%s</td>`, inlineHTML(c))
		}
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</tbody></table>`)
}

func inlineHTML(spans []Span) string {
	var b strings.Builder
	for _, s := range spans {
		if s.Break {
			b.WriteString("<br>")
			continue
		}
		t := esc(s.Text)
		if s.Code {
			t = "<code>" + t + "</code>"
		}
		if s.Under {
			t = "<u>" + t + "</u>"
		}
		if s.Italic {
			t = "<em>" + t + "</em>"
		}
		if s.Bold {
			t = "<strong>" + t + "</strong>"
		}
		if u := safeURL(s.Link); u != "" {
			t = `<a href="` + esc(u) + `">` + t + "</a>"
		}
		b.WriteString(t)
	}
	return b.String()
}

func safeURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return u.String()
	}
	return ""
}

func esc(s string) string { return html.EscapeString(s) }

// pageSize is the paper size in millimetres.
func pageSize(o Options) (w, h float64, css string) {
	if o.PageSize == "letter" {
		return 215.9, 279.4, "letter"
	}
	return 210, 297, "A4"
}

// CSS styles a Body, scoped to .xdoc, with the page rules for printing.
func CSS(o Options, title string) string {
	_, _, size := pageSize(o)
	var page strings.Builder
	fmt.Fprintf(&page, "@page { size: %s; margin: 18mm 16mm 20mm;", size)
	if o.header() && title != "" {
		fmt.Fprintf(&page, ` @top-center { content: %s; font: italic 9pt Georgia, "Times New Roman", serif; color: #555; }`, cssString(title))
	}
	if o.footer() {
		page.WriteString(` @bottom-center { content: counter(page); font: 9pt Georgia, "Times New Roman", serif; color: #555; }`)
	}
	page.WriteString(" }\n")
	return page.String() + fmt.Sprintf(`.xdoc { font-family: Georgia, "Times New Roman", "Noto Serif", serif; font-size: %gpt; line-height: 1.45; color: #111; background: #fff; }
.xdoc h1.xd-title { font-size: 1.6em; margin: 0 0 .6em; }
.xdoc h2 { font-size: 1.3em; margin: 1.2em 0 .2em; break-after: avoid; }
.xdoc h3 { font-size: 1.1em; margin: 1em 0 .2em; break-after: avoid; }
.xdoc h4 { font-size: 1em; margin: .8em 0 .2em; break-after: avoid; }
.xdoc p { margin: 0 0 .55em; }
.xdoc .xd-sub { color: #444; font-style: italic; margin-top: 0; }
.xdoc .xd-n { font-size: .7em; color: #555; font-weight: normal; vertical-align: super; line-height: 0; padding-right: .15em; }
.xdoc td.xd-n { vertical-align: top; line-height: inherit; padding-top: .35em; }
.xdoc blockquote { margin: .4em 0 .6em 1.2em; padding-left: .8em; border-left: 2px solid #bbb; }
.xdoc .xd-credit, .xdoc .xd-credits li { font-size: .82em; color: #444; }
.xdoc .xd-credit { margin-top: .4em; }
.xdoc .xd-notice { font-size: .9em; border: 1px solid #c90; background: #fff8e6; padding: .4em .6em; border-radius: 3px; }
.xdoc table.xd-par { border-collapse: collapse; width: 100%%; table-layout: fixed; margin: .3em 0 .6em; }
.xdoc table.xd-par th { text-align: start; border-bottom: 1px solid #999; padding: .2em .4em; }
.xdoc table.xd-par td { vertical-align: top; padding: .25em .4em; }
.xdoc table.xd-par th.xd-n, .xdoc table.xd-par td.xd-n { width: 2.2em; padding-left: 0; }
.xdoc .xd-ink { margin: .4em 0 .8em; break-inside: avoid; }
.xdoc .xd-ink svg { display: block; width: 100%%; height: auto; }
.xdoc table.xd-par tr { break-inside: avoid; }
.xdoc table.xd-table th, .xdoc table.xd-table td { border: 1px solid #bbb; padding: .25em .45em; }
.xdoc table.xd-table th { background: #f2f2f2; }
.xdoc a { color: inherit; }
`, o.fontSize())
}

// HTML is the whole document as a standalone page.
func HTML(d Doc) string {
	return `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="generator" content="Xegesis"><title>` + esc(d.Title) + `</title>
<style>
` + CSS(d.Opt, d.Title) + `body { margin: 0; background: #fff; }
@media screen { .xdoc { max-width: 46em; margin: 2em auto; padding: 0 1.2em; } }
</style></head>
<body>` + Body(d) + "</body></html>\n"
}

// cssString quotes s as a CSS string.
func cssString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range oneLine(s) {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f || r == '<' || r == '>':
			fmt.Fprintf(&b, "\\%x ", r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
