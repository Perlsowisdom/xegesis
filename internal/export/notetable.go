package export

import (
	"regexp"
	"strconv"
	"strings"
)

// Tables in notes and sermons, as the editor writes them (frontend/src/editor/markdown.ts):
// GitHub-style rows with a separator row, "\|" for a pipe and "<br>" for a line break in a cell,
// and an optional comment line above for what the table cannot say itself:
//
//	<!-- table widths=120,200 header=no -->
//	| a | b |
//	| --- | --- |
//	| c | d |

var (
	tableNoteRe = regexp.MustCompile(`^<!-- table((?: (?:widths=\d{1,4}(?:,\d{1,4})*|header=no))*) -->$`)
	tableSepRe  = regexp.MustCompile(`^\|(?: *:?-{3,}:? *\|)+$`)
	widthsRe    = regexp.MustCompile(`widths=([\d,]+)`)
)

// parseNoteTable reads a table block, or gives nil when the lines are not one.
func parseNoteTable(lines []string) *Grid {
	note := ""
	rest := lines
	if len(lines) > 0 {
		if m := tableNoteRe.FindStringSubmatch(lines[0]); m != nil {
			note, rest = m[1], lines[1:]
		}
	}
	if len(rest) < 2 || !tableSepRe.MatchString(rest[1]) {
		return nil
	}
	for _, l := range rest {
		if len(l) < 2 || l[0] != '|' || l[len(l)-1] != '|' {
			return nil
		}
	}
	cols := len(rowCells(rest[1]))
	var rows [][]string
	for _, l := range append([]string{rest[0]}, rest[2:]...) {
		r := rowCells(l)
		if len(r) != cols {
			return nil
		}
		rows = append(rows, r)
	}
	cell := func(s string) []Span {
		var out []Span
		for i, part := range strings.Split(s, "<br>") {
			if i > 0 {
				out = append(out, Span{Break: true})
			}
			out = append(out, parseInline(part, Span{})...)
		}
		return out
	}
	g := &Grid{Plain: true}
	if m := widthsRe.FindStringSubmatch(note); m != nil {
		var ws []int
		for _, w := range strings.Split(m[1], ",") {
			n, _ := strconv.Atoi(w)
			ws = append(ws, n)
		}
		if len(ws) == cols {
			g.Widths = ws
		}
	}
	if !strings.Contains(note, "header=no") {
		for _, c := range rows[0] {
			sp := cell(c)
			g.Head = append(g.Head, spansText(sp))
			g.HeadSpans = append(g.HeadSpans, sp)
		}
		rows = rows[1:]
	}
	for _, r := range rows {
		var row GridRow
		for _, c := range r {
			row.Cells = append(row.Cells, cell(c))
		}
		g.Rows = append(g.Rows, row)
	}
	return g
}

// rowCells splits "| a | b\|c |" into ["a", "b|c"].
func rowCells(line string) []string {
	inner := line[1 : len(line)-1]
	var cells []string
	var cur strings.Builder
	for i := 0; i < len(inner); i++ {
		switch {
		case inner[i] == '\\' && i+1 < len(inner) && inner[i+1] == '|':
			cur.WriteByte('|')
			i++
		case inner[i] == '|':
			cells = append(cells, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(inner[i])
		}
	}
	cells = append(cells, cur.String())
	for i, c := range cells {
		cells[i] = strings.TrimSuffix(strings.TrimPrefix(c, " "), " ")
	}
	return cells
}

// mdNoteTable writes a note's table back in the editor's form.
func mdNoteTable(g *Grid) string {
	cols := g.Cols()
	cell := func(sp []Span) string {
		return strings.ReplaceAll(strings.ReplaceAll(writeInline(sp), "|", `\|`), "\n", "<br>")
	}
	row := func(cells [][]Span) string {
		parts := make([]string, cols)
		for i := range parts {
			if i < len(cells) {
				parts[i] = cell(cells[i])
			}
		}
		return "| " + strings.Join(parts, " | ") + " |"
	}
	var lines []string
	note := ""
	if len(g.Widths) == cols {
		ws := make([]string, cols)
		for i, w := range g.Widths {
			ws[i] = strconv.Itoa(w)
		}
		note += " widths=" + strings.Join(ws, ",")
	}
	rows := g.Rows
	var first [][]Span
	if g.HasHead() {
		first = g.headSpans()
	} else if len(rows) > 0 {
		note += " header=no"
		first, rows = rows[0].Cells, rows[1:]
	}
	if note != "" {
		lines = append(lines, "<!-- table"+note+" -->")
	}
	sep := make([]string, cols)
	for i := range sep {
		sep[i] = "---"
	}
	lines = append(lines, row(first), "| "+strings.Join(sep, " | ")+" |")
	for _, r := range rows {
		lines = append(lines, row(r.Cells))
	}
	return strings.Join(lines, "\n")
}

// Cols is the number of columns, without the verse-number column.
func (g *Grid) Cols() int {
	n := len(g.Head)
	for _, r := range g.Rows {
		n = max(n, len(r.Cells))
	}
	return max(n, 1)
}

// HasHead tells whether the table has a header row.
func (g *Grid) HasHead() bool { return len(g.Head) > 0 }

// headSpans are the header cells with their formatting (plain text for side-by-side Bibles).
func (g *Grid) headSpans() [][]Span {
	if len(g.HeadSpans) == len(g.Head) {
		return g.HeadSpans
	}
	out := make([][]Span, len(g.Head))
	for i, h := range g.Head {
		out[i] = []Span{{Text: h}}
	}
	return out
}

// fractions are the columns' shares of the width: from the widths set in the editor, or equal.
func (g *Grid) fractions() []float64 {
	cols := g.Cols()
	out := make([]float64, cols)
	total := 0
	if len(g.Widths) == cols {
		for _, w := range g.Widths {
			total += max(w, 1)
		}
	}
	for i := range out {
		if total > 0 {
			out[i] = float64(max(g.Widths[i], 1)) / float64(total)
		} else {
			out[i] = 1 / float64(cols)
		}
	}
	return out
}

// numbered tells whether a table gets the verse-number column: Bibles side by side only.
func (g *Grid) numbered(opt Options) bool { return !g.Plain && opt.verseNumbers() }
