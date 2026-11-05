package export

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/Perlsowisdom/xegesis/internal/backup"
)

// Notes are stored as the editor's Markdown (frontend/src/editor/markdown.ts). parseNote reads it
// the same conservative way: only what that writer emits is recognised, anything else is text.
// Exported Markdown follows the same rules and any's block spacing (backup.RenderBlocks).

var (
	headingRe = regexp.MustCompile(`^(#{1,6}) (\S.*)$`)
	bulletRe  = regexp.MustCompile(`^- \S`)
	checkRe   = regexp.MustCompile(`^- \[([ x])\] \S`)
	quoteRe   = regexp.MustCompile(`^>( \S|$)`)
	linkRe    = regexp.MustCompile(`^\[([^\]\n]+)\]\(([^\s()]+)\)`)
)

func parseNote(md string) []Block {
	var out []Block
	for _, src := range backup.ParseBlocks(md) {
		out = append(out, parseBlock(src)...)
	}
	return out
}

func parseBlock(src string) []Block {
	if src == "" {
		return []Block{{Kind: Para}}
	}
	lines := strings.Split(src, "\n")
	if len(lines) == 1 {
		if m := headingRe.FindStringSubmatch(src); m != nil {
			return []Block{{Kind: Heading, Level: min(len(m[1])+1, 3), Spans: parseInline(m[2], Span{})}}
		}
		if src == "---" {
			return []Block{{Kind: Rule}}
		}
	}
	if g := parseNoteTable(lines); g != nil {
		return []Block{{Kind: Table, Table: g}}
	}
	if all(lines, func(i int, l string) bool { return checkRe.MatchString(l) }) {
		var out []Block
		for _, l := range lines {
			check := "todo"
			if l[3] == 'x' {
				check = "done"
			}
			out = append(out, Block{Kind: Bullet, Check: check, Spans: parseInline(l[6:], Span{})})
		}
		return out
	}
	if all(lines, func(i int, l string) bool { return bulletRe.MatchString(l) }) {
		var out []Block
		for _, l := range lines {
			out = append(out, Block{Kind: Bullet, Spans: parseInline(l[2:], Span{})})
		}
		return out
	}
	if all(lines, func(i int, l string) bool {
		p := strconv.Itoa(i+1) + ". "
		return strings.HasPrefix(l, p) && strings.TrimSpace(l[len(p):]) != ""
	}) {
		var out []Block
		for i, l := range lines {
			n := strconv.Itoa(i + 1)
			out = append(out, Block{Kind: Numbered, Num: n, Spans: parseInline(l[len(n)+2:], Span{})})
		}
		return out
	}
	kind, cut := Para, false
	if all(lines, func(i int, l string) bool { return quoteRe.MatchString(l) }) {
		kind, cut = Quote, true
	}
	var spans []Span
	for i, l := range lines {
		if i > 0 {
			spans = append(spans, Span{Break: true})
		}
		if cut {
			l = strings.TrimPrefix(strings.TrimPrefix(l, ">"), " ")
		}
		spans = append(spans, parseInline(l, Span{})...)
	}
	return []Block{{Kind: kind, Spans: spans}}
}

func all(lines []string, ok func(int, string) bool) bool {
	for i, l := range lines {
		if !ok(i, l) {
			return false
		}
	}
	return true
}

// parseInline reads **bold**, *italic*, <u>underline</u>, `code` and [text](url), as the editor writes them.
func parseInline(s string, marks Span) []Span {
	var out []Span
	lit := ""
	flush := func() {
		if lit != "" {
			sp := marks
			sp.Text = lit
			out = append(out, sp)
			lit = ""
		}
	}
	for i := 0; i < len(s); {
		rest := s[i:]
		var spans []Span
		n := 0
		switch {
		case rest[0] == '`':
			spans, n = codeAt(rest, marks)
		case strings.HasPrefix(rest, "***"):
			bi := marks
			bi.Bold, bi.Italic = true, true
			if spans, n = emphasisAt(rest, "***", bi); n == 0 {
				b := marks
				b.Bold = true
				spans, n = emphasisAt(rest, "**", b)
			}
		case strings.HasPrefix(rest, "**"):
			b := marks
			b.Bold = true
			spans, n = emphasisAt(rest, "**", b)
		case rest[0] == '*':
			it := marks
			it.Italic = true
			spans, n = emphasisAt(rest, "*", it)
		case strings.HasPrefix(rest, "<u>") && !marks.Under:
			if end := strings.Index(rest[3:], "</u>"); end > 0 {
				u := marks
				u.Under = true
				spans, n = parseInline(rest[3:3+end], u), end+7
			}
		case rest[0] == '[' && marks.Link == "":
			if m := linkRe.FindStringSubmatch(rest); m != nil {
				l := marks
				l.Link = m[2]
				spans, n = parseInline(m[1], l), len(m[0])
			}
		}
		if n > 0 {
			flush()
			out = append(out, spans...)
			i += n
			continue
		}
		lit += s[i : i+1]
		i++
	}
	flush()
	return out
}

func codeAt(rest string, marks Span) ([]Span, int) {
	end := strings.Index(rest[1:], "`") + 1
	if end < 2 || strings.TrimSpace(rest[1:end]) != rest[1:end] {
		return nil, 0
	}
	sp := marks
	sp.Text, sp.Code = rest[1:end], true
	return []Span{sp}, end + 1
}

func emphasisAt(rest, mark string, marks Span) ([]Span, int) {
	body := rest[len(mark):]
	if body == "" || unicode.IsSpace(rune(body[0])) || body[0] == '*' {
		return nil, 0
	}
	from := 0
	for {
		e := strings.Index(body[from:], mark)
		if e < 0 {
			return nil, 0
		}
		end := from + e
		if end < 1 {
			from = end + 1
			continue
		}
		inner := body[:end]
		after := byte(0)
		if end+len(mark) < len(body) {
			after = body[end+len(mark)]
		}
		if !unicode.IsSpace(rune(inner[len(inner)-1])) && !((mark == "*" || mark == "**") && after == '*') {
			return parseInline(inner, marks), len(mark)*2 + end
		}
		from = end + 1
	}
}

// Writing ----------------------------------------------------------------------------------------

func writeInline(spans []Span) string {
	var b strings.Builder
	for _, s := range spans {
		if s.Break {
			b.WriteString("\n")
			continue
		}
		t := s.Text
		if s.Code {
			t = wrapMark("`", t)
		}
		if s.Under {
			t = wrapTag("u", t)
		}
		if s.Italic {
			t = wrapMark("*", t)
		}
		if s.Bold {
			t = wrapMark("**", t)
		}
		if s.Link != "" && strings.TrimSpace(t) != "" {
			t = "[" + strings.TrimSpace(t) + "](" + s.Link + ")"
		}
		b.WriteString(t)
	}
	return b.String()
}

// wrapTag puts <tag> and </tag> round text without its edge spaces, as the editor writes underline.
func wrapTag(tag, text string) string {
	body := strings.TrimSpace(text)
	if body == "" {
		return text
	}
	i := strings.Index(text, body)
	return text[:i] + "<" + tag + ">" + body + "</" + tag + ">" + text[i+len(body):]
}

// wrapMark puts marks round text without its edge spaces, which Markdown does not allow inside.
func wrapMark(m, text string) string {
	body := strings.TrimSpace(text)
	if body == "" {
		return text
	}
	i := strings.Index(text, body)
	return text[:i] + m + body + m + text[i+len(body):]
}

// Markdown writes a document as any's canonical Markdown.
func Markdown(d Doc) []byte {
	var blocks []string
	if d.Title != "" {
		blocks = append(blocks, "# "+oneLine(d.Title))
	}
	for _, p := range d.Pieces {
		if d.showTitle(p) {
			blocks = append(blocks, "## "+oneLine(p.Title))
		}
		if p.Subtitle != "" {
			blocks = append(blocks, "*"+oneLine(p.Subtitle)+"*")
		}
		blocks = append(blocks, mdBlocks(p.Blocks, d.Opt)...)
		if !d.creditsAtEnd() {
			for _, c := range p.Credits {
				blocks = append(blocks, "*"+oneLine(c)+"*")
			}
		}
	}
	if d.creditsAtEnd() {
		if cs := d.credits(); len(cs) > 0 {
			blocks = append(blocks, "## Credits")
			blocks = append(blocks, bulletLines(cs))
		}
	}
	return []byte(backup.EncodeBody(backup.RenderBlocks(blocks)))
}

func mdBlocks(bs []Block, opt Options) []string {
	var out []string
	var list []string
	flush := func() {
		if len(list) > 0 {
			out = append(out, strings.Join(list, "\n"))
			list = nil
		}
	}
	n := 0 // numbered items so far, counted on over bullets (an outline's sub-points)
	for i, b := range bs {
		if b.Kind != Bullet && b.Kind != Numbered || i > 0 && bs[i-1].Kind != b.Kind {
			flush()
		}
		if b.Kind != Bullet && b.Kind != Numbered {
			n = 0
		}
		switch b.Kind {
		case Para:
			out = append(out, writeInline(b.Spans))
		case Heading:
			if t := oneLine(spansText(b.Spans)); t != "" {
				out = append(out, strings.Repeat("#", b.Level+1)+" "+writeInline(b.Spans))
			}
		case Quote:
			lines := strings.Split(writeInline(b.Spans), "\n")
			for j := range lines {
				lines[j] = "> " + lines[j]
			}
			out = append(out, strings.Join(lines, "\n"))
		case Bullet:
			list = append(list, "- "+checkMark(b.Check, "[ ] ", "[x] ")+oneLine(writeInline(b.Spans)))
		case Numbered:
			n++
			list = append(list, fmt.Sprintf("%d. %s", n, oneLine(writeInline(b.Spans))))
		case Verse:
			t := oneLine(writeInline(b.Spans))
			if opt.verseNumbers() && b.Num != "" {
				t = "**" + b.Num + "** " + t
			}
			out = append(out, t)
		case Notice:
			out = append(out, "> "+oneLine(writeInline(b.Spans)))
		case Table:
			out = append(out, mdTable(b.Table, opt))
		case Rule:
			out = append(out, "---")
		}
	}
	flush()
	return out
}

func mdTable(g *Grid, opt Options) string {
	if g.Plain {
		return mdNoteTable(g)
	}
	cell := func(s string) string { return strings.ReplaceAll(oneLine(s), "|", `\|`) }
	var lines []string
	head := []string{""}
	sep := []string{"---"}
	for _, h := range g.Head {
		head, sep = append(head, cell(h)), append(sep, "---")
	}
	if !opt.verseNumbers() {
		head, sep = head[1:], sep[1:]
	}
	lines = append(lines, "| "+strings.Join(head, " | ")+" |", "| "+strings.Join(sep, " | ")+" |")
	for _, r := range g.Rows {
		var cells []string
		if opt.verseNumbers() {
			cells = append(cells, cell(r.Num))
		}
		for _, c := range r.Cells {
			cells = append(cells, cell(writeInline(c)))
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
	}
	return strings.Join(lines, "\n")
}

func bulletLines(items []string) string {
	lines := make([]string, len(items))
	for i, s := range items {
		lines[i] = "- " + oneLine(s)
	}
	return strings.Join(lines, "\n")
}

// oneLine joins the lines of s so it stays one Markdown block line.
// checkMark is the box before a check-list item: todo or done, or "" for a plain bullet.
func checkMark(check, todo, done string) string {
	switch check {
	case "todo":
		return todo
	case "done":
		return done
	}
	return ""
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
