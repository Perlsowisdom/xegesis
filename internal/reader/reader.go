// Package reader opens library books for reading: tables of contents, one section at a time as
// sanitised HTML (EPUB chapters, documents) or page information and bytes (PDF), search locators
// turned into positions, highlights, the reading position, and copying within the licence.
//
// Book content never reaches the interface unsanitised: scripts, styles and remote resources are
// removed, links between chapters become in-book jumps, and images come only from inside the book
// file, as data URIs.
package reader

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/library"
)

// Register adds the reading commands to r.
func Register(r *command.Registry) { register(r, newService(r)) }

func register(r *command.Registry, s *service) {
	core.BookVerses = s.linkVerses
	r.Add(
		command.Define("book.open", "Open a library book: its format, number of sections or pages, table of contents and saved reading position.", false, s.openCmd),
		command.Define("book.section", "Read one section of a book: an EPUB chapter or a whole document as sanitised HTML, or a PDF page's size.", false, s.sectionCmd),
		command.Define("book.file", "Read part of a PDF book's bytes, for the interface's PDF renderer.", false, s.fileCmd),
		command.Define("book.locate", "Turn a search locator from search.query or search.refs into a section and paragraph to open at.", false, s.locateCmd),
		command.Define("book.set_position", "Remember where the user is reading in a book.", true, s.setPositionCmd),
		command.Define("book.copy", "Copy text from a book within its licence: the per-copy and per-day export limit applies and the attribution line is added.", true, s.copyCmd),
		command.Define("book.excerpt", "Read part of a book for printing or exporting: a selection or whole sections, with its licence and length.", false, s.excerptCmd),
		command.Define("book.bible_map", "Map a Bible-type book to the Bible: which chapters its contents and headings place, and which entries could not be placed.", false, s.bibleMapCmd),
		command.Define("book.at_reference", "Find where a Bible-type book holds a reference: the section and anchor of that chapter, or of the book when the chapter is not found.", false, s.atRefCmd),
		command.Define("book.verse_text", "Read the text of verses from a Bible-type book, found by its chapter map and verse numbers, within one chapter; the text is not licence-checked, book.copy does that.", false, s.verseTextCmd),
		command.Define("book.refs", "Find Bible references in pieces of text, such as the lines of a PDF page, so they can become links.", false, s.refsCmd),
		command.Define("highlight.create", "Highlight a passage in a book, Bible verses, or exact words within verses.", true, s.highlightCreate),
		command.Define("highlight.list", "List highlights: in a book, on Bible verses, in a book of the Bible or everywhere, by colour or words.", false, s.highlightList),
		command.Define("highlight.update", "Change a highlight's colour, whether it underlines, or the exact words of a Bible highlight.", true, s.highlightUpdate),
		command.Define("highlight.delete", "Delete a highlight. It can be restored from its history.", true, s.highlightDelete),
	)
}

type service struct {
	reg   *command.Registry
	today func() string // the local date, YYYY-MM-DD; tests replace it

	mu        sync.Mutex
	length    map[string]int      // characters of text per file version, for percentage limits
	bibleMaps map[string]BibleMap // reference map per file version
}

func newService(r *command.Registry) *service {
	return &service{reg: r, length: map[string]int{}, bibleMaps: map[string]BibleMap{},
		today: func() string { return time.Now().Format("2006-01-02") }}
}

func now() int64 { return time.Now().UnixMilli() }

// Formats the reader can show.
var reflowable = map[string]bool{"epub": true, "docx": true, "html": true, "md": true, "txt": true}

func readable(format string) bool { return reflowable[format] || format == "pdf" }

// book is an item and the one file of it being read.
type book struct {
	item library.Item
	file library.File
}

// open finds an item's file: the one asked for, or the first readable one, preferring files whose
// folder is online.
func (s *service) open(ctx context.Context, env *command.Env, itemID, fileID string) (book, error) {
	if env.Store == nil {
		return book{}, command.Errorf("unavailable", "no library is open")
	}
	res, err := s.reg.Run(ctx, env, "library.item", []byte(fmt.Sprintf(`{"id":%q}`, itemID)))
	if err != nil {
		return book{}, err
	}
	it, ok := res.(library.Item)
	if !ok {
		return book{}, fmt.Errorf("library.item returned %T", res)
	}
	var pick *library.File
	for i := range it.Files {
		f := &it.Files[i]
		switch {
		case fileID != "":
			if f.ID == fileID {
				pick = f
			}
		case readable(f.Format) && (pick == nil || f.Online && !pick.Online):
			pick = f
		}
	}
	if pick == nil {
		if fileID != "" {
			return book{}, command.Errorf("not_found", "item %s has no file %s", itemID, fileID)
		}
		return book{}, command.Errorf("unavailable", "%q has no file the reader can open", it.Title)
	}
	if !readable(pick.Format) {
		return book{}, command.Errorf("unavailable", "the reader cannot open %s files", pick.Format)
	}
	if !pick.Online || pick.Path == "" {
		return book{}, command.Errorf("unavailable", "%q is in a library folder that is offline; connect its drive", it.Title)
	}
	return book{item: it, file: *pick}, nil
}
