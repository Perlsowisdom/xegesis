package reader

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf16"

	xhtml "golang.org/x/net/html"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

type VerseTextParams struct {
	BookParams
	Ref string `json:"ref" required:"true" desc:"The verses, within one chapter, e.g. Psalm 84:3-5 or Psalm 84"`
}

type VerseText struct {
	Found   bool   `json:"found"`
	Reason  string `json:"reason,omitempty" desc:"Why no text was found"`
	Text    string `json:"text,omitempty" desc:"The verses' text without verse numbers"`
	Display string `json:"display,omitempty" desc:"The reference as written in a quotation"`
	Title   string `json:"title,omitempty" desc:"The book's title"`
	OSIS    string `json:"osis,omitempty"`
}

// paraUnits is the text of each numbered paragraph of rendered HTML, in UTF-16 units as positions count.
func paraUnits(html string) map[int][]uint16 {
	out := map[int][]uint16{}
	z := xhtml.NewTokenizer(strings.NewReader(html))
	para := 0
	for tt := z.Next(); tt != xhtml.ErrorToken; tt = z.Next() {
		t := z.Token()
		switch tt {
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			if t.Data == "span" && attrOf(t, "class") == "bp" {
				para, _ = strconv.Atoi(attrOf(t, "data-p"))
			}
		case xhtml.TextToken:
			if para > 0 {
				out[para] = append(out[para], utf16.Encode([]rune(t.Data))...)
			}
		}
	}
	return out
}

// textBetween joins the text from (p1, o1) up to, not including, (p2, o2).
func textBetween(texts map[int][]uint16, p1, o1, p2, o2 int, numbers []verseMark) string {
	var parts []string
	for p := p1; p <= p2; p++ {
		u := texts[p]
		lo, hi := 0, len(u)
		if p == p1 {
			lo = min(o1, len(u))
		}
		if p == p2 {
			hi = min(o2, len(u))
		}
		if lo >= hi {
			continue
		}
		seg := append([]uint16(nil), u[lo:hi]...)
		for _, m := range numbers { // the verse numbers inside are not text
			if m.Para == p && m.Offset >= lo && m.Offset+m.Skip <= hi {
				for i := m.Offset - lo; i < m.Offset-lo+m.Skip; i++ {
					seg[i] = ' '
				}
			}
		}
		parts = append(parts, string(utf16.Decode(seg)))
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

func (s *service) verseTextCmd(ctx context.Context, env *command.Env, p VerseTextParams) (VerseText, error) {
	r, err := verse.Parse(p.Ref)
	if err != nil {
		return VerseText{}, command.Errorf("bad_params", "%v", err)
	}
	out := VerseText{Display: r.String(), OSIS: r.Start.OSIS()}
	if r.End != r.Start {
		out.OSIS += "-" + r.End.OSIS()
	}
	b, err := s.open(ctx, env, p.ItemID, p.FileID)
	if err != nil {
		return VerseText{}, err
	}
	out.Title = b.item.Title
	no := func(why string) (VerseText, error) { out.Reason = why; return out, nil }
	if r.Start.Book() != r.End.Book() || r.Start.Chapter() != r.End.Chapter() || r.Start.Chapter() == 0 {
		return no("a quotation from a book is of verses in one chapter")
	}
	m, err := s.bibleMap(ctx, env, BookParams{ItemID: p.ItemID, FileID: b.file.ID})
	if err != nil {
		return VerseText{}, err
	}
	idx := -1
	for i, e := range m.Entries {
		if e.Book == r.Start.Book() && e.Chapter == r.Start.Chapter() {
			idx = i
			break
		}
	}
	if idx < 0 {
		return no(out.Title + " does not hold " + r.String())
	}
	rs, ok := renderedSection(b, m.Entries[idx].Section)
	if !ok {
		return no(out.Title + " is not a kind of book whose verses can be read")
	}
	from, to, ok := chapterBounds(rs, m.Entries, idx)
	if !ok {
		return no("the chapter's place in " + out.Title + " is not clear")
	}
	marks := acceptedMarks(rs.marks, from, to)
	first, last := r.Start.Verse(), r.End.Verse()
	var m1 *verseMark
	nextAfter := verseMark{Para: rs.paras + 1}
	if to > 0 {
		nextAfter.Para = to
	}
	for i := range marks {
		if m1 == nil && marks[i].N >= first {
			m1 = &marks[i]
		}
		if marks[i].N > last && last != verse.EndOfChapter {
			nextAfter = marks[i]
			break
		}
	}
	if m1 == nil || first > 1 && m1.N != first {
		return no(out.Title + " has no verse numbers to find " + r.String() + " by")
	}
	text := textBetween(paraUnits(rs.html), m1.Para, m1.Offset+m1.Skip, nextAfter.Para, nextAfter.Offset, marks)
	if text == "" {
		return no("no text was found for " + r.String())
	}
	out.Found, out.Text = true, text
	return out, nil
}
