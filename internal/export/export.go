// Package export prints and exports Bible passages, book excerpts, notes, lists, reading plans and
// reports, each within its own licence: public domain and your own work without limits, free-to-
// share items with their credit line, and personal copies up to their print or export limit per day,
// measured as book.copy measures them and counted in copy_log. What a limit cuts is always said.
package export

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// Register adds export.preview, export.make and export.print. Files go to dataDir/exports unless
// a path is given; Bible text comes from bibles.
func Register(r *command.Registry, dataDir string, bibles *bible.Library) {
	s := &service{reg: r, dir: filepath.Join(dataDir, "exports"), bibles: bibles,
		today: func() string { return time.Now().Format("2006-01-02") }}
	r.Add(
		command.Define("export.preview", "Show what a print or export would contain: the page, and what each item's licence cuts and why. Nothing is counted.", false, s.preview),
		command.Define("export.make", "Write a print-ready file (PDF, Word, HTML or Markdown) of passages, book excerpts, notes, lists, plans or reports. Personal copies count against their daily export limit.", true, s.make),
		command.Define("export.print", "Prepare a print: the print-ready page for the system print dialog. Personal copies count against their daily print limit.", true, s.print),
	)
}

type service struct {
	reg    *command.Registry
	dir    string
	bibles *bible.Library
	today  func() string
}

// Source is one thing to print or export.
type Source struct {
	Kind string `json:"kind" required:"true" enum:"passage,book,note,list,plan,report" desc:"What to include"`

	Ref           string   `json:"ref,omitempty" desc:"passage: the reference, e.g. \"John 3:16-18\""`
	Modules       []string `json:"modules,omitempty" desc:"passage: Bible module IDs, in column order"`
	Versification string   `json:"versification,omitempty" desc:"passage: numbering of ref, english (default) or hebrew"`

	ItemID string `json:"item_id,omitempty" desc:"book: the library item"`
	FileID string `json:"file_id,omitempty" desc:"book: which of its files"`
	Text   string `json:"text,omitempty" desc:"book: selected text; or give from and to"`
	From   int    `json:"from,omitempty" desc:"book: first section or PDF page"`
	To     int    `json:"to,omitempty" desc:"book: last section or PDF page"`
	Skip   int    `json:"skip,omitempty" desc:"book: paragraphs at the start to leave out (the part already printed)"`

	ID     string `json:"id,omitempty" desc:"note, list or plan: its ID"`
	Module string `json:"module,omitempty" desc:"list: a Bible to quote a verse list's verses from"`

	Report string          `json:"report,omitempty" desc:"report: passage, word or topic"`
	Params json.RawMessage `json:"params,omitempty" desc:"report: the report command's parameters"`
}

// Options shape the page. Every one has a sensible default.
type Options struct {
	PageSize     string  `json:"page_size,omitempty" enum:"a4,letter" desc:"Paper size (default a4)"`
	FontSize     float64 `json:"font_size,omitempty" desc:"Text size in points (default 11)"`
	VerseNumbers *bool   `json:"verse_numbers,omitempty" desc:"Show verse numbers (default true)"`
	Notes        bool    `json:"notes,omitempty" desc:"Add your notes on each Bible passage"`
	Layout       string  `json:"layout,omitempty" enum:"side_by_side,one_after_another" desc:"Several Bibles: in columns side by side (default) or one after another"`
	Header       *bool   `json:"header,omitempty" desc:"The title at the top of each page (default true)"`
	Footer       *bool   `json:"footer,omitempty" desc:"Page numbers at the foot of each page (default true)"`
	Credits      string  `json:"credits,omitempty" enum:"each,end" desc:"Credit lines under each piece (default) or together at the end"`
}

func (o Options) verseNumbers() bool { return o.VerseNumbers == nil || *o.VerseNumbers }
func (o Options) header() bool       { return o.Header == nil || *o.Header }
func (o Options) footer() bool       { return o.Footer == nil || *o.Footer }
func (o Options) sideBySide() bool   { return o.Layout != "one_after_another" }
func (o Options) fontSize() float64 {
	if o.FontSize < 6 || o.FontSize > 36 {
		return 11
	}
	return o.FontSize
}

type PreviewParams struct {
	Sources []Source `json:"sources" required:"true" desc:"What to include, in order"`
	Title   string   `json:"title,omitempty" desc:"The document's title; made from the first source when omitted"`
	Format  string   `json:"format,omitempty" enum:"print,pdf,docx,html,md" desc:"What the preview is for, which decides the limit that applies (default print)"`
	Options Options  `json:"options,omitempty"`
}

type MakeParams struct {
	Sources []Source `json:"sources" required:"true" desc:"What to include, in order"`
	Title   string   `json:"title,omitempty"`
	Format  string   `json:"format" required:"true" enum:"pdf,docx,html,md" desc:"The file format"`
	Path    string   `json:"path,omitempty" desc:"The file to write; default a new file in the exports folder of the data folder"`
	Replace bool     `json:"replace,omitempty" desc:"Overwrite the file if it exists"`
	Options Options  `json:"options,omitempty"`
}

type PrintParams struct {
	Sources []Source `json:"sources" required:"true"`
	Title   string   `json:"title,omitempty"`
	Options Options  `json:"options,omitempty"`
}

// PieceInfo says what one piece holds and what its licence allowed.
type PieceInfo struct {
	Title   string   `json:"title"`
	Licence string   `json:"licence" desc:"The licence in plain words: Public domain, Free to share, Personal copy or My own work"`
	Credits []string `json:"credits,omitempty"`
	Pages   float64  `json:"pages" desc:"Length in pages of the whole piece"`
	Cut     *Cut     `json:"cut,omitempty" desc:"Present when a licence left part of the piece out"`
	Confirm bool     `json:"confirm,omitempty" desc:"The item's licence was never confirmed; it is treated as a personal copy"`
}

// Cut is what a limit left out.
type Cut struct {
	Reason string  `json:"reason" desc:"In plain words, e.g. \"Personal copy: up to 10% of this book per day; 3 of 12 pages fit.\""`
	Kept   float64 `json:"kept_pages"`
	Total  float64 `json:"total_pages"`
	Rest   *Source `json:"rest,omitempty" desc:"The part left out, ready to print or export another day"`
}

type Preview struct {
	Title    string      `json:"title"`
	Format   string      `json:"format"`
	Pieces   []PieceInfo `json:"pieces"`
	Cuts     []string    `json:"cuts" desc:"Every cut, in plain words; empty when nothing was left out"`
	Warnings []string    `json:"warnings" desc:"Other things to know, such as characters a PDF font cannot show"`
	Body     string      `json:"body" desc:"The page's HTML, for showing inside the interface"`
	CSS      string      `json:"css" desc:"Styles for body, scoped to .xdoc, with the page size for printing"`
	HTML     string      `json:"html,omitempty" desc:"export.print: the whole page as one HTML document"`
}

type MakeResult struct {
	Preview
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

func (s *service) preview(ctx context.Context, env *command.Env, p PreviewParams) (Preview, error) {
	format := p.Format
	if format == "" {
		format = "print"
	}
	b, err := s.build(ctx, env, p.Sources, p.Title, p.Options, action(format))
	if err != nil {
		return Preview{}, err
	}
	out := b.preview(format)
	if format == "pdf" {
		out.Warnings = append(out.Warnings, pdfWarnings(b.doc)...)
	}
	return out, nil
}

func (s *service) print(ctx context.Context, env *command.Env, p PrintParams) (Preview, error) {
	b, err := s.build(ctx, env, p.Sources, p.Title, p.Options, "print")
	if err != nil {
		return Preview{}, err
	}
	if err := b.record(ctx, env, "export.print", s.today()); err != nil {
		return Preview{}, err
	}
	out := b.preview("print")
	out.HTML = HTML(b.doc)
	return out, nil
}

func (s *service) make(ctx context.Context, env *command.Env, p MakeParams) (MakeResult, error) {
	b, err := s.build(ctx, env, p.Sources, p.Title, p.Options, "export")
	if err != nil {
		return MakeResult{}, err
	}
	var data []byte
	warnings := []string{}
	switch p.Format {
	case "pdf":
		data, warnings, err = PDF(b.doc)
	case "docx":
		data, err = DOCX(b.doc)
	case "html":
		data = []byte(HTML(b.doc))
	case "md":
		data = Markdown(b.doc)
	default:
		return MakeResult{}, command.Errorf("bad_params", "format must be pdf, docx, html or md")
	}
	if err != nil {
		return MakeResult{}, err
	}
	path, err := s.target(p.Path, b.doc.Title, p.Format, p.Replace)
	if err != nil {
		return MakeResult{}, err
	}
	if err := writeFile(path, data); err != nil {
		return MakeResult{}, err
	}
	if err := b.record(ctx, env, "export.make", s.today()); err != nil {
		os.Remove(path)
		return MakeResult{}, err
	}
	out := MakeResult{Preview: b.preview(p.Format), Path: path, Bytes: int64(len(data))}
	out.Warnings = append(out.Warnings, warnings...)
	return out, nil
}

func action(format string) string {
	if format == "print" {
		return "print"
	}
	return "export"
}

var unsafeName = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)

// target is the file to write: the path given, or a new name in the exports folder.
func (s *service) target(path, title, format string, replace bool) (string, error) {
	if path != "" {
		if !strings.EqualFold(filepath.Ext(path), "."+format) {
			path += "." + format
		}
		if _, err := os.Stat(path); err == nil && !replace {
			return "", command.Errorf("bad_params", "%s already exists; give replace to overwrite it", path)
		}
		return path, nil
	}
	name := strings.TrimSpace(unsafeName.ReplaceAllString(title, " "))
	if len(name) > 80 {
		name = strings.TrimSpace(name[:80])
	}
	if name == "" {
		name = "Xegesis export"
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", err
	}
	for i := 1; ; i++ {
		p := filepath.Join(s.dir, name+"."+format)
		if i > 1 {
			p = filepath.Join(s.dir, fmt.Sprintf("%s (%d).%s", name, i, format))
		}
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p, nil
		}
	}
}

// writeFile writes through a temporary file, so a failed export leaves no half-written file.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// record counts what personal copies gave, per item, in copy_log, in one logged write.
func (b *built) record(ctx context.Context, env *command.Env, cmd, day string) error {
	if len(b.taken) == 0 {
		return nil
	}
	return env.Store.Write(ctx, cmd, func(tx *store.Tx) error {
		for _, o := range b.owners {
			n := b.taken[o]
			if n == 0 {
				continue
			}
			row := store.Row{"id": store.NewID(), "action": b.action, "day": day, "chars": int64(n), "created_at": time.Now().UnixMilli()}
			if o.module {
				row["module"] = o.key
			} else {
				row["item_id"] = o.key
			}
			if err := tx.Create("copy_log", row); err != nil {
				return err
			}
		}
		return nil
	})
}
