package reader

import (
	"context"
	"fmt"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/search"
)

// CharsPerPage is the page equivalent that licence limits on book text are measured in.
const CharsPerPage = charsPerPage

type ExcerptParams struct {
	BookParams
	Text string `json:"text,omitempty" desc:"Selected text; or give from and to for whole sections; or neither for only the licence and length"`
	From int    `json:"from,omitempty" desc:"First section (EPUB chapter or PDF page) to include"`
	To   int    `json:"to,omitempty" desc:"Last section to include; default from"`
}

// ExcerptBlock is one paragraph or heading of an excerpt.
type ExcerptBlock struct {
	Section int    `json:"section,omitempty"`
	Text    string `json:"text"`
	Heading bool   `json:"heading,omitempty"`
}

// Excerpt is book text ready for printing or exporting, with what its licence needs.
type Excerpt struct {
	ItemID      string          `json:"item_id"`
	FileID      string          `json:"file_id"`
	Title       string          `json:"title"`
	Authors     []string        `json:"authors"`
	Format      string          `json:"format"`
	Licence     licence.Licence `json:"licence" desc:"The licence that governs printing and exporting"`
	Attribution string          `json:"attribution,omitempty"`
	TotalPages  int             `json:"total_pages" desc:"The book's length in pages, for percentage limits"`
	Where       string          `json:"where" desc:"Which part of the book, in words"`
	Blocks      []ExcerptBlock  `json:"blocks"`
}

func (s *service) excerptCmd(ctx context.Context, env *command.Env, p ExcerptParams) (Excerpt, error) {
	b, err := s.open(ctx, env, p.ItemID, p.FileID)
	if err != nil {
		return Excerpt{}, err
	}
	lic := effectiveLicence(b.item)
	out := Excerpt{ItemID: b.item.ID, FileID: b.file.ID, Title: b.item.Title, Authors: []string{}, Format: b.file.Format,
		Licence: lic, Attribution: lic.AttributionLine(), Blocks: []ExcerptBlock{}}
	for _, pe := range b.item.People {
		if pe.Role == "author" {
			out.Authors = append(out.Authors, pe.Name)
		}
	}
	if out.TotalPages, err = s.bookPages(b); err != nil {
		return Excerpt{}, err
	}
	if strings.TrimSpace(p.Text) == "" && p.From < 1 {
		return out, nil // only the licence and length
	}
	if strings.TrimSpace(p.Text) != "" {
		out.Where = "Selected text"
		for _, para := range strings.Split(strings.ReplaceAll(p.Text, "\r\n", "\n"), "\n") {
			if t := strings.TrimSpace(para); t != "" {
				out.Blocks = append(out.Blocks, ExcerptBlock{Text: t})
			}
		}
		return out, nil
	}
	if p.To < p.From {
		p.To = p.From
	}
	secs, err := search.ReadSections(b.file.Path, b.file.Format)
	if err != nil {
		return Excerpt{}, unreadable(err)
	}
	for i, sec := range secs {
		n := sectionNumber(sec.Loc, i)
		if n < p.From || n > p.To {
			continue
		}
		for _, bl := range sec.Blocks {
			out.Blocks = append(out.Blocks, ExcerptBlock{Section: n, Text: bl.Text, Heading: bl.Heading})
		}
	}
	out.Where = where(b.file.Format, p.From, p.To)
	return out, nil
}

func sectionNumber(l search.Locator, i int) int {
	switch {
	case l.Spine > 0:
		return l.Spine
	case l.Page > 0:
		return l.Page
	}
	return i + 1
}

func where(format string, from, to int) string {
	unit := "Section"
	if format == "pdf" {
		unit = "Page"
	}
	if from == to {
		return fmt.Sprintf("%s %d", unit, from)
	}
	return fmt.Sprintf("%ss %d–%d", unit, from, to)
}
