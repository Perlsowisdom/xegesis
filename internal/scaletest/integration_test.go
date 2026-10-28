package scaletest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/backup"
	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/reader"
	"github.com/Perlsowisdom/xegesis/internal/search"
)

const usfmRomans = `\id ROM Test Bible
\h Romans
\c 8
\p
\v 28 And we know that all things work together for good to those who love God.
\v 29 For whom he foreknew, he also predestined.
\v 30 Whom he predestined, those he also called.
`

// TestM1 walks through milestone 1 against a scratch library, only through the command registry:
// add books, install a Bible from a file, read a book beside it, search and open a hit,
// highlight, export and back up, and search again with one library folder offline.
func TestM1(t *testing.T) {
	root, err := os.MkdirTemp("", "xegesis-m1-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) }) // best effort: index files stay open until the process ends
	gen, err := Generate(Config{Dir: filepath.Join(root, "books"), Books: 20, Seed: 5, MaxPages: 30})
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "data")
	os.MkdirAll(data, 0o755)
	l := openLib(t, data)

	var added library.AddResult
	l.must("library.add", map[string]any{"paths": []string{gen.Internal, gen.External}}, &added)
	if added.Added != 20 || len(added.Folders) != 2 || len(added.Errors) != 0 {
		t.Fatalf("library.add: %+v", added)
	}
	licensed := 0
	for _, c := range added.Licences {
		licensed += c.Count
	}
	if licensed != 20 {
		t.Errorf("licences: %+v", added.Licences)
	}

	usfm := filepath.Join(root, "ROM.usfm")
	os.WriteFile(usfm, []byte(usfmRomans), 0o644)
	var inst bible.InstallResult
	l.must("bible.install", map[string]any{"path": usfm, "id": "test", "name": "Test Bible", "abbreviation": "TST",
		"licence_type": "public_domain"}, &inst)

	// Read a book beside the Bible.
	var items library.ItemsResult
	l.must("library.items", map[string]any{"kind": "book"}, &items)
	if items.Total != 20 {
		t.Fatalf("items: %d", items.Total)
	}
	var b reader.Book
	for _, it := range items.Items {
		if l.must("book.open", map[string]any{"item_id": it.ID}, &b); b.Format == "epub" {
			break
		}
	}
	var sec reader.Section
	l.must("book.section", map[string]any{"item_id": b.ItemID, "section": 1}, &sec)
	var read struct {
		Verses []bible.ReadVerse `json:"verses"`
	}
	l.must("bible.read", map[string]any{"module": "test", "ref": "Rom 8:28"}, &read)
	if b.Sections == 0 || sec.Paras == 0 || len(read.Verses) != 1 {
		t.Fatalf("book %+v, %d paragraphs, verses %+v", b, sec.Paras, read.Verses)
	}

	// Search the library and open the hit.
	l.must("search.index", map[string]any{"wait": true}, nil)
	var res search.QueryResult
	l.must("search.query", map[string]any{"query": RareWord}, &res)
	if len(res.Hits) != 1 || !strings.Contains(res.Hits[0].Snippet, "<mark>"+RareWord+"</mark>") {
		t.Fatalf("search: %+v", res)
	}
	hit := res.Hits[0]
	var loc reader.Location
	l.must("book.locate", map[string]any{"item_id": hit.ItemID, "file_id": hit.FileID, "locator": hit.Locator}, &loc)
	l.must("book.section", map[string]any{"item_id": hit.ItemID, "file_id": hit.FileID, "section": loc.Section}, &sec)
	if !strings.Contains(sec.HTML, RareWord) || loc.Para == 0 {
		t.Fatalf("the hit's section does not hold the word: %+v", loc)
	}
	l.must("search.refs", map[string]any{"ref": "Rom 8:28"}, &res)
	if len(res.Hits) == 0 {
		t.Error("no books cite Rom 8:28")
	}

	// Highlight the hit and the verse, and note the verse.
	var hl reader.Highlight
	l.must("highlight.create", map[string]any{"item_id": hit.ItemID, "locator": map[string]any{
		"section": loc.Section, "para": loc.Para, "offset": 0, "end_para": loc.Para, "end_offset": 10,
		"text": "synthetic"}, "style": "green"}, &hl)
	l.must("highlight.create", map[string]any{"ref": "Rom 8:28"}, nil)
	var hls []reader.Highlight
	l.must("highlight.list", map[string]any{"item_id": hit.ItemID}, &hls)
	if len(hls) != 1 || hls[0].ID != hl.ID {
		t.Errorf("highlights: %+v", hls)
	}
	l.must("note.create", map[string]any{"body": "All things work together.", "ref": "Rom 8:28"}, nil)

	var ex backup.ExportResult
	l.must("export.data", map[string]any{"path": filepath.Join(root, "export.zip")}, &ex)
	if ex.Counts["highlight"] != 2 || ex.Counts["note"] != 1 {
		t.Errorf("export: %+v", ex.Counts)
	}
	var bk backup.BackupResult
	l.must("backup.create", map[string]any{"path": filepath.Join(root, "backup.db")}, &bk)
	if fi, err := os.Stat(bk.Path); err != nil || fi.Size() == 0 || bk.Bytes == 0 {
		t.Errorf("backup %+v: %v", bk, err)
	}

	// Unplug the external drive: its books stay searchable, marked offline.
	if err := os.Rename(filepath.Dir(gen.External), filepath.Join(root, "unplugged")); err != nil {
		t.Fatal(err)
	}
	l.must("search.query", map[string]any{"query": "grace", "limit": 100}, &res)
	online, offline := 0, 0
	for _, h := range res.Hits {
		if h.Online {
			online++
		} else {
			offline++
		}
	}
	if online == 0 || offline == 0 {
		t.Errorf("hits %d online, %d offline", online, offline)
	}
}
