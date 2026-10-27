package sword

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// A SWORD verse index is positional: entry 0 is the module introduction, entry 1 the testament
// introduction, and then each book has an introduction entry followed, for each chapter, by a
// chapter introduction and one entry per verse. Mapping entries to verses therefore needs each
// numbering's book, chapter and verse counts.

//go:embed kjv.txt
var kjvCounts string

// Book is one book of a numbering with the verse count of each chapter.
type Book struct {
	Number int // book number in internal/verse
	Verses []int
}

// Versification is a verse numbering: the books of each testament in index order.
type Versification struct {
	Name   string
	System verse.System // the numbering Xegesis records for modules in it
	OT, NT []Book
}

var versifications = map[string]*Versification{}

func init() {
	v, err := parseCounts("KJV", verse.English, kjvCounts)
	if err != nil {
		panic(err)
	}
	versifications["kjv"] = v
}

// Versifications lists the SWORD numberings Xegesis can read.
func Versifications() []string { return []string{"KJV"} }

// LookupVersification finds a numbering by its .conf name; "" is KJV, the documented default.
func LookupVersification(name string) (*Versification, bool) {
	if name == "" {
		name = "KJV"
	}
	v, ok := versifications[strings.ToLower(name)]
	return v, ok
}

func parseCounts(name string, sys verse.System, data string) (*Versification, error) {
	v := &Versification{Name: name, System: sys}
	for _, line := range strings.Split(data, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		b, ok := verse.LookupBook(f[0])
		if !ok {
			return nil, fmt.Errorf("sword: %s numbering: unknown book %q", name, f[0])
		}
		book := Book{Number: b.Number}
		for _, s := range f[1:] {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 || n > 998 {
				return nil, fmt.Errorf("sword: %s numbering: bad count %q in %s", name, s, f[0])
			}
			book.Verses = append(book.Verses, n)
		}
		if b.Number <= 39 {
			v.OT = append(v.OT, book)
		} else {
			v.NT = append(v.NT, book)
		}
	}
	return v, nil
}

// Entries is the number of index entries a testament (0 Old, 1 New) has in this numbering.
func (v *Versification) Entries(testament int) int {
	n := 2
	for _, b := range v.books(testament) {
		n++
		for _, c := range b.Verses {
			n += 1 + c
		}
	}
	return n
}

func (v *Versification) books(testament int) []Book {
	if testament == 0 {
		return v.OT
	}
	return v.NT
}

// slot says what one index entry holds.
type slot struct {
	id    verse.ID // the verse, or for an introduction the first verse after it
	intro bool
}

// slots lists a testament's index entries in order.
func (v *Versification) slots(testament int) []slot {
	books := v.books(testament)
	out := make([]slot, 0, v.Entries(testament))
	first := verse.ID(0)
	if len(books) > 0 {
		first = mustID(books[0].Number, 1, 1)
	}
	out = append(out, slot{first, true}, slot{first, true})
	for _, b := range books {
		out = append(out, slot{mustID(b.Number, 1, 1), true})
		for c, n := range b.Verses {
			out = append(out, slot{mustID(b.Number, c+1, 1), true})
			for vs := 1; vs <= n; vs++ {
				out = append(out, slot{mustID(b.Number, c+1, vs), false})
			}
		}
	}
	return out
}

func mustID(b, c, v int) verse.ID {
	id, err := verse.Make(b, c, v)
	if err != nil {
		panic(err)
	}
	return id
}
