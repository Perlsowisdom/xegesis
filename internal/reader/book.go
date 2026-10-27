package reader

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ledongthuc/pdf"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/search"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

type BookParams struct {
	ItemID string `json:"item_id" required:"true" desc:"The library item"`
	FileID string `json:"file_id,omitempty" desc:"Which of the item's files; the first readable one when omitted"`
}

// Position is where the user was reading.
type Position struct {
	FileID    string `json:"file_id,omitempty"`
	Section   int    `json:"section"`
	Para      int    `json:"para,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

type Book struct {
	ItemID      string     `json:"item_id"`
	FileID      string     `json:"file_id"`
	Title       string     `json:"title"`
	Authors     []string   `json:"authors"`
	Format      string     `json:"format" enum:"epub,pdf,docx,html,md,txt"`
	Reflowable  bool       `json:"reflowable" desc:"False for PDF, whose pages have a fixed layout"`
	Sections    int        `json:"sections" desc:"EPUB chapters in reading order, PDF pages, or 1 for other formats"`
	TOC         []TOCEntry `json:"toc" desc:"Table of contents; for PDF the interface reads the outline itself"`
	Licence     string     `json:"licence" desc:"The licence type that governs copying"`
	Attribution string     `json:"attribution,omitempty" desc:"The credit added to copied text"`
	Position    *Position  `json:"position,omitempty" desc:"The saved reading position"`
}

func (s *service) openCmd(ctx context.Context, env *command.Env, p BookParams) (Book, error) {
	b, err := s.open(ctx, env, p.ItemID, p.FileID)
	if err != nil {
		return Book{}, err
	}
	lic := effectiveLicence(b.item)
	out := Book{ItemID: b.item.ID, FileID: b.file.ID, Title: b.item.Title, Authors: []string{}, Format: b.file.Format,
		Reflowable: b.file.Format != "pdf", TOC: []TOCEntry{}, Licence: string(lic.Type), Attribution: lic.AttributionLine()}
	for _, pe := range b.item.People {
		if pe.Role == "author" {
			out.Authors = append(out.Authors, pe.Name)
		}
	}
	switch b.file.Format {
	case "epub":
		e, err := openEPUB(b.file.Path)
		if err != nil {
			return Book{}, unreadable(err)
		}
		defer e.Close()
		out.Sections = len(e.spine)
		if out.TOC = e.toc(); len(out.TOC) == 0 {
			out.TOC = e.chapterTOC()
		}
	case "pdf":
		if out.Sections, err = pdfPages(b.file.Path); err != nil {
			return Book{}, unreadable(err)
		}
	default:
		out.Sections = 1
		secs, err := search.ReadSections(b.file.Path, b.file.Format)
		if err != nil {
			return Book{}, unreadable(err)
		}
		for _, sec := range secs {
			for i, bl := range sec.Blocks {
				if bl.Heading {
					out.TOC = append(out.TOC, TOCEntry{Title: bl.Text, Section: 1, Anchor: fmt.Sprintf("bp-%d", i+1), Level: 1})
				}
			}
		}
	}
	if out.TOC == nil {
		out.TOC = []TOCEntry{}
	}
	out.Position, err = loadPosition(ctx, env.Store, b.item.ID)
	return out, err
}

// chapterTOC lists every chapter under its first heading, for books without a navigation
// document.
func (e *epub) chapterTOC() []TOCEntry {
	out := []TOCEntry{}
	for i, it := range e.spine {
		if !it.html {
			continue
		}
		title := fmt.Sprintf("Section %d", i+1)
		if b, err := e.read(it.path); err == nil {
			if h := sanitize(b, epubRes{e: e, base: it.path}).heading; h != "" {
				title = h
			}
		}
		out = append(out, TOCEntry{Title: title, Section: i + 1, Level: 1})
	}
	return out
}

func unreadable(err error) error {
	return command.Errorf("failed", "the book cannot be read: %v", err)
}

type SectionParams struct {
	BookParams
	Section int `json:"section" required:"true" desc:"1-based section: EPUB chapter in reading order, PDF page, or 1"`
}

type Section struct {
	Section  int     `json:"section"`
	Sections int     `json:"sections"`
	Title    string  `json:"title,omitempty" desc:"The section's first heading"`
	HTML     string  `json:"html,omitempty" desc:"Sanitised HTML; paragraphs start with <span class=\"bp\" data-p=\"N\">"`
	Paras    int     `json:"paras,omitempty" desc:"Paragraphs in the section"`
	CFI      string  `json:"cfi,omitempty" desc:"EPUB: the chapter's canonical fragment identifier"`
	Href     string  `json:"href,omitempty" desc:"EPUB: the chapter's path inside the book"`
	Width    float64 `json:"width,omitempty" desc:"PDF: page width in points"`
	Height   float64 `json:"height,omitempty" desc:"PDF: page height in points"`
}

func (s *service) sectionCmd(ctx context.Context, env *command.Env, p SectionParams) (Section, error) {
	b, err := s.open(ctx, env, p.ItemID, p.FileID)
	if err != nil {
		return Section{}, err
	}
	switch b.file.Format {
	case "epub":
		return epubSection(b.file.Path, p.Section)
	case "pdf":
		return pdfSection(b.file.Path, p.Section)
	}
	if p.Section != 1 {
		return Section{}, command.Errorf("not_found", "%s books have one section", b.file.Format)
	}
	var r rendered
	if b.file.Format == "html" {
		doc, err := os.ReadFile(b.file.Path)
		if err != nil {
			return Section{}, unreadable(err)
		}
		r = sanitize(doc, docRes{})
	} else {
		secs, err := search.ReadSections(b.file.Path, b.file.Format)
		if err != nil {
			return Section{}, unreadable(err)
		}
		var blocks []search.Block
		for _, sec := range secs {
			blocks = append(blocks, sec.Blocks...)
		}
		r = blocksHTML(blocks)
	}
	return Section{Section: 1, Sections: 1, Title: r.heading, HTML: r.html, Paras: r.paras}, nil
}

func epubSection(file string, n int) (Section, error) {
	e, err := openEPUB(file)
	if err != nil {
		return Section{}, unreadable(err)
	}
	defer e.Close()
	if n < 1 || n > len(e.spine) {
		return Section{}, command.Errorf("not_found", "the book has %d sections", len(e.spine))
	}
	it := e.spine[n-1]
	out := Section{Section: n, Sections: len(e.spine), CFI: e.cfi(n), Href: it.href}
	if !it.html {
		return out, nil
	}
	doc, err := e.read(it.path)
	if err != nil {
		return Section{}, unreadable(err)
	}
	r := sanitize(doc, epubRes{e: e, base: it.path})
	out.Title, out.HTML, out.Paras = r.heading, r.html, r.paras
	return out, nil
}

// pdfPages counts a PDF's pages, recovering from files the library cannot parse.
func pdfPages(file string) (n int, err error) {
	defer func() {
		if x := recover(); x != nil {
			n, err = 0, fmt.Errorf("pdf: %v", x)
		}
	}()
	f, r, err := pdf.Open(file)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return r.NumPage(), nil
}

func pdfSection(file string, n int) (out Section, err error) {
	defer func() {
		if x := recover(); x != nil {
			out, err = Section{}, unreadable(fmt.Errorf("pdf: %v", x))
		}
	}()
	f, r, err := pdf.Open(file)
	if err != nil {
		return Section{}, unreadable(err)
	}
	defer f.Close()
	total := r.NumPage()
	if n < 1 || n > total {
		return Section{}, command.Errorf("not_found", "the book has %d pages", total)
	}
	out = Section{Section: n, Sections: total}
	// MediaBox may be inherited from the page tree.
	for v := r.Page(n).V; !v.IsNull(); v = v.Key("Parent") {
		if box := v.Key("MediaBox"); box.Len() == 4 {
			out.Width = box.Index(2).Float64() - box.Index(0).Float64()
			out.Height = box.Index(3).Float64() - box.Index(1).Float64()
			break
		}
	}
	return out, nil
}

type FileParams struct {
	BookParams
	Offset int64 `json:"offset,omitempty" desc:"First byte to read"`
	Length int   `json:"length,omitempty" desc:"Bytes to read (default and at most 4 MiB)"`
}

type FileChunk struct {
	Size   int64  `json:"size" desc:"The whole file's size in bytes"`
	Offset int64  `json:"offset"`
	Data   string `json:"data" desc:"The bytes, base64-encoded"`
	EOF    bool   `json:"eof" desc:"This chunk ends the file"`
}

const maxChunk = 4 << 20

func (s *service) fileCmd(ctx context.Context, env *command.Env, p FileParams) (FileChunk, error) {
	b, err := s.open(ctx, env, p.ItemID, p.FileID)
	if err != nil {
		return FileChunk{}, err
	}
	if b.file.Format != "pdf" {
		return FileChunk{}, command.Errorf("bad_params", "book.file serves PDF files; use book.section for %s", b.file.Format)
	}
	if p.Length <= 0 || p.Length > maxChunk {
		p.Length = maxChunk
	}
	f, err := os.Open(b.file.Path)
	if err != nil {
		return FileChunk{}, unreadable(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return FileChunk{}, err
	}
	if p.Offset < 0 || p.Offset > st.Size() {
		return FileChunk{}, command.Errorf("bad_params", "offset %d is outside the file (%d bytes)", p.Offset, st.Size())
	}
	buf := make([]byte, min(int64(p.Length), st.Size()-p.Offset))
	if _, err := f.ReadAt(buf, p.Offset); err != nil && !errors.Is(err, io.EOF) {
		return FileChunk{}, err
	}
	return FileChunk{Size: st.Size(), Offset: p.Offset, Data: base64.StdEncoding.EncodeToString(buf),
		EOF: p.Offset+int64(len(buf)) >= st.Size()}, nil
}

type LocateParams struct {
	BookParams
	Locator search.Locator `json:"locator" desc:"A hit's locator from search.query or search.refs"`
}

type Location struct {
	FileID  string `json:"file_id"`
	Section int    `json:"section" desc:"Section or PDF page to open"`
	Para    int    `json:"para,omitempty" desc:"Paragraph within the section"`
	Anchor  string `json:"anchor,omitempty" desc:"Element ID of the paragraph's marker in the section's HTML"`
}

func (s *service) locateCmd(ctx context.Context, env *command.Env, p LocateParams) (Location, error) {
	b, err := s.open(ctx, env, p.ItemID, p.FileID)
	if err != nil {
		return Location{}, err
	}
	l := p.Locator
	out := Location{FileID: b.file.ID, Section: 1, Para: l.Para}
	switch b.file.Format {
	case "epub":
		e, err := openEPUB(b.file.Path)
		if err != nil {
			return Location{}, unreadable(err)
		}
		defer e.Close()
		out.Section = l.Spine
		if l.Href != "" && (out.Section < 1 || out.Section > len(e.spine) || e.spine[out.Section-1].href != l.Href) {
			out.Section = 0
			for i, it := range e.spine {
				if it.href == l.Href {
					out.Section = i + 1
				}
			}
		}
		if out.Section < 1 || out.Section > len(e.spine) {
			return Location{}, command.Errorf("not_found", "the locator names no chapter of this book")
		}
	case "pdf":
		n, err := pdfPages(b.file.Path)
		if err != nil {
			return Location{}, unreadable(err)
		}
		if l.Page < 1 || l.Page > n {
			return Location{}, command.Errorf("not_found", "the locator names no page of this book")
		}
		out.Section = l.Page
	}
	if out.Para > 0 && b.file.Format != "pdf" {
		out.Anchor = fmt.Sprintf("bp-%d", out.Para)
	}
	return out, nil
}

type SetPositionParams struct {
	BookParams
	Section int `json:"section" required:"true" desc:"Section or PDF page"`
	Para    int `json:"para,omitempty" desc:"Paragraph at the top of the view"`
}

func positionKey(itemID string) string { return "reader.position." + itemID }

func (s *service) setPositionCmd(ctx context.Context, env *command.Env, p SetPositionParams) (Position, error) {
	if p.Section < 1 || p.Para < 0 {
		return Position{}, command.Errorf("bad_params", "section must be 1 or more")
	}
	if err := itemExists(ctx, env.Store, p.ItemID); err != nil {
		return Position{}, err
	}
	pos := Position{FileID: p.FileID, Section: p.Section, Para: p.Para, UpdatedAt: now()}
	raw, _ := json.Marshal(pos)
	err := env.Store.Write(ctx, "book.set_position", func(tx *store.Tx) error {
		key := store.Row{"key": positionKey(p.ItemID)}
		if _, err := tx.Get("settings", key); errors.Is(err, store.ErrNotFound) {
			return tx.Create("settings", store.Row{"key": positionKey(p.ItemID), "value_json": string(raw)})
		} else if err != nil {
			return err
		}
		return tx.Update("settings", key, store.Row{"value_json": string(raw)})
	})
	return pos, err
}

func loadPosition(ctx context.Context, st *store.Store, itemID string) (*Position, error) {
	var raw string
	err := st.QueryRow(ctx, `SELECT value_json FROM settings WHERE key = ?`, positionKey(itemID)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p Position
	if json.Unmarshal([]byte(raw), &p) != nil {
		return nil, nil
	}
	return &p, nil
}

func itemExists(ctx context.Context, st *store.Store, id string) error {
	var n int
	if err := st.QueryRow(ctx, `SELECT COUNT(*) FROM items WHERE id = ? AND deleted_at IS NULL`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return command.Errorf("not_found", "no item %s", id)
	}
	return nil
}
