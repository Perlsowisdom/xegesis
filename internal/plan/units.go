package plan

import (
	"fmt"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/bible/sword"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// sectionWeight is how many verses one section of a library book counts for when a plan is
// balanced: about the length of an average Bible chapter.
const sectionWeight = 30

// Reading is one thing to read on one day: a passage of the Bible or a section of a library book.
type Reading struct {
	Ref     string   `json:"ref" desc:"What to read, in words: \"Genesis 1-3\" or \"Grace, section 4\""`
	OSIS    string   `json:"osis,omitempty" desc:"A Bible passage as a reference verse.parse and bible.read accept"`
	Start   verse.ID `json:"start,omitempty" desc:"First verse of a Bible passage"`
	End     verse.ID `json:"end,omitempty" desc:"Last verse of a Bible passage"`
	ItemID  string   `json:"item_id,omitempty" desc:"The library book, for a book reading"`
	Section int      `json:"section,omitempty" desc:"The section of that book"`
	Title   string   `json:"title,omitempty" desc:"The book's title"`
	Verses  int      `json:"verses,omitempty" desc:"How many verses the passage has"`
}

// unit is the smallest piece a plan is cut into: some verses of one chapter, or one book section.
type unit struct {
	weight int
	bible  *piece
	book   *bookPart
}

type piece struct{ book, chapter, from, to int }

type bookPart struct {
	item, title string
	section     int
}

// chapterVerses holds the verse count of every chapter, from the KJV numbering that Xegesis uses
// for references (the public-domain counts in internal/bible/sword/kjv.txt).
var chapterVerses = func() map[int][]int {
	v, ok := sword.LookupVersification("")
	if !ok {
		panic("plan: no KJV numbering")
	}
	m := map[int][]int{}
	for _, b := range append(append([]sword.Book{}, v.OT...), v.NT...) {
		if b.Number != 0 {
			m[b.Number] = b.Verses
		}
	}
	return m
}()

func verseID(book, chapter, v int) verse.ID {
	id, _ := verse.Make(book, chapter, v)
	return id
}

// rangeUnits cuts the verses from start to end into one unit per chapter.
func rangeUnits(start, end verse.ID) ([]unit, error) {
	var out []unit
	for b := start.Book(); b <= end.Book(); b++ {
		counts, ok := chapterVerses[b]
		if !ok {
			return nil, command.Errorf("bad_params", "plans cannot include book %d", b)
		}
		fc, fv, lc, lv := 1, 1, len(counts), counts[len(counts)-1]
		if b == start.Book() {
			fc, fv = start.Chapter(), max(start.Verse(), 1)
		}
		if b == end.Book() {
			lc, lv = end.Chapter(), end.Verse()
		}
		if fc > len(counts) || lc > len(counts) {
			return nil, command.Errorf("bad_params", "%s has only %d chapters", bookName(b), len(counts))
		}
		for c := fc; c <= lc; c++ {
			from, to := 1, counts[c-1]
			if c == fc {
				from = fv
			}
			if c == lc {
				to = min(lv, counts[c-1])
			}
			if from > to {
				return nil, command.Errorf("bad_params", "%s %d has only %d verses", bookName(b), c, counts[c-1])
			}
			out = append(out, unit{weight: to - from + 1, bible: &piece{b, c, from, to}})
		}
	}
	return out, nil
}

func bookName(n int) string {
	b, _ := verse.BookByNumber(n)
	return b.Name
}

// bookUnits is every chapter of the given books.
func bookUnits(books []int) ([]unit, error) {
	var out []unit
	for _, b := range books {
		counts := chapterVerses[b]
		u, err := rangeUnits(verseID(b, 1, 1), verseID(b, len(counts), counts[len(counts)-1]))
		if err != nil {
			return nil, err
		}
		out = append(out, u...)
	}
	return out, nil
}

// parseRange reads "Genesis", "Gen-Deut" or a reference such as "Rom 1-8" or "John 3:16-21".
func parseRange(s string) (start, end verse.ID, err error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "–", "-")
	if b, ok := verse.LookupBook(s); ok {
		return verseID(b.Number, 1, 1), lastVerse(b.Number), nil
	}
	if a, z, cut := strings.Cut(s, "-"); cut {
		ba, ok1 := verse.LookupBook(strings.TrimSpace(a))
		bz, ok2 := verse.LookupBook(strings.TrimSpace(z))
		if ok1 && ok2 && ba.Number <= bz.Number {
			return verseID(ba.Number, 1, 1), lastVerse(bz.Number), nil
		}
	}
	r, err := verse.Parse(s)
	if err != nil {
		return 0, 0, command.Errorf("bad_params", "%v", err)
	}
	return r.Start, r.End, nil
}

func lastVerse(book int) verse.ID {
	counts := chapterVerses[book]
	return verseID(book, len(counts), counts[len(counts)-1])
}

// split cuts units into n runs of consecutive units, each with about the same weight. Every run
// has at least one unit.
func split(units []unit, n int) ([][]unit, error) {
	if n < 1 {
		return nil, command.Errorf("bad_params", "a plan needs at least one day")
	}
	if n > len(units) {
		return nil, command.Errorf("bad_params", "there are only %d chapters or sections to read; use at most that many days", len(units))
	}
	total := 0
	for _, u := range units {
		total += u.weight
	}
	out := make([][]unit, 0, n)
	idx, cum := 0, 0
	for i := 0; i < n; i++ {
		start := idx
		if i == n-1 {
			idx = len(units)
		} else {
			target := total * (i + 1) / n
			idx++ // at least one unit
			cum += units[start].weight
			for idx < len(units)-(n-1-i) && abs(cum+units[idx].weight-target) < abs(cum-target) {
				cum += units[idx].weight
				idx++
			}
		}
		out = append(out, units[start:idx])
	}
	return out, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// span is a run of verses in one book, possibly over several chapters.
type span struct{ book, c1, v1, c2, v2 int }

// bibleReading describes the verses from start to end.
func bibleReading(start, end verse.ID) Reading {
	shown := end
	if counts := chapterVerses[end.Book()]; end.Chapter() <= len(counts) && end.Verse() == counts[end.Chapter()-1] {
		shown = verseID(end.Book(), end.Chapter(), verse.EndOfChapter) // whole chapters read as "Genesis 1-3"
	}
	n := 0
	for _, u := range mustUnits(start, end) {
		n += u.weight
	}
	return Reading{Ref: verse.Range{Start: start, End: shown}.String(), Start: start, End: end,
		OSIS: start.OSIS() + "-" + end.OSIS(), Verses: n}
}

func mustUnits(start, end verse.ID) []unit {
	u, _ := rangeUnits(start, end)
	return u
}

// readings turns a day's units into readings, joining neighbouring chapters of one book.
func readings(units []unit) []Reading {
	var out []Reading
	var cur *span
	flush := func() {
		if cur != nil {
			out = append(out, bibleReading(verseID(cur.book, cur.c1, cur.v1), verseID(cur.book, cur.c2, cur.v2)))
			cur = nil
		}
	}
	for _, u := range units {
		if u.book != nil {
			flush()
			out = append(out, Reading{Ref: fmt.Sprintf("%s, section %d", u.book.title, u.book.section),
				ItemID: u.book.item, Section: u.book.section, Title: u.book.title})
			continue
		}
		p := u.bible
		if cur != nil && cur.book == p.book && cur.v2 == chapterVerses[p.book][cur.c2-1] && p.chapter == cur.c2+1 && p.from == 1 {
			cur.c2, cur.v2 = p.chapter, p.to
			continue
		}
		flush()
		cur = &span{p.book, p.chapter, p.from, p.chapter, p.to}
	}
	flush()
	return out
}
