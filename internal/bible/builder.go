package bible

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Verse is one verse, or several combined verses, as stored in a module.
type Verse struct {
	ID      verse.ID `json:"id"`
	End     verse.ID `json:"end,omitempty"` // last verse of a combined verse such as "1-2"
	Text    string   `json:"text"`
	Markup  string   `json:"markup,omitempty"`
	Strongs []string `json:"-"`
}

// Doc is what an importer reads from source files.
type Doc struct {
	Name     string // title found in the source, if any
	Language string
	Verses   []Verse
	Warnings []string
}

// span is an open markup element. Hidden spans (notes, headings) appear in the markup only;
// pending spans (USFM headings) belong before the next verse.
type span struct {
	tag, attrs      string
	hidden, pending bool
	emitted         bool
}

// builder collects verses from an importer's stream of events.
type builder struct {
	verses   map[verse.ID]*Verse
	cur      *Verse
	text     strings.Builder
	markup   strings.Builder
	pending  strings.Builder // markup that goes before the next verse
	para     bool            // a paragraph starts before the next text
	open     []span
	warnings map[string]int
	order    []string
	where    string // current location for warnings
	intro    bool   // reading an introduction entry, whose text outside headings is dropped quietly
}

func newBuilder() *builder {
	return &builder{verses: map[verse.ID]*Verse{}, warnings: map[string]int{}}
}

func (b *builder) warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if b.warnings[msg] == 0 {
		b.order = append(b.order, msg)
	}
	b.warnings[msg]++
}

func (b *builder) startVerse(id, end verse.ID) {
	b.endVerse()
	if end < id {
		end = id
	}
	b.cur = &Verse{ID: id, End: end}
	b.where = id.String()
	b.markup.WriteString(b.pending.String())
	b.pending.Reset()
	if b.para {
		b.markup.WriteString("<p/>")
		b.para = false
	}
	for i := range b.open {
		if !b.open[i].hidden && !b.open[i].pending {
			b.emitOpen(&b.open[i])
		}
	}
}

func (b *builder) endVerse() {
	if b.cur == nil {
		return
	}
	for i := len(b.open) - 1; i >= 0; i-- {
		if b.open[i].emitted && !b.open[i].pending {
			b.markup.WriteString("</" + b.open[i].tag + ">")
			b.open[i].emitted = false
		}
	}
	v := b.cur
	v.Text = squeeze(b.text.String())
	v.Markup = tidy(squeeze(b.markup.String()))
	b.text.Reset()
	b.markup.Reset()
	b.cur = nil
	b.store(v)
}

func (b *builder) store(v *Verse) {
	if old, ok := b.verses[v.ID]; ok {
		b.warn("%s appears more than once; texts joined", v.ID)
		old.Text = squeeze(old.Text + " " + v.Text)
		old.Markup = squeeze(old.Markup + " " + v.Markup)
		old.Strongs = append(old.Strongs, v.Strongs...)
		if v.End > old.End {
			old.End = v.End
		}
		return
	}
	b.verses[v.ID] = v
}

// out is where markup goes now.
func (b *builder) out() *strings.Builder {
	for _, s := range b.open {
		if s.pending {
			return &b.pending
		}
	}
	if b.cur == nil {
		return &b.pending
	}
	return &b.markup
}

func (b *builder) hidden() bool {
	for _, s := range b.open {
		if s.hidden {
			return true
		}
	}
	return false
}

// openSpan starts a markup element. Visible elements outside a verse wait until one starts.
func (b *builder) openSpan(tag, attrs string, hidden, pending bool) {
	s := span{tag: tag, attrs: attrs, hidden: hidden, pending: pending}
	if b.cur != nil || hidden || pending {
		if !hidden && !pending {
			b.flushPara()
		}
		b.emitOpen(&s)
	}
	b.open = append(b.open, s)
}

func (b *builder) emitOpen(s *span) {
	w := b.out()
	if s.pending {
		w = &b.pending
	}
	w.WriteString("<" + s.tag + s.attrs + ">")
	s.emitted = true
}

// closeSpan ends the innermost open element with this tag.
func (b *builder) closeSpan(tag string) {
	for i := len(b.open) - 1; i >= 0; i-- {
		if b.open[i].tag != tag {
			continue
		}
		s := b.open[i]
		if s.emitted {
			w := b.out()
			if s.pending {
				w = &b.pending
			}
			w.WriteString("</" + tag + ">")
		}
		b.open = append(b.open[:i], b.open[i+1:]...)
		return
	}
}

// closeAll ends every open element, as at the end of a book.
func (b *builder) closeAll() {
	for len(b.open) > 0 {
		b.closeSpan(b.open[len(b.open)-1].tag)
	}
}

func (b *builder) paragraph() { b.para = true }

func (b *builder) flushPara() {
	if b.para && b.cur != nil && b.markup.Len() > 0 {
		b.markup.WriteString("<p/>")
		b.para = false
	}
}

// addText adds source text to the current verse, or to the open note or heading.
func (b *builder) addText(s string) {
	if b.hidden() {
		b.out().WriteString(esc(s))
		return
	}
	if b.cur == nil {
		if strings.TrimSpace(s) != "" && !b.intro {
			b.warn("text outside a verse after %s dropped", b.whereOr("the start"))
		}
		return
	}
	if strings.TrimSpace(s) != "" {
		b.flushPara()
	}
	b.text.WriteString(s)
	b.markup.WriteString(esc(s))
}

// raw writes markup that has no plain-text form.
func (b *builder) raw(m string) { b.out().WriteString(m) }

func (b *builder) strong(nums []string) {
	if b.cur != nil {
		b.cur.Strongs = append(b.cur.Strongs, nums...)
	}
}

func (b *builder) whereOr(def string) string {
	if b.where == "" {
		return def
	}
	return b.where
}

// doc finishes the import and returns the verses in canonical order.
func (b *builder) doc() *Doc {
	b.endVerse()
	if strings.TrimSpace(stripTags(b.pending.String())) != "" {
		b.warn("heading or note after the last verse dropped")
	}
	d := &Doc{}
	for _, v := range b.verses {
		d.Verses = append(d.Verses, *v)
	}
	sort.Slice(d.Verses, func(i, j int) bool { return d.Verses[i].ID < d.Verses[j].ID })
	for i := 1; i < len(d.Verses); i++ {
		if d.Verses[i].ID <= d.Verses[i-1].End {
			b.warn("%s overlaps the combined verse %s", d.Verses[i].ID, d.Verses[i-1].ID)
		}
	}
	for _, m := range b.order {
		if n := b.warnings[m]; n > 1 {
			m = fmt.Sprintf("%s (%d times)", m, n)
		}
		d.Warnings = append(d.Warnings, m)
	}
	return d
}

// squeeze collapses runs of white space and trims the ends.
func squeeze(s string) string {
	var out strings.Builder
	space := false
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			space = true
			continue
		}
		if space && out.Len() > 0 {
			out.WriteByte(' ')
		}
		space = false
		out.WriteRune(r)
	}
	return out.String()
}

// tidy drops spaces just inside headings and notes, whose text is not part of the verse.
var tidy = strings.NewReplacer(" </h>", "</h>", " </f>", "</f>", " </x>", "</x>", "<h> ", "<h>", "<f> ", "<f>", "<x> ", "<x>").Replace

var escaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

func esc(s string) string { return escaper.Replace(s) }

func attr(name, value string) string {
	if value == "" {
		return ""
	}
	return " " + name + `="` + esc(value) + `"`
}

// stripTags removes markup elements, leaving their text.
func stripTags(m string) string {
	var out strings.Builder
	in := false
	for _, r := range m {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// normStrong turns "strong:H07225" or "h7225" into "H7225"; it returns "" for other values.
func normStrong(s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "strong:")
	if len(s) < 2 {
		return ""
	}
	p := strings.ToUpper(s[:1])
	if p != "H" && p != "G" {
		return ""
	}
	n := strings.TrimLeft(s[1:], "0")
	if n == "" || n[0] < '0' || n[0] > '9' {
		return ""
	}
	return p + n
}
