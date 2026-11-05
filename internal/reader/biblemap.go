package reader

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// A book whose type is Bible is read like a Bible: a reference goes to its place in the book. The
// place is found from the book's contents and headings, once per file version.

type bibleKey struct{ book, chapter int } // chapter 0 is the start of the book

// MapEntry is one place in a book that holds a chapter of the Bible.
type MapEntry struct {
	Book    int    `json:"book" desc:"Bible book number, 1 to 66"`
	Chapter int    `json:"chapter" desc:"Chapter; 0 for the start of the book"`
	Section int    `json:"section"`
	Anchor  string `json:"anchor,omitempty"`
	Title   string `json:"title" desc:"The contents entry it was read from"`
}

type BibleMap struct {
	ItemID   string     `json:"item_id"`
	FileID   string     `json:"file_id"`
	Entries  []MapEntry `json:"entries"`
	Unmapped []string   `json:"unmapped" desc:"Contents entries that could not be placed in the Bible"`
}

var (
	chapterWord = regexp.MustCompile(`(?i)^(?:chapter|chap\.?|ch\.?|psalm|ps\.?|psalms)?\s*(\d{1,3})\b`)
	bookPrefix  = regexp.MustCompile(`(?i)^(?:the\s+)?(?:book|gospel|epistle|letter)\s+(?:of|according to)\s+(?:st\.?\s+|saint\s+)?`)
)

// mapTitle reads a contents entry as a chapter ("Psalm 84", "Genesis 1"), a book ("The Book of
// Psalms"), or, under a book, a bare chapter ("84", "Chapter 3").
func mapTitle(title string, cur int) (book, chapter int, ok bool) {
	t := strings.Join(strings.Fields(title), " ")
	if r, err := verse.Parse(t); err == nil && r.Start.Chapter() > 0 && r.Start.Book() == r.End.Book() {
		return r.Start.Book(), r.Start.Chapter(), true
	}
	for _, name := range []string{t, bookPrefix.ReplaceAllString(t, "")} {
		if b, found := verse.LookupBook(strings.TrimRight(name, ".:")); found {
			return b.Number, 0, true
		}
	}
	if cur > 0 {
		if m := chapterWord.FindStringSubmatch(t); m != nil {
			n, _ := strconv.Atoi(m[1])
			if n > 0 && n <= 150 {
				return cur, n, true
			}
		}
	}
	return 0, 0, false
}

// buildBibleMap reads a book's contents in order. A bare chapter number belongs to the book named
// by the entry before it.
func buildBibleMap(toc []TOCEntry) (entries []MapEntry, unmapped []string) {
	cur := 0
	for _, e := range toc {
		b, c, ok := mapTitle(e.Title, cur)
		if !ok {
			unmapped = append(unmapped, e.Title)
			continue
		}
		if c == 0 || b != cur {
			cur = b
		}
		entries = append(entries, MapEntry{Book: b, Chapter: c, Section: e.Section, Anchor: e.Anchor, Title: e.Title})
	}
	return entries, unmapped
}

func (s *service) bibleMap(ctx context.Context, env *command.Env, p BookParams) (BibleMap, error) {
	b, err := s.open(ctx, env, p.ItemID, p.FileID)
	if err != nil {
		return BibleMap{}, err
	}
	key := b.file.ID + "@" + b.file.SHA256
	s.mu.Lock()
	m, ok := s.bibleMaps[key]
	s.mu.Unlock()
	if ok {
		return m, nil
	}
	book, err := s.openCmd(ctx, env, BookParams{ItemID: p.ItemID, FileID: b.file.ID})
	if err != nil {
		return BibleMap{}, err
	}
	m = BibleMap{ItemID: b.item.ID, FileID: b.file.ID, Entries: []MapEntry{}, Unmapped: []string{}}
	if b.file.Format != "pdf" {
		e, u := buildBibleMap(book.TOC)
		m.Entries = append(m.Entries, e...)
		m.Unmapped = append(m.Unmapped, u...)
	}
	s.mu.Lock()
	if len(s.bibleMaps) > 64 {
		clear(s.bibleMaps)
	}
	s.bibleMaps[key] = m
	s.mu.Unlock()
	return m, nil
}

func (s *service) bibleMapCmd(ctx context.Context, env *command.Env, p BookParams) (BibleMap, error) {
	return s.bibleMap(ctx, env, p)
}

type AtRefParams struct {
	BookParams
	Ref string `json:"ref" required:"true" desc:"A Bible reference, e.g. Psalm 84 or John 3:16"`
}

type AtRef struct {
	FileID  string `json:"file_id"`
	Section int    `json:"section"`
	Anchor  string `json:"anchor,omitempty"`
	Found   string `json:"found" enum:"chapter,book,none" desc:"chapter: the chapter's own place; book: only the start of the book was found; none: the book does not hold it"`
	Title   string `json:"title,omitempty" desc:"The contents entry opened"`
}

func (s *service) atRefCmd(ctx context.Context, env *command.Env, p AtRefParams) (AtRef, error) {
	r, err := verse.Parse(p.Ref)
	if err != nil {
		if b, ok := verse.LookupBook(p.Ref); ok {
			r = verse.Range{Start: verse.ID(b.Number * 1_000_000), End: verse.ID(b.Number*1_000_000 + 999_999)}
		} else {
			return AtRef{}, command.Errorf("bad_params", "%v", err)
		}
	}
	m, err := s.bibleMap(ctx, env, p.BookParams)
	if err != nil {
		return AtRef{}, err
	}
	out := AtRef{FileID: m.FileID, Found: "none"}
	var book *MapEntry
	for i, e := range m.Entries {
		if e.Book != r.Start.Book() {
			continue
		}
		if e.Chapter == r.Start.Chapter() && e.Chapter > 0 {
			out.Section, out.Anchor, out.Found, out.Title = e.Section, e.Anchor, "chapter", e.Title
			return out, nil
		}
		if book == nil || e.Chapter == 0 {
			book = &m.Entries[i]
		}
	}
	if book != nil {
		out.Section, out.Anchor, out.Found, out.Title = book.Section, book.Anchor, "book", book.Title
	}
	return out, nil
}
