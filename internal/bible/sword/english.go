package sword

import (
	"slices"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Pair says that a verse in a SWORD numbering is a verse in Xegesis's english numbering.
type Pair struct{ Module, English verse.ID }

// EnglishMap lists the verses of this numbering whose english number differs from the one
// Xegesis would guess without a table (the same number, or for the hebrew numbering
// verse.Convert). Installed modules store it in their versification_map table.
//
// The table is worked out from the verse counts alone, using differences between numbering
// traditions that are facts of the texts:
//   - Psalms numbered as in the Greek and Latin Bibles (Psalm 9 holding English Psalms 9 and 10,
//     and so on), recognised by a Psalm 9 of more than 30 verses; Psalm 151 is the Psalm 151 book.
//   - Psalm titles counted as verses: a psalm with one or two verses more than the English one
//     has its first verses as the title (english verse 0) and the rest shifted down.
//   - Books whose chapters follow the Hebrew text (as verse.Convert describes) are converted
//     with it.
//   - Daniel with the Greek additions inside chapter 3 (verses 24-90 are the Prayer of Azariah)
//     and as chapters 13 and 14 (Susanna, and Bel and the Dragon), and Esther with the Greek
//     additions as chapters 10:4 to 16.
//
// Chapters that differ from the English ones in any other way keep their numbers; Approximate
// lists them.
func (v *Versification) EnglishMap() []Pair {
	if v.System == verse.English {
		return nil
	}
	m := newMapper(v)
	var out []Pair
	for _, books := range [][]Book{v.OT, v.NT} {
		for _, b := range books {
			if b.Number == 0 {
				continue
			}
			for c, n := range b.Verses {
				for vs := 1; vs <= n; vs++ {
					id := mustID(b.Number, c+1, vs)
					e := m.english(b, c+1, vs)
					if e != m.fallback(id) {
						out = append(out, Pair{id, e})
					}
				}
			}
		}
	}
	return out
}

// Approximate lists the chapters EnglishMap could not line up with the English numbering, as
// verse IDs of their first verse.
func (v *Versification) Approximate() []verse.ID {
	if v.System == verse.English {
		return nil
	}
	m := newMapper(v)
	return m.approx
}

type mapper struct {
	v      *Versification
	kjv    *Versification
	hebrew map[int][]int // KJV books laid out in the Hebrew chapters, by book number
	greek  bool          // Psalms in the Greek numbering
	plan   map[int]func(c, vs int) verse.ID
	approx []verse.ID
}

func newMapper(v *Versification) *mapper {
	kjv, _ := LookupVersification("KJV")
	m := &mapper{v: v, kjv: kjv, hebrew: hebrewLayout(kjv), plan: map[int]func(int, int) verse.ID{}}
	if ps, ok := v.Book(19); ok && len(ps.Verses) >= 9 && ps.Verses[8] > 30 {
		m.greek = true
	}
	for _, books := range [][]Book{v.OT, v.NT} {
		for _, b := range books {
			if b.Number != 0 {
				m.plan[b.Number] = m.planBook(b)
			}
		}
	}
	return m
}

// fallback is what Xegesis assumes without a table.
func (m *mapper) fallback(id verse.ID) verse.ID {
	if c, err := verse.Convert(id, m.v.System, verse.English); err == nil {
		return c
	}
	return id
}

func (m *mapper) english(b Book, c, vs int) verse.ID { return m.plan[b.Number](c, vs) }

// hebrewLayout lays the KJV verses out in the Hebrew chapters with verse.Convert.
func hebrewLayout(kjv *Versification) map[int][]int {
	out := map[int][]int{}
	for _, b := range kjv.OT {
		var counts []int
		for c, n := range b.Verses {
			for vs := 1; vs <= n; vs++ {
				h, _ := verse.Convert(mustID(b.Number, c+1, vs), verse.English, verse.Hebrew)
				for len(counts) < h.Chapter() {
					counts = append(counts, 0)
				}
				counts[h.Chapter()-1] = max(counts[h.Chapter()-1], h.Verse())
			}
		}
		out[b.Number] = counts
	}
	return out
}

func (m *mapper) planBook(b Book) func(c, vs int) verse.ID {
	same := func(c, vs int) verse.ID { return mustID(b.Number, c, vs) }
	k, inKJV := m.kjv.Book(b.Number)
	switch {
	case !inKJV:
		return same // books outside the KJV keep their numbers
	case b.Number == 19:
		return m.planPsalms(b, k)
	}
	counts, pre := b.Verses, func(c, vs int) (verse.ID, int, int) { return 0, c, vs }
	switch {
	case b.Number == 27 && len(b.Verses) >= 3 && b.Verses[2] >= 90:
		counts, pre = greekDaniel(b)
	case b.Number == 17 && len(b.Verses) == 16:
		counts, pre = greekEsther(b)
	}
	var rest func(c, vs int) verse.ID
	switch {
	case slices.Equal(counts, k.Verses):
		rest = same
	case slices.Equal(counts, m.hebrew[b.Number]):
		rest = func(c, vs int) verse.ID {
			e, _ := verse.Convert(mustID(b.Number, c, vs), verse.Hebrew, verse.English)
			return e
		}
	default:
		for c := range counts {
			if c >= len(k.Verses) || counts[c] != k.Verses[c] {
				m.approx = append(m.approx, mustID(b.Number, c+1, 1))
			}
		}
		rest = same
	}
	return func(c, vs int) verse.ID {
		id, c, vs := pre(c, vs)
		if id != 0 {
			return id
		}
		return rest(c, vs)
	}
}

// greekDaniel places the Greek additions of Daniel in their own books and returns the Hebrew-style
// counts of what is left. In the Greek and Latin text Daniel 3:24-90 is the Prayer of Azariah, and
// chapters 13 and 14 are Susanna and Bel and the Dragon.
func greekDaniel(b Book) ([]int, func(c, vs int) (verse.ID, int, int)) {
	counts := slices.Clone(b.Verses)
	counts[2] -= 67
	if len(counts) > 12 {
		counts = counts[:12]
	}
	return counts, func(c, vs int) (verse.ID, int, int) {
		switch {
		case c == 3 && vs >= 24 && vs <= 90:
			return mustID(74, 1, vs-23), c, vs
		case c == 3 && vs > 90:
			return 0, c, vs - 67
		case c == 13:
			return mustID(75, 1, vs), c, vs
		case c == 14:
			return mustID(76, 1, vs), c, vs
		}
		return 0, c, vs
	}
}

// greekEsther places Esther 10:4-16:24 (the Greek additions, as numbered in the Latin Bible) in
// Esther (Greek).
func greekEsther(b Book) ([]int, func(c, vs int) (verse.ID, int, int)) {
	counts := slices.Clone(b.Verses[:10])
	counts[9] = min(counts[9], 3)
	return counts, func(c, vs int) (verse.ID, int, int) {
		if c > 10 || c == 10 && vs > 3 {
			return mustID(69, c, vs), c, vs
		}
		return 0, c, vs
	}
}

// planPsalms maps the Psalms, in the Greek or Hebrew chapter numbering, with titles counted as verses.
func (m *mapper) planPsalms(b Book, k Book) func(c, vs int) verse.ID {
	eng := func(c int) int { // English verses in chapter c
		if c < 1 || c > len(k.Verses) {
			return 0
		}
		return k.Verses[c-1]
	}
	count := func(c int) int {
		if c < 1 || c > len(b.Verses) {
			return 0
		}
		return b.Verses[c-1]
	}
	// title is the number of title verses: the extra verses beyond the English count.
	title := func(c, english int) int {
		if d := count(c) - english; d == 1 || d == 2 {
			return d
		}
		return 0
	}
	at := func(c, vs int) verse.ID {
		if vs < 0 {
			vs = 0
		}
		return mustID(19, c, vs)
	}
	if !m.greek {
		for c := 1; c <= len(b.Verses) && c <= 150; c++ {
			if d := count(c) - eng(c); d != 0 && title(c, eng(c)) == 0 {
				m.approx = append(m.approx, mustID(19, c, 1))
			}
		}
		return func(c, vs int) verse.ID {
			if c > 150 {
				return mustID(84, 1, vs)
			}
			return at(c, vs-title(c, eng(c)))
		}
	}
	// one-to-one Greek chapters and their English chapter
	single := func(c int) int {
		switch {
		case c <= 8, c >= 148:
			return c
		case c >= 10 && c <= 112:
			return c + 1
		case c >= 116 && c <= 145:
			return c + 1
		}
		return 0
	}
	// merged: Greek 9 = English 9 + 10, Greek 113 = English 114 + 115
	merged := func(c, first, vs int) verse.ID {
		d := title(c, eng(first)+eng(first+1))
		vs -= d
		if vs <= eng(first) {
			return at(first, vs)
		}
		return at(first+1, vs-eng(first))
	}
	// split: Greek 114 + 115 = English 116 (split after verse 9), Greek 146 + 147 = English 147
	// (split after verse 11)
	split := func(c, english, before int, second bool, vs int) verse.ID {
		n := before
		if second {
			n = eng(english) - before
		}
		vs -= title(c, n)
		if second {
			vs += before
		}
		return at(english, vs)
	}
	for c := 1; c <= len(b.Verses) && c <= 150; c++ {
		if e := single(c); e != 0 && count(c) != eng(e) && title(c, eng(e)) == 0 {
			m.approx = append(m.approx, mustID(19, c, 1))
		}
	}
	return func(c, vs int) verse.ID {
		switch {
		case c > 150:
			return mustID(84, 1, vs)
		case c == 9:
			return merged(c, 9, vs)
		case c == 113:
			return merged(c, 114, vs)
		case c == 114 || c == 115:
			return split(c, 116, 9, c == 115, vs)
		case c == 146 || c == 147:
			return split(c, 147, 11, c == 147, vs)
		}
		e := single(c)
		return at(e, vs-title(c, eng(e)))
	}
}
