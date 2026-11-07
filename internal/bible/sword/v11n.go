package sword

import (
	"embed"
	"fmt"
	"path"
	"sort"
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

//go:embed v11n/*.txt
var v11nFiles embed.FS

// Book is one book of a numbering with the verse count of each chapter.
type Book struct {
	Number int    // book number in internal/verse; 0 for a book Xegesis does not know, which is skipped
	OSIS   string // the book's ID in the numbering
	Verses []int
}

// Versification is a verse numbering: the books of each testament in index order.
type Versification struct {
	Name   string
	System verse.System // the numbering Xegesis records for modules in it
	OT, NT []Book
}

// numberings lists the supported SWORD numberings: the .conf name, the counts file and the
// Xegesis numbering recorded for modules in it. MT and Leningrad follow the Hebrew text and use
// Xegesis's hebrew numbering; the others are recorded under their own lower-case name.
var numberings = []struct {
	name, file string
	system     verse.System
}{
	{"KJV", "", verse.English},
	{"NRSV", "nrsv", "nrsv"},
	{"MT", "mt", verse.Hebrew},
	{"Leningrad", "leningrad", verse.Hebrew},
	{"Synodal", "synodal", "synodal"},
	{"Vulg", "vulg", "vulg"},
	{"Catholic", "catholic", "catholic"},
	{"Catholic2", "catholic2", "catholic2"},
	{"German", "german", "german"},
	{"Luther", "luther", "luther"},
	{"Calvin", "calvin", "calvin"},
}

var versifications = map[string]*Versification{}

func init() {
	for _, n := range numberings {
		data := kjvCounts
		if n.file != "" {
			b, err := v11nFiles.ReadFile(path.Join("v11n", n.file+".txt"))
			if err != nil {
				panic(err)
			}
			data = string(b)
		}
		v, err := parseCounts(n.name, n.system, data)
		if err != nil {
			panic(err)
		}
		versifications[strings.ToLower(n.name)] = v
	}
}

// Versifications lists the SWORD numberings Xegesis can read.
func Versifications() []string {
	out := make([]string, 0, len(numberings))
	for _, n := range numberings {
		out = append(out, n.name)
	}
	sort.Strings(out)
	return out
}

// LookupVersification finds a numbering by its .conf name; "" is KJV, the documented default.
func LookupVersification(name string) (*Versification, bool) {
	if name == "" {
		name = "KJV"
	}
	v, ok := versifications[strings.ToLower(strings.TrimSpace(name))]
	return v, ok
}

// parseCounts reads a counts file. Lines after "[ot]" or "[nt]" belong to that testament; a file
// without sections (kjv.txt) is split by book number.
func parseCounts(name string, sys verse.System, data string) (*Versification, error) {
	v := &Versification{Name: name, System: sys}
	section := ""
	for _, line := range strings.Split(data, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if f[0] == "[ot]" || f[0] == "[nt]" {
			section = f[0]
			continue
		}
		book := Book{OSIS: f[0]}
		if b, ok := verse.LookupBook(f[0]); ok {
			book.Number = b.Number
			if len(f)-1 > b.Chapters {
				return nil, fmt.Errorf("sword: %s numbering: %s has %d chapters, more than Xegesis allows",
					name, f[0], len(f)-1)
			}
		}
		for _, s := range f[1:] {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 || n > 998 {
				return nil, fmt.Errorf("sword: %s numbering: bad count %q in %s", name, s, f[0])
			}
			book.Verses = append(book.Verses, n)
		}
		switch {
		case section == "[nt]", section == "" && book.Number >= 40:
			v.NT = append(v.NT, book)
		case section == "[ot]", section == "" && book.Number != 0:
			v.OT = append(v.OT, book)
		default:
			return nil, fmt.Errorf("sword: %s numbering: unknown book %q", name, f[0])
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

// Book finds a book of the numbering by its Xegesis number.
func (v *Versification) Book(number int) (Book, bool) {
	for _, books := range [][]Book{v.OT, v.NT} {
		for _, b := range books {
			if b.Number == number && number != 0 {
				return b, true
			}
		}
	}
	return Book{}, false
}

// slot says what one index entry holds. id is 0 for entries of books Xegesis does not know.
type slot struct {
	id    verse.ID // the verse, or for an introduction the first verse after it
	intro bool
}

// slots lists a testament's index entries in order.
func (v *Versification) slots(testament int) []slot {
	books := v.books(testament)
	out := make([]slot, 0, v.Entries(testament))
	first := verse.ID(0)
	for _, b := range books {
		if b.Number != 0 {
			first = mustID(b.Number, 1, 1)
			break
		}
	}
	out = append(out, slot{first, true}, slot{first, true})
	for _, b := range books {
		id := func(c, vs int) verse.ID {
			if b.Number == 0 {
				return 0
			}
			return mustID(b.Number, c, vs)
		}
		out = append(out, slot{id(1, 1), true})
		for c, n := range b.Verses {
			out = append(out, slot{id(c+1, 1), true})
			for vs := 1; vs <= n; vs++ {
				out = append(out, slot{id(c+1, vs), false})
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
