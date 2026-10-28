package search

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackText(t *testing.T) {
	long := strings.Repeat("The grace of the Lord Jesus Christ be with you all. ", 20)
	for _, s := range []string{"", "a", "Ἐν ἀρχῇ ἦν ὁ λόγος", long} {
		b := packText(s)
		got, err := unpackText(b)
		if err != nil || got != s {
			t.Fatalf("%.20q: got %.20q, %v", s, got, err)
		}
	}
	if b := packText(long); b[0] != flateV1 || len(b) > len(long)/4 {
		t.Errorf("long text packed to %d of %d bytes", len(b), len(long))
	}
	if b := packText("a"); b[0] != rawText {
		t.Errorf("short text should be stored as is: %v", b)
	}
	if _, err := unpackText([]byte{9, 1}); err == nil {
		t.Error("unknown blob kind accepted")
	}
	loc, text, err := unpackPassage(packPassage(`{"page":2}`, "line one\nline two"))
	if loc != `{"page":2}` || text != "line one\nline two" || err != nil {
		t.Errorf("passage: %q %q %v", loc, text, err)
	}
}

// An index file of the old format is replaced by an empty one of the current format, and the next
// run indexes the folder again.
func TestOldIndexRebuilt(t *testing.T) {
	e := setup(t)
	dir := t.TempDir()
	books(t, dir)
	id := e.folder(dir)
	path := filepath.Join(e.data, "index", id+".db")
	os.MkdirAll(filepath.Dir(path), 0o755)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`CREATE TABLE passages (id INTEGER PRIMARY KEY, text TEXT)`,
		`INSERT INTO passages (text) VALUES ('old')`, `PRAGMA user_version = 1`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	if st := e.status(id); st.Passages != 0 || st.Indexed != 0 {
		t.Errorf("old index not replaced: %+v", st)
	}
	if p := e.index(id); p.Indexed != 6 {
		t.Errorf("rebuild: %+v", p)
	}
	ix, _ := e.svc.index(id, false)
	var v int
	ix.rd.QueryRow(`PRAGMA user_version`).Scan(&v)
	if v != indexVersion {
		t.Errorf("format %d", v)
	}
	if r := e.query(map[string]any{"query": "righteous faith"}); len(r.Hits) != 1 ||
		!strings.Contains(r.Hits[0].Snippet, "<mark>righteous</mark>") {
		t.Errorf("hits %+v", r.Hits)
	}
}

// A word in more passages than a slice holds is ranked among a sample from every slice.
func TestBroadSearch(t *testing.T) {
	e := setup(t)
	dir := t.TempDir()
	for i := range 40 {
		body := fmt.Sprintf("Title: Book %02d\n\n", i)
		for j := range 6 {
			body += strings.Repeat("filler words here. ", 3+j) + fmt.Sprintf("common word %d.\n\n", j)
			body += strings.Repeat("padding text again. ", 20) + "\n\n"
		}
		if i == 37 {
			body += "common common common common common.\n"
		}
		writeFile(t, filepath.Join(dir, fmt.Sprintf("b%02d.txt", i)), []byte(body))
	}
	id := e.folder(dir)
	e.index(id)
	exact := e.query(map[string]any{"query": "common", "limit": 5})
	old := sliceRows
	sliceRows = 3
	t.Cleanup(func() { sliceRows = old })
	broad := e.query(map[string]any{"query": "common", "limit": 5})
	if len(exact.Hits) != 5 || len(broad.Hits) != 5 || !broad.More {
		t.Fatalf("exact %d, broad %d hits", len(exact.Hits), len(broad.Hits))
	}
	if exact.Hits[0].Title != "Book 37" {
		t.Errorf("best hit %q", exact.Hits[0].Title)
	}
	first, last := "Book 99", ""
	for _, h := range e.query(map[string]any{"query": "common", "limit": 24}).Hits {
		first, last = min(first, h.Title), max(last, h.Title)
	}
	if first > "Book 08" || last < "Book 32" {
		t.Errorf("broad search should sample the whole index: %s to %s", first, last)
	}
	one := e.query(map[string]any{"query": "common", "item_ids": []string{exact.Hits[0].ItemID}, "limit": 50})
	if len(one.Hits) != 7 {
		t.Errorf("item filter: %d hits", len(one.Hits))
	}
	itemRanges = 0
	t.Cleanup(func() { itemRanges = 256 })
	if one := e.query(map[string]any{"query": "common", "item_ids": []string{exact.Hits[0].ItemID}, "limit": 50}); len(one.Hits) != 7 {
		t.Errorf("item filter over slices: %d hits", len(one.Hits))
	}
}
