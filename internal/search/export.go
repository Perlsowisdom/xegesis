package search

import (
	"fmt"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// The book reader (internal/reader) shows books with the same paragraph numbering the index uses,
// so a search locator's Para points at the same paragraph on screen. These exports share it.

// Block is one paragraph or heading of a book's text.
type Block struct {
	Text    string
	Heading bool
}

// Section is an EPUB chapter, a PDF page or a whole document, with its locator.
type Section struct {
	Loc    Locator
	Blocks []Block
}

// ReadSections extracts a file's text exactly as indexing does. A file the extractors cannot cope
// with is an error, not a crash.
func ReadSections(path, format string) (out []Section, err error) {
	defer func() {
		if x := recover(); x != nil {
			out, err = nil, fmt.Errorf("cannot read %s: %v", path, x)
		}
	}()
	secs, err := extractFile(path, format)
	if err != nil {
		return nil, err
	}
	for _, s := range secs {
		bs := make([]Block, len(s.blocks))
		for i, b := range s.blocks {
			bs[i] = Block{Text: b.text, Heading: b.heading}
		}
		out = append(out, Section{Loc: s.loc, Blocks: bs})
	}
	return out, nil
}

// Citation is a Bible reference found in text; Start and End are the byte span of the citation.
// A list such as "John 3:16; 4:2" gives one Citation per reference, all with the list's span.
type Citation struct {
	Range      verse.Range
	Start, End int
}

// FindRefs returns the Bible references cited in text, as indexing finds them.
func FindRefs(text string) []Citation {
	var out []Citation
	for _, c := range findRefs(text) {
		out = append(out, Citation{Range: c.r, Start: c.start, End: c.end})
	}
	return out
}

// HTML elements that start or end a paragraph, headings, and elements with no readable text, as
// indexing treats them.
func IsBlockTag(tag string) bool   { return blockTags[tag] }
func IsHeadingTag(tag string) bool { return headingTags[tag] }
func IsSkipTag(tag string) bool    { return skipTags[tag] }
