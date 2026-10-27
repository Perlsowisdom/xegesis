// Package verse defines verse IDs: integers BBCCCVVV (book, chapter, verse), so Romans 8:28 is
// 45008028. IDs sort in canonical order and ranges are simple integer comparisons.
package verse

import (
	"fmt"
	"strconv"
	"strings"
)

type ID int

// EndOfChapter is the verse number used for "to the end of the chapter" in ranges.
const EndOfChapter = 999

// Make builds an ID. Verse 0 is allowed for a chapter heading or superscription.
func Make(book, chapter, verse int) (ID, error) {
	b, ok := BookByNumber(book)
	if !ok {
		return 0, fmt.Errorf("verse: no book %d", book)
	}
	if chapter < 1 || chapter > b.Chapters {
		return 0, fmt.Errorf("verse: %s has no chapter %d", b.Name, chapter)
	}
	if verse < 0 || verse > EndOfChapter {
		return 0, fmt.Errorf("verse: verse %d out of range", verse)
	}
	return ID(book*1_000_000 + chapter*1_000 + verse), nil
}

func (id ID) Book() int    { return int(id) / 1_000_000 }
func (id ID) Chapter() int { return int(id) / 1_000 % 1_000 }
func (id ID) Verse() int   { return int(id) % 1_000 }

// Valid reports whether id names a real book and chapter.
func (id ID) Valid() bool {
	_, err := Make(id.Book(), id.Chapter(), id.Verse())
	return err == nil
}

// String formats id as "Romans 8:28".
func (id ID) String() string {
	b, ok := BookByNumber(id.Book())
	if !ok {
		return strconv.Itoa(int(id))
	}
	return fmt.Sprintf("%s %d:%d", b.Name, id.Chapter(), id.Verse())
}

// OSIS formats id as an OSIS reference, "Rom.8.28".
func (id ID) OSIS() string {
	b, _ := BookByNumber(id.Book())
	return fmt.Sprintf("%s.%d.%d", b.OSIS, id.Chapter(), id.Verse())
}

// Range is an inclusive span of verses.
type Range struct {
	Start ID `json:"start"`
	End   ID `json:"end"`
}

// Contains reports whether id falls in r.
func (r Range) Contains(id ID) bool { return id >= r.Start && id <= r.End }

// Overlaps reports whether two ranges share a verse.
func (r Range) Overlaps(o Range) bool { return r.Start <= o.End && o.Start <= r.End }

// String formats r compactly: "Romans 8:28", "Romans 8:28-30", "Romans 8", "Romans 8:28-9:2".
func (r Range) String() string {
	b, ok := BookByNumber(r.Start.Book())
	if !ok {
		return fmt.Sprintf("%d-%d", r.Start, r.End)
	}
	sc, sv, ec, ev := r.Start.Chapter(), r.Start.Verse(), r.End.Chapter(), r.End.Verse()
	switch {
	case r.End.Book() != r.Start.Book():
		return r.Start.String() + "-" + r.End.String()
	case sv <= 1 && ev == EndOfChapter && sc == ec:
		return fmt.Sprintf("%s %d", b.Name, sc)
	case sv <= 1 && ev == EndOfChapter:
		return fmt.Sprintf("%s %d-%d", b.Name, sc, ec)
	case r.Start == r.End:
		return r.Start.String()
	case sc == ec:
		return fmt.Sprintf("%s %d:%d-%d", b.Name, sc, sv, ev)
	}
	return fmt.Sprintf("%s %d:%d-%d:%d", b.Name, sc, sv, ec, ev)
}

// Parse reads a reference such as "Rom 8:28", "Romans 8:28-30", "Rom 8:28-9:2", "Rom 8", "Ps 23-24",
// "Jude 5" or the OSIS form "Rom.8.28". A whole chapter runs from verse 1 to EndOfChapter.
func Parse(ref string) (Range, error) {
	ref = strings.ReplaceAll(strings.TrimSpace(ref), "–", "-")
	if strings.Count(ref, ".") >= 2 && !strings.ContainsAny(ref, " :") {
		return parseOSIS(ref)
	}
	// The chapter and verse part at the end holds only digits, ':', '-' and spaces.
	i := len(ref)
	for i > 0 && strings.IndexByte("0123456789:- ", ref[i-1]) >= 0 {
		i--
	}
	name, nums := strings.TrimSpace(ref[:i]), strings.ReplaceAll(ref[i:], " ", "")
	if name == "" || nums == "" {
		return Range{}, fmt.Errorf("verse: cannot read %q", ref)
	}
	b, ok := LookupBook(name)
	if !ok {
		return Range{}, fmt.Errorf("verse: unknown book %q", name)
	}
	return parseNumbers(b, nums)
}

func parseNumbers(b Book, nums string) (Range, error) {
	startPart, endPart, isRange := strings.Cut(nums, "-")
	sc, sv, err := chapterVerse(b, startPart)
	if err != nil {
		return Range{}, err
	}
	hasVerse := strings.Contains(startPart, ":") || b.Chapters == 1
	start := Range{}
	if hasVerse {
		start.Start, err = Make(b.Number, sc, sv)
		start.End = start.Start
	} else {
		start.Start, err = Make(b.Number, sc, 1)
		if err == nil {
			start.End, err = Make(b.Number, sc, EndOfChapter)
		}
	}
	if err != nil || !isRange {
		return start, err
	}
	var ec, ev int
	switch {
	case strings.Contains(endPart, ":"):
		ec, ev, err = chapterVerse(b, endPart)
	case hasVerse:
		ec = sc
		ev, err = strconv.Atoi(endPart)
	default:
		ec, err = strconv.Atoi(endPart)
		ev = EndOfChapter
	}
	if err != nil {
		return Range{}, fmt.Errorf("verse: bad range end %q", endPart)
	}
	end, err := Make(b.Number, ec, ev)
	if err != nil {
		return Range{}, err
	}
	if end < start.Start {
		return Range{}, fmt.Errorf("verse: range ends before it starts")
	}
	return Range{start.Start, end}, nil
}

// chapterVerse reads "8:28", "8", or for single-chapter books "5" as verse 5.
func chapterVerse(b Book, s string) (int, int, error) {
	c, v, hasVerse := strings.Cut(s, ":")
	ch, err := strconv.Atoi(c)
	if err != nil {
		return 0, 0, fmt.Errorf("verse: bad chapter %q", c)
	}
	if !hasVerse {
		if b.Chapters == 1 {
			return 1, ch, nil
		}
		return ch, 0, nil
	}
	vs, err := strconv.Atoi(v)
	if err != nil {
		return 0, 0, fmt.Errorf("verse: bad verse %q", v)
	}
	return ch, vs, nil
}

func parseOSIS(ref string) (Range, error) {
	startRef, endRef, isRange := strings.Cut(ref, "-")
	one := func(s string) (ID, error) {
		p := strings.Split(s, ".")
		if len(p) != 3 {
			return 0, fmt.Errorf("verse: bad OSIS reference %q", s)
		}
		b, ok := LookupBook(p[0])
		if !ok {
			return 0, fmt.Errorf("verse: unknown book %q", p[0])
		}
		c, err1 := strconv.Atoi(p[1])
		v, err2 := strconv.Atoi(p[2])
		if err1 != nil || err2 != nil {
			return 0, fmt.Errorf("verse: bad OSIS reference %q", s)
		}
		return Make(b.Number, c, v)
	}
	start, err := one(startRef)
	if err != nil || !isRange {
		return Range{start, start}, err
	}
	end, err := one(endRef)
	if err != nil {
		return Range{}, err
	}
	return Range{start, end}, nil
}
