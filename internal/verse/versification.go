package verse

import "fmt"

// System is a verse-numbering tradition. Verse IDs are stored in the numbering of the text they
// come from, with the system recorded beside them, and converted when texts are compared.
type System string

const (
	// English is the numbering of most English Bibles.
	English System = "english"
	// Hebrew is the numbering of printed Hebrew Bibles (Masoretic Text).
	Hebrew System = "hebrew"
)

// shift says that English verses chapter:from..to are Hebrew verses toChapter:toFrom onward.
type shift struct {
	book              int
	chapter, from, to int
	toChapter, toFrom int
}

// englishToHebrew lists well-known places where English and Hebrew numbering differ. These are facts
// of the texts, documented in standard reference works. Psalm superscriptions, which add a verse in
// many Hebrew psalms, are handled by the full mapping tables loaded with Bible modules (task T07).
var englishToHebrew = []shift{
	{1, 31, 55, 55, 32, 1}, {1, 32, 1, 32, 32, 2},
	{2, 8, 1, 4, 7, 26}, {2, 8, 5, 32, 8, 1},
	{2, 22, 1, 1, 21, 37}, {2, 22, 2, 31, 22, 1},
	{3, 6, 1, 7, 5, 20}, {3, 6, 8, 30, 6, 1},
	{4, 16, 36, 50, 17, 1}, {4, 17, 1, 13, 17, 16},
	{5, 29, 1, 1, 28, 69}, {5, 29, 2, 29, 29, 1},
	{10, 18, 33, 33, 19, 1}, {10, 19, 1, 43, 19, 2},
	{11, 4, 21, 34, 5, 1}, {11, 5, 1, 18, 5, 15},
	{13, 6, 1, 15, 5, 27}, {13, 6, 16, 81, 6, 1},
	{16, 4, 1, 6, 3, 33}, {16, 4, 7, 23, 4, 1},
	{18, 41, 1, 8, 40, 25}, {18, 41, 9, 34, 41, 1},
	{21, 5, 1, 1, 4, 17}, {21, 5, 2, 20, 5, 1},
	{22, 6, 13, 13, 7, 1}, {22, 7, 1, 13, 7, 2},
	{23, 9, 1, 1, 8, 23}, {23, 9, 2, 21, 9, 1},
	{26, 20, 45, 49, 21, 1}, {26, 21, 1, 32, 21, 6},
	{27, 4, 1, 3, 3, 31}, {27, 4, 4, 37, 4, 1},
	{27, 5, 31, 31, 6, 1}, {27, 6, 1, 28, 6, 2},
	{28, 11, 12, 12, 12, 1}, {28, 12, 1, 14, 12, 2},
	{28, 13, 16, 16, 14, 1}, {28, 14, 1, 9, 14, 2},
	{29, 2, 28, 32, 3, 1}, {29, 3, 1, 21, 4, 1},
	{32, 1, 17, 17, 2, 1}, {32, 2, 1, 10, 2, 2},
	{33, 5, 1, 1, 4, 14}, {33, 5, 2, 15, 5, 1},
	{38, 1, 18, 21, 2, 1}, {38, 2, 1, 13, 2, 5},
	{39, 4, 1, 6, 3, 19},
}

// Convert maps a verse from one numbering to another. Verses with no listed difference keep their
// number.
func Convert(id ID, from, to System) (ID, error) {
	switch {
	case from == to:
		return id, nil
	case from == English && to == Hebrew:
		return apply(id, englishToHebrew, false), nil
	case from == Hebrew && to == English:
		return apply(id, englishToHebrew, true), nil
	}
	return 0, fmt.Errorf("verse: no mapping from %s to %s", from, to)
}

// ConvertRange maps both ends of a range.
func ConvertRange(r Range, from, to System) (Range, error) {
	s, err := Convert(r.Start, from, to)
	if err != nil {
		return Range{}, err
	}
	e, err := Convert(r.End, from, to)
	return Range{s, e}, err
}

func apply(id ID, shifts []shift, reverse bool) ID {
	b, c, v := id.Book(), id.Chapter(), id.Verse()
	for _, s := range shifts {
		if s.book != b {
			continue
		}
		fromCh, fromV, toCh, toV := s.chapter, s.from, s.toChapter, s.toFrom
		last := s.from + (s.to - s.from)
		if reverse {
			fromCh, fromV, toCh, toV = s.toChapter, s.toFrom, s.chapter, s.from
			last = s.toFrom + (s.to - s.from)
		}
		if c == fromCh && v >= fromV && v <= last {
			return ID(b*1_000_000 + toCh*1_000 + toV + (v - fromV))
		}
	}
	return id
}
