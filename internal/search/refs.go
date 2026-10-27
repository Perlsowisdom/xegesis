package search

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// citation is a Bible reference found in text, with the byte span of the whole citation.
type citation struct {
	r          verse.Range
	start, end int
}

// A candidate reference is a book name (optionally numbered, possibly "X of Y" or abbreviated
// with a dot) followed by chapter:verse, an optional range, and further verses or chapters after
// commas or semicolons: "Rom 8:28", "1 Cor. 13:4-7", "John 3:16-18; 4:2", "Ps 23:1, 4".
var (
	refRe = regexp.MustCompile(`\b((?:[1-4]|I{1,3}|IV|First|Second|Third)\s?)?` +
		`([A-Z][a-z]{1,15}(?:\s+of\s+[A-Z][a-z]+)?)\.?\s?` +
		`(\d{1,3}:\d{1,3}(?:\s?[-–]\s?\d{1,3}(?::\d{1,3})?)?)` +
		`((?:\s?[,;]\s?\d{1,3}(?::\d{1,3})?(?:\s?[-–]\s?\d{1,3}(?::\d{1,3})?)?\b)*)`)
	moreRe = regexp.MustCompile(`([,;])\s?(\d{1,3}(?::\d{1,3})?(?:\s?[-–]\s?\d{1,3}(?::\d{1,3})?)?)`)
)

// findRefs returns the Bible references cited in text.
func findRefs(text string) []citation {
	var out []citation
	for _, m := range refRe.FindAllStringSubmatchIndex(text, -1) {
		b, ok := lookupBook(sub(text, m, 1), sub(text, m, 2))
		if !ok {
			continue
		}
		r, err := verse.Parse(b.OSIS + " " + strings.ReplaceAll(sub(text, m, 3), " ", ""))
		if err != nil {
			continue
		}
		out = append(out, citation{r: r, start: m[0], end: m[1]})
		chapter := r.End.Chapter()
		for _, mm := range moreRe.FindAllStringSubmatch(sub(text, m, 4), -1) {
			nums := strings.ReplaceAll(mm[2], " ", "")
			if !strings.Contains(nums, ":") && mm[1] == "," {
				nums = strconv.Itoa(chapter) + ":" + nums // "8:28, 30" is verse 30 of chapter 8
			}
			if r, err := verse.Parse(b.OSIS + " " + nums); err == nil {
				chapter = r.End.Chapter()
				out = append(out, citation{r: r, start: m[0], end: m[1]})
			}
		}
	}
	return out
}

// lookupBook finds the book named by a numeral and a name; for "Epistle of James" it falls back
// to the words after "of".
func lookupBook(num, name string) (verse.Book, bool) {
	if b, ok := verse.LookupBook(strings.TrimSpace(num + " " + name)); ok {
		return b, true
	}
	if _, after, ok := strings.Cut(name, " of "); ok && num == "" {
		return verse.LookupBook(strings.TrimSpace(after))
	}
	return verse.Book{}, false
}

func sub(s string, m []int, i int) string {
	if m[2*i] < 0 {
		return ""
	}
	return s[m[2*i]:m[2*i+1]]
}
