package reader

import (
	"context"
	"encoding/json"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// A highlight or note made in a book typed Bible also records the verses it falls in, so it shows
// in passage reports, "Notes on this passage" and the Highlights panel for them. The book's own
// locator is kept.

func (s *service) isBibleBook(ctx context.Context, env *command.Env, itemID string) bool {
	var n int
	err := env.Store.QueryRow(ctx, `SELECT COUNT(*) FROM item_facets WHERE item_id = ? AND facet = 'type' AND value = 'Bible'`, itemID).Scan(&n)
	return err == nil && n > 0
}

// bookVerses gives the verses a place in a Bible-type book covers, or false when the book is not a
// Bible, cannot be mapped, or the place lies outside any mapped chapter.
func (s *service) bookVerses(ctx context.Context, env *command.Env, itemID string, a Anchor) (verse.Range, bool) {
	if env.Store == nil || a.Para < 1 || a.Page > 0 || !s.isBibleBook(ctx, env, itemID) {
		return verse.Range{}, false
	}
	b, err := s.open(ctx, env, itemID, "")
	if err != nil {
		return verse.Range{}, false
	}
	m, err := s.bibleMap(ctx, env, BookParams{ItemID: itemID, FileID: b.file.ID})
	if err != nil || len(m.Entries) == 0 {
		return verse.Range{}, false
	}
	r, ok := renderedSection(b, a.Section)
	if !ok {
		return verse.Range{}, false
	}
	ep, eo := a.EndPara, a.EndOffset
	if ep == 0 {
		ep, eo = a.Para, a.Offset
	}
	if eo == 0 && ep > a.Para {
		ep, eo = ep-1, 1<<30 // a selection that ends at the start of a paragraph stops before it
	}
	start, ok1 := placeVerse(r, m.Entries, a.Section, a.Para, a.Offset, false)
	end, ok2 := placeVerse(r, m.Entries, a.Section, ep, eo, true)
	if !ok1 {
		return verse.Range{}, false
	}
	if !ok2 || end < start {
		end = start
	}
	return verse.Range{Start: start, End: end}, true
}

// placeVerse names the verse a position falls in. A position in a chapter with no readable verse
// numbers gives the chapter's first verse, or, as an end, the chapter's last.
func placeVerse(r rendered, entries []MapEntry, section, para, off int, end bool) (verse.ID, bool) {
	idx := -1
	for i, e := range entries {
		if e.Chapter == 0 || e.Section > section {
			continue
		}
		if e.Section == section && e.Anchor != "" {
			p := r.ids[e.Anchor]
			if p == 0 || p > para {
				continue
			}
		}
		idx = i
	}
	if idx < 0 {
		return 0, false
	}
	e := entries[idx]
	v, have := 0, false
	if e.Section == section {
		if from, to, ok := chapterBounds(r, entries, idx); ok {
			for _, m := range acceptedMarks(r.marks, from, to) {
				have = true
				if m.Para < para || m.Para == para && m.Offset <= off {
					v = m.N
				}
			}
		}
	}
	if v == 0 {
		v = 1
		if end && !have {
			v = verse.EndOfChapter
		}
	}
	return verse.ID(e.Book*1_000_000 + e.Chapter*1000 + v), true
}

// BookVerses is the command package's book linker: the locator is a note's or highlight's book place as JSON.
func (s *service) BookVerses(ctx context.Context, env *command.Env, itemID, locator string) (verse.Range, bool) {
	var a Anchor
	if json.Unmarshal([]byte(locator), &a) != nil {
		return verse.Range{}, false
	}
	return s.bookVerses(ctx, env, itemID, a)
}
