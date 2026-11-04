package export

import "strings"

// A document is built once from its sources and then written by each format's renderer, so print,
// PDF, Word, HTML and Markdown always hold the same content, cuts and credits.

// Span is a run of text with one set of marks.
type Span struct {
	Text   string `json:"text"`
	Bold   bool   `json:"bold,omitempty"`
	Italic bool   `json:"italic,omitempty"`
	Code   bool   `json:"code,omitempty"`
	Under  bool   `json:"underline,omitempty"`
	Link   string `json:"link,omitempty"`
	Break  bool   `json:"br,omitempty"` // a line break inside a block; Text is ""
}

type Kind int

const (
	Para Kind = iota
	Heading
	Quote
	Bullet
	Numbered
	Verse  // Num is the verse number
	Table  // side-by-side Bibles
	Notice // what was left out and why
)

// Block is one paragraph-like unit.
type Block struct {
	Kind  Kind
	Level int    // Heading: 1 to 3
	Num   string // Verse: the verse number; Numbered: the item number
	Spans []Span
	Table *Grid
}

// Grid is parallel Bibles side by side: one column per Bible, one row per verse.
type Grid struct {
	Head []string
	Rows []GridRow
}

type GridRow struct {
	Num   string
	Cells [][]Span
}

// Piece is one source's content with its credits.
type Piece struct {
	Title    string
	Subtitle string
	Blocks   []Block
	Credits  []string
}

// Doc is a whole print or export.
type Doc struct {
	Title  string
	Pieces []Piece
	Opt    Options
}

func plain(s string) []Span {
	if s == "" {
		return nil
	}
	return []Span{{Text: s}}
}

func spansText(sp []Span) string {
	var b strings.Builder
	for _, s := range sp {
		if s.Break {
			b.WriteByte('\n')
		}
		b.WriteString(s.Text)
	}
	return b.String()
}

// credits lists every credit line once, in order, for a credits section at the end.
func (d Doc) credits() []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range d.Pieces {
		for _, c := range p.Credits {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// creditsAtEnd reports whether credits go in one section at the end rather than under each piece.
func (d Doc) creditsAtEnd() bool { return d.Opt.Credits == "end" }

// showTitle reports whether a piece's title is shown: always when there are several pieces, and
// otherwise only when the document's title does not already say it.
func (d Doc) showTitle(p Piece) bool {
	return p.Title != "" && (len(d.Pieces) > 1 || !strings.HasPrefix(d.Title, p.Title))
}
