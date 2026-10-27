package search

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Locator says where in a file a passage starts, so a hit can open at the right place.
type Locator struct {
	Spine  int    `json:"spine,omitempty" desc:"EPUB: 1-based position of the chapter in the reading order"`
	Href   string `json:"href,omitempty" desc:"EPUB: the chapter's path inside the book"`
	CFI    string `json:"cfi,omitempty" desc:"EPUB: canonical fragment identifier of the chapter"`
	Page   int    `json:"page,omitempty" desc:"PDF: 1-based page number"`
	Para   int    `json:"para,omitempty" desc:"1-based paragraph number within the chapter, page or document"`
	Offset int    `json:"offset,omitempty" desc:"Character offset of the passage within the chapter, page or document text"`
}

// block is one paragraph or heading of extracted text.
type block struct {
	text    string
	heading bool
}

// section is a run of blocks that share a locator: an EPUB chapter, a PDF page or a whole document.
type section struct {
	loc    Locator
	blocks []block
}

// passage is one searchable chunk of about one or two paragraphs.
type passage struct {
	heading string
	text    string
	loc     Locator
}

const (
	minPassage = 300  // characters; shorter paragraphs are joined with the next
	maxPassage = 1500 // characters; longer paragraphs are split at sentence ends
	maxParas   = 2
)

// extractFile reads a file's text as sections.
func extractFile(path, format string) ([]section, error) {
	switch format {
	case "epub":
		return epubText(path)
	case "pdf":
		return pdfText(path)
	case "docx":
		return docxText(path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch format {
	case "html":
		return []section{{blocks: htmlBlocks(b)}}, nil
	case "md":
		return []section{{blocks: mdBlocks(string(b))}}, nil
	case "txt":
		return []section{{blocks: txtBlocks(string(b))}}, nil
	}
	return nil, fmt.Errorf("cannot read %s files", format)
}

// chunk splits sections into passages, each labelled with the latest heading. A section's text is
// its blocks joined by blank lines; offsets count characters in it.
func chunk(sections []section) []passage {
	var out []passage
	heading := ""
	for _, s := range sections {
		var cur []string
		curPara, curOff, off := 0, 0, 0
		flush := func() {
			if len(cur) > 0 {
				loc := s.loc
				loc.Para, loc.Offset = curPara, curOff
				out = append(out, passage{heading: heading, text: strings.Join(cur, "\n\n"), loc: loc})
				cur = nil
			}
		}
		for i, b := range s.blocks {
			if b.heading {
				flush()
				heading = b.text
			} else {
				pos := 0
				for _, piece := range splitLong(b.text) {
					at := pos + max(strings.Index(b.text[pos:], piece), 0)
					if len(cur) == 0 {
						curPara, curOff = i+1, off+utf8.RuneCountInString(b.text[:at])
					}
					pos = at + len(piece)
					cur = append(cur, piece)
					if len(cur) >= maxParas || runeLen(cur) >= minPassage {
						flush()
					}
				}
			}
			off += utf8.RuneCountInString(b.text) + 2
		}
		flush()
	}
	return out
}

func runeLen(parts []string) int {
	n := 0
	for _, p := range parts {
		n += utf8.RuneCountInString(p)
	}
	return n
}

var sentenceEnd = regexp.MustCompile(`[.!?;]["'”’)\]]*\s+`)

// splitLong cuts a paragraph longer than maxPassage at sentence ends, or at spaces if it has none.
func splitLong(s string) []string {
	var out []string
	for len(s) > maxPassage {
		cut := -1
		for _, m := range sentenceEnd.FindAllStringIndex(s[:maxPassage], -1) {
			if m[1] >= minPassage {
				cut = m[1]
			}
		}
		if cut < 0 {
			cut = strings.LastIndexByte(s[:maxPassage], ' ') + 1
		}
		if cut <= 0 {
			cut = maxPassage
			for cut < len(s) && !utf8.RuneStart(s[cut]) {
				cut++
			}
		}
		out = append(out, strings.TrimSpace(s[:cut]))
		s = strings.TrimSpace(s[cut:])
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

var (
	spaces     = regexp.MustCompile(`\s+`)
	blankLines = regexp.MustCompile(`\n[ \t]*\n`)
)

func clean(s string) string { return strings.TrimSpace(spaces.ReplaceAllString(s, " ")) }

// paragraphs splits plain text at blank lines and joins the lines within each paragraph.
func paragraphs(s string) []string {
	var out []string
	for _, p := range blankLines.Split(strings.ReplaceAll(s, "\r\n", "\n"), -1) {
		if p = clean(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func txtBlocks(s string) []block {
	var out []block
	for _, p := range paragraphs(s) {
		out = append(out, block{text: p})
	}
	return out
}

var (
	mdHeading = regexp.MustCompile(`^ {0,3}(#{1,6})\s+(.*?)\s*#*\s*$`)
	mdImage   = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	mdLink    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdMarks   = regexp.MustCompile("(\\*\\*|__|\\*|`+|~~)")
	mdPrefix  = regexp.MustCompile(`^\s*(>\s*)+|^\s*([-*+]|\d+[.)])\s+`)
)

// mdBlocks reads Markdown, skipping front matter and dropping inline markup.
func mdBlocks(s string) []block {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if strings.HasPrefix(s, "---\n") {
		if end := strings.Index(s[4:], "\n---"); end >= 0 {
			s = s[4+end+4:]
		}
	}
	var out []block
	var para []string
	flush := func() {
		if t := clean(strings.Join(para, " ")); t != "" {
			out = append(out, block{text: t})
		}
		para = nil
	}
	for _, line := range strings.Split(s, "\n") {
		if m := mdHeading.FindStringSubmatch(line); m != nil {
			flush()
			out = append(out, block{text: mdInline(m[2]), heading: true})
			continue
		}
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "```") {
			flush()
			continue
		}
		para = append(para, mdInline(mdPrefix.ReplaceAllString(line, "")))
	}
	flush()
	return out
}

func mdInline(s string) string {
	s = mdImage.ReplaceAllString(s, "$1")
	s = mdLink.ReplaceAllString(s, "$1")
	return clean(mdMarks.ReplaceAllString(s, ""))
}
