package reader

import (
	"context"
	"unicode/utf16"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/search"
)

type RefsParams struct {
	Texts []string `json:"texts" required:"true" desc:"Pieces of text, such as the lines of a PDF page"`
}

// FoundRef is one Bible reference in a piece of text. Start and End count UTF-16 code units, as
// JavaScript strings do, so the interface can mark the reference without converting.
type FoundRef struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Ref   string `json:"ref" desc:"The reference as OSIS, e.g. Rom.8.28"`
}

// refsCmd finds Bible references in text the core did not render itself, such as a PDF's text
// layer, using the same rules as EPUB chapters and the search index.
func (s *service) refsCmd(_ context.Context, _ *command.Env, p RefsParams) ([][]FoundRef, error) {
	out := make([][]FoundRef, len(p.Texts))
	for i, t := range p.Texts {
		out[i] = []FoundRef{}
		pos := 0
		for _, c := range search.FindRefs(t) {
			if c.Start < pos {
				continue // a further reference in a list already found
			}
			out[i] = append(out[i], FoundRef{Start: units(t[:c.Start]), End: units(t[:c.End]), Ref: osis(c)})
			pos = c.End
		}
	}
	return out, nil
}

// units is the length of s in UTF-16 code units.
func units(s string) int { return len(utf16.Encode([]rune(s))) }
