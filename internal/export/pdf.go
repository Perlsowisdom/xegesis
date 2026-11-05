package export

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/Perlsowisdom/xegesis/internal/command"
)

// PDF writes the document as PDF 1.7 with embedded, subset TrueType fonts from the system. Layout
// is simple and predictable: one column of text (a grid for side-by-side Bibles), headings kept
// with what follows, page numbers and a running title. Right-to-left words are written in visual
// order; there is no shaping beyond that, so for complex scripts the printed page (system print
// dialog) is the better PDF, and the result says so whenever a character has no glyph.
func PDF(d Doc) ([]byte, []string, error) {
	fs, err := systemFonts()
	if err != nil {
		return nil, nil, command.Errorf("unavailable", "%v", err)
	}
	l := newLayout(d, fs)
	l.document()
	return l.write(), l.warnings(), nil
}

// pdfWarnings lays the document out without writing it, to say before exporting what a PDF would lose.
func pdfWarnings(d Doc) []string {
	fs, err := systemFonts()
	if err != nil {
		return []string{err.Error()}
	}
	l := newLayout(d, fs)
	l.document()
	return l.warnings()
}

const mmPt = 72 / 25.4

type tstyle struct {
	bold, italic bool
	size, rise   float64
	gray         float64
}

type tspan struct {
	text string
	st   tstyle
}

// piece is a run of glyphs in one font and style.
type piece struct {
	font  *ttfFont
	st    tstyle
	gids  []uint16
	runes []rune
	w     float64
}

type word struct {
	pieces []piece
	w      float64
	space  float64 // width of the space after it
	brk    bool    // a line break follows
}

type placed struct {
	p piece
	x float64
}

type line struct {
	items []placed
	w     float64
	size  float64 // the largest text size on the line
}

func (ln line) height() float64 { return ln.size * 1.4 }

type glyphRun struct {
	font *ttfFont
	size float64
	gids []uint16
	x, y float64
	gray float64
}

type rule struct {
	x1, y1, x2, y2, width float64
	rgb                   [3]float64
}

type pdfPage struct {
	runs  []glyphRun
	rules []rule
}

type layout struct {
	d       Doc
	fs      *fontSet
	base    float64
	pw, ph  float64
	ml, mr  float64
	mt, mb  float64
	pages   []*pdfPage
	cur     *pdfPage
	y       float64 // top of the next line
	used    map[*ttfFont]map[uint16][]rune
	order   []*ttfFont
	missing map[rune]bool
}

func newLayout(d Doc, fs *fontSet) *layout {
	w, h, _ := pageSize(d.Opt)
	l := &layout{d: d, fs: fs, base: d.Opt.fontSize(), pw: w * mmPt, ph: h * mmPt, ml: 16 * mmPt, mr: 16 * mmPt,
		mt: 18 * mmPt, mb: 20 * mmPt, used: map[*ttfFont]map[uint16][]rune{}, missing: map[rune]bool{}}
	l.newPage()
	return l
}

func (l *layout) warnings() []string {
	if len(l.missing) == 0 {
		return nil
	}
	var rs []string
	for r := range l.missing {
		rs = append(rs, string(r))
	}
	sort.Strings(rs)
	if len(rs) > 12 {
		rs = append(rs[:12], "…")
	}
	return []string{fmt.Sprintf("%d different characters (%s) have no letter shape in the fonts on this computer and show as boxes in the PDF. Print and choose Save as PDF to keep them.",
		len(l.missing), strings.Join(rs, " "))}
}

func (l *layout) width() float64 { return l.pw - l.ml - l.mr }

func (l *layout) newPage() {
	l.cur = &pdfPage{}
	l.pages = append(l.pages, l.cur)
	l.y = l.ph - l.mt
}

// need starts a new page unless h points fit, keeping a heading with what follows it.
func (l *layout) need(h float64) {
	if l.y-h < l.mb && l.y < l.ph-l.mt-1 {
		l.newPage()
	}
}

func (l *layout) style(bold, italic bool, scale, gray float64) tstyle {
	return tstyle{bold: bold, italic: italic, size: l.base * scale, gray: gray}
}

func (l *layout) face(st tstyle) *ttfFont {
	i := 0
	if st.bold {
		i |= 1
	}
	if st.italic {
		i |= 2
	}
	return l.fs.styles[i]
}

// fontFor picks the face for a character: the style's own, or the first fallback that has it.
func (l *layout) fontFor(r rune, st tstyle) (*ttfFont, uint16) {
	f := l.face(st)
	if g, ok := f.glyph(r); ok {
		return f, g
	}
	if g, ok := l.fs.styles[0].glyph(r); ok {
		return l.fs.styles[0], g
	}
	for _, fb := range l.fs.fallbacks {
		if g, ok := fb.glyph(r); ok {
			return fb, g
		}
	}
	if !unicode.IsControl(r) && !unicode.Is(unicode.Cf, r) {
		l.missing[r] = true
	}
	return f, 0
}

type styledRune struct {
	r  rune
	st tstyle
}

// words splits spans into words, putting right-to-left letters in visual order.
func (l *layout) words(spans []tspan) []word {
	var out []word
	var cur []styledRune
	flush := func(space float64, brk bool) {
		if len(cur) > 0 {
			out = append(out, l.shape(cur))
			cur = nil
		}
		if len(out) > 0 {
			out[len(out)-1].space = max(out[len(out)-1].space, space)
			out[len(out)-1].brk = out[len(out)-1].brk || brk
		} else if brk {
			out = append(out, word{brk: true})
		}
	}
	for _, s := range spans {
		for _, r := range s.text {
			switch {
			case r == '\n':
				flush(0, true)
			case unicode.IsSpace(r) && r != ' ':
				f, g := l.fontFor(' ', s.st)
				flush(f.advance(g)*s.st.size/1000, false)
			default:
				cur = append(cur, styledRune{r, s.st})
			}
		}
	}
	flush(0, false)
	return out
}

func (l *layout) shape(rs []styledRune) word {
	rtl := false
	for _, r := range rs {
		rtl = rtl || rtlRune(r.r)
	}
	if rtl {
		rs = reverseClusters(rs)
	}
	var w word
	for _, sr := range rs {
		f, g := l.fontFor(sr.r, sr.st)
		if n := len(w.pieces); n == 0 || w.pieces[n-1].font != f || w.pieces[n-1].st != sr.st {
			w.pieces = append(w.pieces, piece{font: f, st: sr.st})
		}
		p := &w.pieces[len(w.pieces)-1]
		adv := f.advance(g) * sr.st.size / 1000
		p.gids, p.runes, p.w = append(p.gids, g), append(p.runes, sr.r), p.w+adv
		w.w += adv
	}
	return w
}

// reverseClusters reverses letters, keeping each letter's combining marks after it.
func reverseClusters(rs []styledRune) []styledRune {
	var clusters [][]styledRune
	for _, r := range rs {
		if n := len(clusters); n > 0 && unicode.Is(unicode.Mn, r.r) {
			clusters[n-1] = append(clusters[n-1], r)
			continue
		}
		clusters = append(clusters, []styledRune{r})
	}
	out := make([]styledRune, 0, len(rs))
	for i := len(clusters) - 1; i >= 0; i-- {
		out = append(out, clusters[i]...)
	}
	return out
}

// lines breaks words into lines no wider than width.
func (l *layout) lines(ws []word, width float64) []line {
	var out []line
	cur := line{}
	x := 0.0
	push := func() {
		if cur.size == 0 {
			cur.size = l.base
		}
		out = append(out, cur)
		cur, x = line{}, 0
	}
	for _, w := range ws {
		for w.w > width && len(cur.items) == 0 && len(w.pieces) > 0 {
			head, rest := splitWord(w, width)
			if len(head.pieces) == 0 {
				break
			}
			l.put(&cur, head, 0)
			push()
			w = rest
		}
		if len(cur.items) > 0 && x+w.w > width {
			push()
		}
		l.put(&cur, w, x)
		x += w.w + w.space
		if w.brk {
			push()
		}
	}
	if len(cur.items) > 0 || len(out) == 0 {
		push()
	}
	return out
}

func (l *layout) put(ln *line, w word, x float64) {
	for _, p := range w.pieces {
		ln.items = append(ln.items, placed{p, x})
		x += p.w
		ln.size = max(ln.size, p.st.size)
	}
	ln.w = x
}

// splitWord cuts a word too long for a line at the last glyph that fits.
func splitWord(w word, width float64) (word, word) {
	var head, rest word
	x := 0.0
	full := false
	for _, p := range w.pieces {
		if full {
			rest.pieces = append(rest.pieces, p)
			rest.w += p.w
			continue
		}
		hp, rp := piece{font: p.font, st: p.st}, piece{font: p.font, st: p.st}
		for i, g := range p.gids {
			adv := p.font.advance(g) * p.st.size / 1000
			if !full && (x+adv <= width || x == 0) {
				hp.gids, hp.runes, hp.w = append(hp.gids, g), append(hp.runes, p.runes[i]), hp.w+adv
				x += adv
				continue
			}
			full = true
			rp.gids, rp.runes, rp.w = append(rp.gids, g), append(rp.runes, p.runes[i]), rp.w+adv
		}
		if len(hp.gids) > 0 {
			head.pieces = append(head.pieces, hp)
			head.w += hp.w
		}
		if len(rp.gids) > 0 {
			rest.pieces = append(rest.pieces, rp)
			rest.w += rp.w
		}
	}
	rest.space, rest.brk = w.space, w.brk
	return head, rest
}

// emit draws a line with its top at top, from x0 (or right-aligned in width for right-to-left).
func (l *layout) emit(ln line, x0, width, top float64, rtl bool) {
	baseline := top - ln.size*1.1
	off := 0.0
	if rtl {
		off = width - ln.w
	}
	for _, it := range ln.items {
		if len(it.p.gids) == 0 {
			continue
		}
		x := x0 + off + it.x
		if rtl {
			x = x0 + width - it.x - it.p.w
		}
		l.cur.runs = append(l.cur.runs, glyphRun{font: it.p.font, size: it.p.st.size, gids: it.p.gids, x: x,
			y: baseline + it.p.st.rise, gray: it.p.st.gray})
		l.record(it.p)
	}
}

func (l *layout) record(p piece) {
	m := l.used[p.font]
	if m == nil {
		m = map[uint16][]rune{}
		l.used[p.font] = m
		l.order = append(l.order, p.font)
	}
	for i, g := range p.gids {
		if _, ok := m[g]; !ok {
			m[g] = []rune{p.runes[i]}
		}
	}
}

// para lays out a paragraph; bar, when set, draws a rule beside each line.
func (l *layout) para(spans []tspan, indent, after float64, bar *[3]float64) {
	rtl := isRTL(spansPlain(spans))
	x0, width := l.ml+indent, l.width()-indent
	for _, ln := range l.lines(l.words(spans), width) {
		h := ln.height()
		if l.y-h < l.mb {
			l.newPage()
		}
		l.emit(ln, x0, width, l.y, rtl)
		if bar != nil {
			bx := x0 - 8
			if rtl {
				bx = x0 + width + 8
			}
			l.cur.rules = append(l.cur.rules, rule{bx, l.y, bx, l.y - h, 1.5, *bar})
		}
		l.y -= h
	}
	l.y -= after
}

func spansPlain(s []tspan) string {
	var b strings.Builder
	for _, x := range s {
		b.WriteString(x.text)
	}
	return b.String()
}

func (l *layout) spans(sp []Span, scale, gray float64, italic bool) []tspan {
	var out []tspan
	for _, s := range sp {
		if s.Break {
			out = append(out, tspan{"\n", l.style(false, false, scale, gray)})
			continue
		}
		out = append(out, tspan{s.Text, l.style(s.Bold, s.Italic || italic, scale, gray)})
	}
	return out
}

func (l *layout) text(s string, bold, italic bool, scale, gray float64) []tspan {
	return []tspan{{s, l.style(bold, italic, scale, gray)}}
}

func (l *layout) heading(s []tspan, before float64) {
	scale := s[0].st.size / l.base
	l.y -= before
	l.need(l.base*scale*1.4 + l.base*1.4*2)
	l.para(s, 0, l.base*0.25, nil)
}

func (l *layout) document() {
	d := l.d
	if d.Title != "" {
		l.para(l.text(d.Title, true, false, 1.6, 0), 0, l.base*0.6, nil)
	}
	for _, p := range d.Pieces {
		if d.showTitle(p) {
			l.heading(l.text(p.Title, true, false, 1.3, 0), l.base*0.9)
		}
		if p.Subtitle != "" {
			l.para(l.text(p.Subtitle, false, true, 1, 0.27), 0, l.base*0.4, nil)
		}
		l.blocks(p.Blocks)
		if !d.creditsAtEnd() {
			for _, c := range p.Credits {
				l.para(l.text(c, false, false, 0.82, 0.27), 0, l.base*0.3, nil)
			}
		}
	}
	if cs := d.credits(); d.creditsAtEnd() && len(cs) > 0 {
		l.heading(l.text("Credits", true, false, 1.3, 0), l.base*0.9)
		for _, c := range cs {
			l.para(l.text(c, false, false, 0.82, 0.27), 0, l.base*0.2, nil)
		}
	}
	l.furniture()
}

var (
	quoteBar  = [3]float64{0.7, 0.7, 0.7}
	noticeBar = [3]float64{0.8, 0.55, 0}
)

func (l *layout) blocks(bs []Block) {
	after := l.base * 0.55
	n := 0
	for i := 0; i < len(bs); i++ {
		b := bs[i]
		if b.Kind != Numbered && b.Kind != Bullet { // numbering goes on over sub-point bullets
			n = 0
		}
		switch b.Kind {
		case Heading:
			scale := 1.1
			if b.Level >= 3 {
				scale = 1
			}
			l.heading(l.spans(b.Spans, scale, 0, false), l.base*0.6)
		case Quote:
			l.para(l.spans(b.Spans, 1, 0, false), 18, after, &quoteBar)
		case Notice:
			l.para(l.spans(b.Spans, 0.92, 0, true), 10, after, &noticeBar)
		case Bullet, Numbered:
			mark := "•"
			if b.Check != "" {
				mark = checkMark(b.Check, "[ ]", "[x]")
			}
			if b.Kind == Numbered {
				n++
				mark = fmt.Sprintf("%d.", n)
			}
			l.listItem(mark, l.spans(b.Spans, 1, 0, false))
		case Verse:
			var sp []tspan
			for ; i < len(bs) && bs[i].Kind == Verse; i++ {
				if l.d.Opt.verseNumbers() && bs[i].Num != "" {
					st := l.style(false, false, 0.65, 0.33)
					st.rise = l.base * 0.33
					sp = append(sp, tspan{bs[i].Num + " ", st})
				}
				sp = append(sp, l.spans(bs[i].Spans, 1, 0, false)...)
				sp = append(sp, tspan{" ", l.style(false, false, 1, 0)})
			}
			i--
			l.para(sp, 0, after, nil)
		case Table:
			l.table(b.Table)
		case Rule:
			l.need(l.base)
			l.y -= l.base * 0.5
			l.cur.rules = append(l.cur.rules, rule{l.ml, l.y, l.ml + l.width(), l.y, 0.6, [3]float64{0.6, 0.6, 0.6}})
			l.y -= l.base * 0.6
		default:
			l.para(l.spans(b.Spans, 1, 0, false), 0, after, nil)
		}
	}
}

func (l *layout) listItem(mark string, sp []tspan) {
	rtl := isRTL(spansPlain(sp))
	lines := l.lines(l.words(sp), l.width()-18)
	ml := l.lines(l.words(l.text(mark, false, false, 1, 0)), 16)
	for i, ln := range lines {
		if l.y-ln.height() < l.mb {
			l.newPage()
		}
		if i == 0 {
			if rtl {
				l.emit(ml[0], l.ml+l.width()-16, 16, l.y, false)
			} else {
				l.emit(ml[0], l.ml+4, 14, l.y, false)
			}
		}
		x0 := l.ml + 18
		if rtl {
			x0 = l.ml
		}
		l.emit(ln, x0, l.width()-18, l.y, rtl)
		l.y -= ln.height()
	}
	l.y -= l.base * 0.2
}

// table lays out side-by-side Bibles line by line across all columns, so a row may continue on the
// next page; the column heads repeat at the top of each page.
func (l *layout) table(g *Grid) {
	numW := 0.0
	if g.numbered(l.d.Opt) {
		numW = l.base * 2.2
	}
	gap := l.base * 0.8
	cols := g.Cols()
	avail := l.width() - numW - gap*float64(cols-1)
	xs := make([]float64, cols)
	ws := make([]float64, cols)
	x := l.ml + numW
	for c, f := range g.fractions() {
		xs[c], ws[c] = x, avail*f
		x += ws[c] + gap
	}
	head := func() {
		if !g.HasHead() {
			return
		}
		var cells [][]line
		hmax := 0.0
		for i, h := range g.Head {
			ls := l.lines(l.words(l.text(h, true, false, 1, 0)), ws[i])
			cells = append(cells, ls)
			hmax = max(hmax, float64(len(ls))*ls[0].height())
		}
		l.need(hmax + l.base*1.4*2)
		top := l.y
		for c, ls := range cells {
			y := top
			for _, ln := range ls {
				l.emit(ln, xs[c], ws[c], y, isRTL(g.Head[c]))
				y -= ln.height()
			}
		}
		l.y = top - hmax - 2
		l.cur.rules = append(l.cur.rules, rule{l.ml, l.y, l.ml + l.width(), l.y, 0.6, [3]float64{0.6, 0.6, 0.6}})
		l.y -= 3
	}
	head()
	for _, r := range g.Rows {
		var cells [][]line
		rtl := make([]bool, len(r.Cells))
		rows := 0
		for c, cell := range r.Cells {
			sp := l.spans(cell, 1, 0, false)
			rtl[c] = isRTL(spansPlain(sp))
			ls := l.lines(l.words(sp), ws[min(c, cols-1)])
			cells = append(cells, ls)
			rows = max(rows, len(ls))
		}
		num := l.lines(l.words(l.text(r.Num, false, false, 0.7, 0.33)), numW)
		for k := 0; k < rows; k++ {
			h := l.base * 1.4
			for _, ls := range cells {
				if k < len(ls) {
					h = max(h, ls[k].height())
				}
			}
			if l.y-h < l.mb {
				l.newPage()
				head()
			}
			if k == 0 && numW > 0 {
				l.emit(num[0], l.ml, numW, l.y-l.base*0.1, false)
			}
			for c, ls := range cells {
				if k < len(ls) && c < cols {
					l.emit(ls[k], xs[c], ws[c], l.y, rtl[c])
				}
			}
			l.y -= h
		}
		l.y -= l.base * 0.3
	}
	l.y -= l.base * 0.4
}

// furniture adds the running title and page numbers once the page count is known.
func (l *layout) furniture() {
	cur := l.cur
	for i, p := range l.pages {
		l.cur = p
		if l.d.Opt.header() && l.d.Title != "" && i > 0 {
			ln := l.lines(l.words(l.text(oneLine(l.d.Title), false, true, 0.8, 0.33)), l.width())[0]
			l.emit(ln, l.ml+(l.width()-ln.w)/2, ln.w, l.ph-l.mt/2+ln.size*0.5, false)
		}
		if l.d.Opt.footer() {
			ln := l.lines(l.words(l.text(fmt.Sprint(i+1), false, false, 0.8, 0.33)), l.width())[0]
			l.emit(ln, l.ml+(l.width()-ln.w)/2, ln.w, l.mb/2+ln.size*0.5, false)
		}
	}
	l.cur = cur
}

// Writing ----------------------------------------------------------------------------------------

type pdfWriter struct {
	buf  bytes.Buffer
	offs map[int]int
}

func (w *pdfWriter) obj(n int, body string) {
	w.offs[n] = w.buf.Len()
	fmt.Fprintf(&w.buf, "%d 0 obj\n%s\nendobj\n", n, body)
}

func (w *pdfWriter) stream(n int, dict string, data []byte, compress bool) {
	if compress {
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		zw.Write(data)
		zw.Close()
		data = z.Bytes()
		dict += " /Filter /FlateDecode"
	}
	w.offs[n] = w.buf.Len()
	fmt.Fprintf(&w.buf, "%d 0 obj\n<< %s /Length %d >>\nstream\n", n, dict, len(data))
	w.buf.Write(data)
	w.buf.WriteString("\nendstream\nendobj\n")
}

func (l *layout) write() []byte {
	w := &pdfWriter{offs: map[int]int{}}
	w.buf.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	next := 4
	fontObj := map[*ttfFont]int{}
	for _, f := range l.order {
		fontObj[f] = next
		next += 5
	}
	var resources strings.Builder
	resources.WriteString("<< /Font <<")
	for i, f := range l.order {
		fmt.Fprintf(&resources, " /F%d %d 0 R", i+1, fontObj[f])
	}
	resources.WriteString(" >> >>")
	fontName := map[*ttfFont]string{}
	for i, f := range l.order {
		fontName[f] = fmt.Sprintf("/F%d", i+1)
		l.writeFont(w, f, fontObj[f], i)
	}
	var kids []string
	for _, p := range l.pages {
		page, content := next, next+1
		next += 2
		kids = append(kids, fmt.Sprintf("%d 0 R", page))
		w.obj(page, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources %s /Contents %d 0 R >>",
			l.pw, l.ph, resources.String(), content))
		w.stream(content, "", pageContent(p, fontName), true)
	}
	w.obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	w.obj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(l.pages)))
	w.obj(3, fmt.Sprintf("<< /Title %s /Producer (Xegesis) /CreationDate (D:%s) >>", pdfText(l.d.Title), time.Now().UTC().Format("20060102150405Z")))
	xref := w.buf.Len()
	fmt.Fprintf(&w.buf, "xref\n0 %d\n0000000000 65535 f \n", next)
	for i := 1; i < next; i++ {
		fmt.Fprintf(&w.buf, "%010d 00000 n \n", w.offs[i])
	}
	fmt.Fprintf(&w.buf, "trailer\n<< /Size %d /Root 1 0 R /Info 3 0 R >>\nstartxref\n%d\n%%%%EOF\n", next, xref)
	return w.buf.Bytes()
}

func pageContent(p *pdfPage, names map[*ttfFont]string) []byte {
	var b bytes.Buffer
	for _, r := range p.rules {
		fmt.Fprintf(&b, "%.3f %.3f %.3f RG %.2f w %.2f %.2f m %.2f %.2f l S\n", r.rgb[0], r.rgb[1], r.rgb[2], r.width, r.x1, r.y1, r.x2, r.y2)
	}
	b.WriteString("BT\n")
	for _, r := range p.runs {
		fmt.Fprintf(&b, "%s %.2f Tf %.3f g 1 0 0 1 %.2f %.2f Tm <", names[r.font], r.size, r.gray, r.x, r.y)
		for _, g := range r.gids {
			fmt.Fprintf(&b, "%04X", g)
		}
		b.WriteString("> Tj\n")
	}
	b.WriteString("ET\n")
	return b.Bytes()
}

func (l *layout) writeFont(w *pdfWriter, f *ttfFont, n, i int) {
	used := l.used[f]
	gids := make([]int, 0, len(used))
	keep := map[uint16]bool{}
	for g := range used {
		gids = append(gids, int(g))
		keep[g] = true
	}
	sort.Ints(gids)
	tag := fmt.Sprintf("XG%c%c%c%c", 'A'+i/17576%26, 'A'+i/676%26, 'A'+i/26%26, 'A'+i%26)
	name := "/" + tag + "+" + f.name
	scale := 1000 / float64(f.unitsPerEm)
	var widths strings.Builder
	for _, g := range gids {
		fmt.Fprintf(&widths, "%d [%d] ", g, int(f.advance(uint16(g))+0.5))
	}
	w.obj(n, fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont %s /Encoding /Identity-H /DescendantFonts [%d 0 R] /ToUnicode %d 0 R >>", name, n+1, n+4))
	w.obj(n+1, fmt.Sprintf("<< /Type /Font /Subtype /CIDFontType2 /BaseFont %s /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor %d 0 R /CIDToGIDMap /Identity /DW 1000 /W [%s] >>",
		name, n+2, widths.String()))
	flags := 32
	if f.italic != 0 {
		flags |= 64
	}
	w.obj(n+2, fmt.Sprintf("<< /Type /FontDescriptor /FontName %s /Flags %d /FontBBox [%d %d %d %d] /ItalicAngle %.1f /Ascent %d /Descent %d /CapHeight %d /StemV 80 /FontFile2 %d 0 R >>",
		name, flags, int(float64(f.bbox[0])*scale), int(float64(f.bbox[1])*scale), int(float64(f.bbox[2])*scale), int(float64(f.bbox[3])*scale),
		f.italic, int(float64(f.ascent)*scale), int(float64(f.descent)*scale), int(float64(f.capHeight)*scale), n+3))
	font := f.subset(keep)
	w.stream(n+3, fmt.Sprintf("/Length1 %d", len(font)), font, true)
	w.stream(n+4, "", toUnicode(gids, used), true)
}

func toUnicode(gids []int, used map[uint16][]rune) []byte {
	var b bytes.Buffer
	b.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	var entries []string
	for _, g := range gids {
		rs := used[uint16(g)]
		if g == 0 || len(rs) == 0 {
			continue
		}
		var hex strings.Builder
		for _, u := range utf16.Encode(rs) {
			fmt.Fprintf(&hex, "%04X", u)
		}
		entries = append(entries, fmt.Sprintf("<%04X> <%s>", g, hex.String()))
	}
	for i := 0; i < len(entries); i += 100 {
		chunk := entries[i:min(i+100, len(entries))]
		fmt.Fprintf(&b, "%d beginbfchar\n%s\nendbfchar\n", len(chunk), strings.Join(chunk, "\n"))
	}
	b.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return b.Bytes()
}

// pdfText encodes a string for the document information dictionary as UTF-16 with a byte order mark.
func pdfText(s string) string {
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, u := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&b, "%04X", u)
	}
	b.WriteString(">")
	return b.String()
}
