package apibible

import (
	"errors"
	"fmt"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// usfm maps Xegesis book numbers to the three-character book codes (USFM) API.Bible uses in IDs.
var usfm = map[int]string{
	1: "GEN", 2: "EXO", 3: "LEV", 4: "NUM", 5: "DEU", 6: "JOS", 7: "JDG", 8: "RUT", 9: "1SA", 10: "2SA",
	11: "1KI", 12: "2KI", 13: "1CH", 14: "2CH", 15: "EZR", 16: "NEH", 17: "EST", 18: "JOB", 19: "PSA",
	20: "PRO", 21: "ECC", 22: "SNG", 23: "ISA", 24: "JER", 25: "LAM", 26: "EZK", 27: "DAN", 28: "HOS",
	29: "JOL", 30: "AMO", 31: "OBA", 32: "JON", 33: "MIC", 34: "NAM", 35: "HAB", 36: "ZEP", 37: "HAG",
	38: "ZEC", 39: "MAL", 40: "MAT", 41: "MRK", 42: "LUK", 43: "JHN", 44: "ACT", 45: "ROM", 46: "1CO",
	47: "2CO", 48: "GAL", 49: "EPH", 50: "PHP", 51: "COL", 52: "1TH", 53: "2TH", 54: "1TI", 55: "2TI",
	56: "TIT", 57: "PHM", 58: "HEB", 59: "JAS", 60: "1PE", 61: "2PE", 62: "1JN", 63: "2JN", 64: "3JN",
	65: "JUD", 66: "REV",
	67: "TOB", 68: "JDT", 69: "ESG", 70: "WIS", 71: "SIR", 72: "BAR", 73: "LJE", 74: "S3Y", 75: "SUS",
	76: "BEL", 77: "1MA", 78: "2MA", 79: "3MA", 80: "4MA", 81: "MAN", 82: "1ES", 83: "2ES", 84: "PS2",
}

// BookCode returns API.Bible's book code for a Xegesis book number.
func BookCode(book int) (string, error) {
	c, ok := usfm[book]
	if !ok {
		return "", fmt.Errorf("apibible: no book %d", book)
	}
	return c, nil
}

// ErrOpenEnd is returned for a range that stops at "the end of a chapter" part-way through a
// chapter, because API.Bible needs the explicit last verse number.
var ErrOpenEnd = errors.New("apibible: give an explicit last verse (for example Rom 8:28-39)")

// maxChapters caps how many whole chapters one request may fetch, to protect the monthly quota.
const maxChapters = 5

// Target is one API.Bible request that covers part of a reference: a chapter or a passage.
type Target struct {
	Chapter bool   // ID is a chapter ID such as "ROM.8", fetched from the chapters endpoint
	ID      string // chapter ID, or passage ID such as "ROM.8.28-ROM.8.30"
}

// Targets converts a Xegesis range into API.Bible requests. Whole chapters use chapter IDs, since
// passage IDs need a concrete last verse; everything else becomes one passage ID.
func Targets(r verse.Range) ([]Target, error) {
	sb, eb := r.Start.Book(), r.End.Book()
	sc, ec := r.Start.Chapter(), r.End.Chapter()
	scode, err := BookCode(sb)
	if err != nil {
		return nil, err
	}
	ecode, err := BookCode(eb)
	if err != nil {
		return nil, err
	}
	wholeChapters := r.Start.Verse() <= 1 && r.End.Verse() == verse.EndOfChapter
	if wholeChapters && sb == eb {
		if ec-sc+1 > maxChapters {
			return nil, fmt.Errorf("apibible: at most %d whole chapters per request", maxChapters)
		}
		var out []Target
		for c := sc; c <= ec; c++ {
			out = append(out, Target{Chapter: true, ID: fmt.Sprintf("%s.%d", scode, c)})
		}
		return out, nil
	}
	if r.End.Verse() == verse.EndOfChapter {
		return nil, ErrOpenEnd
	}
	sv := max(r.Start.Verse(), 1)
	id := fmt.Sprintf("%s.%d.%d-%s.%d.%d", scode, sc, sv, ecode, ec, r.End.Verse())
	return []Target{{ID: id}}, nil
}

// PassageID converts a range with explicit verses into a passage ID such as "ROM.8.28-ROM.8.30".
func PassageID(r verse.Range) (string, error) {
	t, err := Targets(r)
	if err != nil {
		return "", err
	}
	if len(t) != 1 || t[0].Chapter {
		return "", ErrOpenEnd
	}
	return t[0].ID, nil
}
