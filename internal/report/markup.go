package report

import (
	"regexp"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

var (
	refRe   = regexp.MustCompile(`(?s)<r(?: o="([^"]*)")?>(.*?)</r>`)
	wRe     = regexp.MustCompile(`(?s)<w ([^>]*)>(.*?)</w>`)
	attrRe  = regexp.MustCompile(`([a-z]+)="([^"]*)"`)
	tagRe   = regexp.MustCompile(`<[^>]*>`)
	pieceRe = regexp.MustCompile(`[;,]`)
	chapVRe = regexp.MustCompile(`^\d+:\d+(?:-\d+(?::\d+)?)?$`)
	verseRe = regexp.MustCompile(`^\d+(?:-\d+)?$`)
)

var unescape = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`).Replace

var escape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace

// plain is markup as text.
func plain(markup string) string {
	return strings.Join(strings.Fields(unescape(tagRe.ReplaceAllString(strings.ReplaceAll(markup, "<p/>", " "), ""))), " ")
}

// osisOf is a range as an OSIS reference, as verse.parse gives it.
func osisOf(r verse.Range) string {
	if r.End == r.Start || r.End == 0 {
		return r.Start.OSIS()
	}
	return r.Start.OSIS() + "-" + r.End.OSIS()
}

// refContext remembers the book and chapter of the last reference, so that "3:22" after "Prov. 1:9"
// and "19" after "John 6:44" can be read.
type refContext struct {
	book    verse.Book
	chapter int
}

func (c *refContext) read(s string) (verse.Range, bool) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "."))
	if s == "" {
		return verse.Range{}, false
	}
	r, err := verse.Parse(s)
	if err != nil && c.book.Number > 0 {
		switch {
		case chapVRe.MatchString(s):
			r, err = verse.Parse(c.book.Name + " " + s)
		case verseRe.MatchString(s) && c.chapter > 0:
			r, err = verse.Parse(c.book.Name + " " + itoa(c.chapter) + ":" + s)
		}
	}
	if err != nil {
		return verse.Range{}, false
	}
	c.note(r)
	return r, true
}

func (c *refContext) note(r verse.Range) {
	if b, ok := verse.BookByNumber(r.Start.Book()); ok {
		c.book, c.chapter = b, r.End.Chapter()
	}
}

func itoa(n int) string {
	var b [8]byte
	i := len(b)
	for {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			return string(b[i:])
		}
	}
}

// linkRefs gives every reference in markup a target: a <r> without one ("Ps 84:11; Jas 1:17", as
// some topical indexes have) is split into one <r o="..."> per reference that can be read. It
// returns the new markup and the references in order.
func linkRefs(markup string) (string, []Link) {
	var ctx refContext
	var links []Link
	out := refRe.ReplaceAllStringFunc(markup, func(m string) string {
		sub := refRe.FindStringSubmatch(m)
		o, label := sub[1], sub[2]
		if o != "" {
			if r, err := verse.Parse(o); err == nil {
				ctx.note(r)
			}
			links = append(links, Link{Ref: o, Label: plain(label)})
			return m
		}
		var b strings.Builder
		text := label
		last := 0
		for _, sep := range append(pieceRe.FindAllStringIndex(text, -1), []int{len(text), len(text)}) {
			piece := text[last:sep[0]]
			if r, ok := ctx.read(unescape(tagRe.ReplaceAllString(piece, ""))); ok {
				lead := piece[:len(piece)-len(strings.TrimLeft(piece, " "))]
				osis := osisOf(r)
				b.WriteString(lead + `<r o="` + escape(osis) + `">` + strings.TrimLeft(piece, " ") + `</r>`)
				links = append(links, Link{Ref: osis, Label: plain(piece)})
			} else {
				b.WriteString(piece)
			}
			b.WriteString(text[sep[0]:sep[1]])
			last = sep[1]
		}
		return b.String()
	})
	return out, links
}

// word is one tagged word of a verse.
type word struct {
	text, morph, gloss string
	strongs            []string
}

// tagged lists the tagged words of verse markup.
func tagged(markup string) []word {
	var out []word
	for _, m := range wRe.FindAllStringSubmatch(markup, -1) {
		w := word{text: plain(m[2])}
		for _, a := range attrRe.FindAllStringSubmatch(m[1], -1) {
			switch a[1] {
			case "s":
				for _, s := range strings.Fields(a[2]) {
					if n := bible.NormStrong(s); n != "" {
						w.strongs = append(w.strongs, n)
					}
				}
			case "m":
				w.morph = unescape(a[2])
			case "g":
				w.gloss = unescape(a[2])
			}
		}
		if len(w.strongs) > 0 {
			out = append(out, w)
		}
	}
	return out
}

// main is the word's own number: the last one that is not a Hebrew prefix or particle (H9xxx in
// STEPBible's data), as "God" is often tagged G3588 G2316, the article then the noun.
func (w word) main() string {
	for i := len(w.strongs) - 1; i >= 0; i-- {
		if s := w.strongs[i]; !(strings.HasPrefix(s, "H9") && len(s) == 5) {
			return s
		}
	}
	return w.strongs[len(w.strongs)-1]
}

var strongRefRe = regexp.MustCompile(`\b(?:see (GREEK|HEBREW) for (\d{1,5})|([GH])(\d{1,5})[a-z]?\b)`)

// relatedNumbers finds the Strong's numbers a lexicon entry points to: "G26", or Strong's own
// "see GREEK for 26".
func relatedNumbers(text, self string) []string {
	seen := map[string]bool{self: true}
	var out []string
	for _, m := range strongRefRe.FindAllStringSubmatch(text, -1) {
		n := ""
		switch {
		case m[1] == "GREEK":
			n = "G" + m[2]
		case m[1] == "HEBREW":
			n = "H" + m[2]
		default:
			n = m[3] + m[4]
		}
		if n = bible.NormStrong(n); n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}
