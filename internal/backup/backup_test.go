package backup

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

func openStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func run[R any](t *testing.T, r *command.Registry, s *store.Store, id string, params any) R {
	t.Helper()
	raw, _ := json.Marshal(params)
	res, err := r.Run(context.Background(), &command.Env{Store: s}, id, raw)
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	return res.(R)
}

func registry(dir string) *command.Registry {
	r := command.NewRegistry()
	Register(r, dir)
	return r
}

var bodies = []string{
	"",
	"alpha",
	"alpha\n",
	"\n\nalpha",
	"alpha\n\n\nbeta\n\n",
	"# Title\r\n\r\ntext with \"quotes\" and éè שלום\n\n- [ ] one\n- [x] two\n",
	"\n",
}

// seed fills a library with every kind of data an export carries.
func seed(t *testing.T, s *store.Store) {
	t.Helper()
	ctx := context.Background()
	err := s.Write(ctx, "test.seed", func(tx *store.Tx) error {
		creates := []struct {
			table string
			row   store.Row
		}{
			{"items", store.Row{"id": "ITEM1", "kind": "commentary", "title": "Romans", "year": int64(1999), "source": "own", "created_at": int64(1700000000123), "updated_at": int64(1700000000456)}},
			{"items", store.Row{"id": "ITEM2", "kind": "monograph", "title": "Grace", "source": "import", "created_at": int64(5), "updated_at": int64(6)}},
			{"anchors", store.Row{"id": "A1", "verse_start": int64(45008028), "verse_end": int64(45008030), "versification": "english"}},
			{"anchors", store.Row{"id": "A2", "item_id": "ITEM1", "locator_json": `{"cfi":"epubcfi(/6/4)"}`}},
			{"anchors", store.Row{"id": "A3", "verse_start": int64(43003016), "verse_end": int64(43003016), "locator_json": `{ "spaced": 1 }`}},
			{"notes", store.Row{"id": "N1", "anchor_id": "A1", "title": "Called", "body_md": bodies[4], "created_at": int64(1), "updated_at": int64(2)}},
			{"notes", store.Row{"id": "N2", "body_md": bodies[5], "created_at": int64(3), "updated_at": int64(4)}},
			{"notes", store.Row{"id": "N3", "anchor_id": "A2", "body_md": "", "created_at": int64(3), "updated_at": int64(4)}},
			{"notes", store.Row{"id": "N4", "body_md": bodies[3], "created_at": int64(3), "updated_at": int64(4), "deleted_at": int64(9)}},
			{"highlights", store.Row{"id": "H1", "anchor_id": "A1", "style": "green", "created_at": int64(10)}},
			{"highlights", store.Row{"id": "H2", "anchor_id": "A3", "created_at": int64(11), "deleted_at": int64(12)}},
			{"lists", store.Row{"id": "L1", "kind": "verses", "name": "Favourites", "created_at": int64(1), "updated_at": int64(2)}},
			{"lists", store.Row{"id": "L2", "kind": "words", "name": "Greek", "created_at": int64(1), "updated_at": int64(2)}},
			{"list_entries", store.Row{"id": "E1", "list_id": "L1", "anchor_id": "A3", "note": "key verse", "position": int64(1)}},
			{"list_entries", store.Row{"id": "E2", "list_id": "L2", "word": "agape", "language": "grc", "position": int64(0)}},
			{"collections", store.Row{"id": "C1", "name": "Parent", "kind": "manual", "created_at": int64(1), "updated_at": int64(2)}},
			{"collections", store.Row{"id": "C2", "parent_id": "C1", "name": "Child", "kind": "rule", "rule_json": `{"facet":"era","value":"early"}`, "position": int64(1), "created_at": int64(1), "updated_at": int64(2)}},
			{"collection_items", store.Row{"collection_id": "C1", "item_id": "ITEM1", "position": int64(2)}},
			{"collection_items", store.Row{"collection_id": "C1", "item_id": "ITEM2", "position": int64(1)}},
			{"reading_plans", store.Row{"id": "P1", "name": "Gospels", "start_date": "2026-01-01", "created_at": int64(1)}},
			{"plan_days", store.Row{"plan_id": "P1", "day_no": int64(1), "verse_start": int64(43001001), "verse_end": int64(43001018)}},
			{"plan_days", store.Row{"plan_id": "P1", "day_no": int64(2), "verse_start": int64(1), "verse_end": int64(2), "item_id": "ITEM2", "locator_json": `{"page":3}`}},
			{"plan_progress", store.Row{"plan_id": "P1", "day_no": int64(1), "completed_at": int64(1700000999999)}},
			{"documents", store.Row{"id": "D1", "kind": "sermon", "title": "Grace alone", "body_md": bodies[4], "outline_json": `[{"h":"One"},{"h":"Two"}]`, "created_at": int64(1), "updated_at": int64(2)}},
			{"documents", store.Row{"id": "D2", "title": "Empty", "body_md": "", "created_at": int64(1), "updated_at": int64(2)}},
		}
		for _, c := range creates {
			if err := tx.Create(c.table, c.row); err != nil {
				return fmt.Errorf("%s: %w", c.table, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

var dumpTables = []string{"items", "anchors", "notes", "highlights", "lists", "list_entries", "collections",
	"collection_items", "reading_plans", "plan_days", "plan_progress", "documents"}

// dump renders the exported tables, row by row, so two libraries can be compared exactly.
func dump(t *testing.T, s *store.Store) string {
	t.Helper()
	var sb strings.Builder
	for _, tb := range dumpTables {
		rows, err := s.Query(context.Background(), "SELECT * FROM "+tb+" ORDER BY 1, 2")
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&sb, "%s", tb)
			for i, c := range cols {
				fmt.Fprintf(&sb, " %s=%#v", c, vals[i])
			}
			sb.WriteByte('\n')
		}
		rows.Close()
	}
	return sb.String()
}

func TestRoundTripFolderAndZip(t *testing.T) {
	for _, zipped := range []bool{false, true} {
		t.Run(fmt.Sprintf("zip=%v", zipped), func(t *testing.T) {
			src, srcDir := openStore(t)
			seed(t, src)
			dest := filepath.Join(t.TempDir(), "out")
			if zipped {
				dest += ".zip"
			}
			res := run[ExportResult](t, registry(srcDir), src, "export.data", ExportParams{Path: dest, History: true})
			if res.Counts["note"] != 4 || res.Counts["document"] != 2 || res.Counts["item"] != 2 {
				t.Fatalf("counts %v", res.Counts)
			}
			dst, dstDir := openStore(t)
			got := run[ImportResult](t, registry(dstDir), dst, "import.data", ImportParams{Path: res.Path})
			if len(got.Updated) != 0 || got.Unchanged["note"] != 0 || got.Created["anchor"] != 3 {
				t.Fatalf("empty library should only create: %+v", got)
			}
			if want, have := dump(t, src), dump(t, dst); want != have {
				t.Fatalf("library differs after round trip\nwant:\n%s\nhave:\n%s", want, have)
			}
			// Importing again changes nothing and logs nothing new.
			before := changeCount(t, dst)
			again := run[ImportResult](t, registry(dstDir), dst, "import.data", ImportParams{Path: res.Path})
			if len(again.Created) != 0 || len(again.Updated) != 0 {
				t.Fatalf("second import changed things: %+v", again)
			}
			if changeCount(t, dst) != before {
				t.Fatal("second import logged changes")
			}
		})
	}
}

func changeCount(t *testing.T, s *store.Store) int {
	t.Helper()
	var n int
	if err := s.QueryRow(context.Background(), "SELECT count(*) FROM changes").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestImportMergesAndLogs(t *testing.T) {
	src, srcDir := openStore(t)
	seed(t, src)
	res := run[ExportResult](t, registry(srcDir), src, "export.data", ExportParams{Path: filepath.Join(t.TempDir(), "x")})

	dst, dstDir := openStore(t)
	run[ImportResult](t, registry(dstDir), dst, "import.data", ImportParams{Path: res.Path})
	// Edit the export's note body and add a new document; matching IDs update, new ones are created.
	noteMD := filepath.Join(res.Path, "objects", "note", "N2.md")
	if err := os.WriteFile(noteMD, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := `{"id":"D3","type":"document","kind":"study","title":"New","outline":null,
	"created_at":{"$date":"2026-01-01T00:00:00.000Z"},"updated_at":{"$date":"2026-01-01T00:00:00.000Z"},"deleted_at":null}`
	if err := os.WriteFile(filepath.Join(res.Path, "objects", "document", "D3.json"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	got := run[ImportResult](t, registry(dstDir), dst, "import.data", ImportParams{Path: res.Path})
	if got.Updated["note"] != 1 || got.Created["document"] != 1 {
		t.Fatalf("result %+v", got)
	}
	var body string
	dst.QueryRow(context.Background(), "SELECT body_md FROM notes WHERE id = 'N2'").Scan(&body)
	if body != "changed" {
		t.Fatalf("body %q", body)
	}
	hist, err := dst.History(context.Background(), "notes", "N2")
	if err != nil || hist[len(hist)-1].Command != "import.data" || hist[len(hist)-1].Field != "body_md" {
		t.Fatalf("history %+v %v", hist, err)
	}
	var created int64
	dst.QueryRow(context.Background(), "SELECT created_at FROM documents WHERE id = 'D3'").Scan(&created)
	if created != time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("created_at %d", created)
	}
	var n int
	dst.QueryRow(context.Background(), "SELECT count(*) FROM changes WHERE command NOT IN ('import.data')").Scan(&n)
	if n != 0 {
		t.Fatalf("%d changes were not logged under import.data", n)
	}
}

func TestExportTypesFilterAndLayout(t *testing.T) {
	src, srcDir := openStore(t)
	seed(t, src)
	res := run[ExportResult](t, registry(srcDir), src, "export.data", ExportParams{Path: t.TempDir(), Types: []string{"document"}})
	if res.Counts["note"] != 0 || res.Counts["document"] != 2 || res.Counts["item"] != 0 {
		t.Fatalf("counts %v", res.Counts)
	}
	for _, f := range []string{"manifest.json", "types.json", "objects/document/D1.json", "objects/document/D1.md"} {
		if _, err := os.Stat(filepath.Join(res.Path, filepath.FromSlash(f))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(res.Path, "history.jsonl")); err == nil {
		t.Fatal("history.jsonl written without history: true")
	}
	raw, _ := os.ReadFile(filepath.Join(res.Path, "objects", "document", "D1.json"))
	var props map[string]any
	json.Unmarshal(raw, &props)
	if c := props["created_at"].(map[string]any)["$date"]; c != "1970-01-01T00:00:00.001Z" {
		t.Fatalf("created_at %v", c)
	}
	if _, ok := props["body"]; ok {
		t.Fatal("body belongs in the .md file")
	}
	if _, err := r(src, srcDir, "export.data", ExportParams{Types: []string{"nope"}}); err == nil {
		t.Fatal("unknown type accepted")
	}
}

func r(s *store.Store, dir, id string, params any) (any, error) {
	raw, _ := json.Marshal(params)
	return registry(dir).Run(context.Background(), &command.Env{Store: s}, id, raw)
}

func TestExportFormatsAnchorsAndRelations(t *testing.T) {
	src, srcDir := openStore(t)
	seed(t, src)
	res := run[ExportResult](t, registry(srcDir), src, "export.data", ExportParams{Path: t.TempDir(), History: true})
	read := func(name string) map[string]any {
		raw, err := os.ReadFile(filepath.Join(res.Path, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		json.Unmarshal(raw, &m)
		return m
	}
	n := read("objects/note/N1.json")
	if n["ref"] != "Romans 8:28-30" || n["verse_start"].(float64) != 45008028 {
		t.Fatalf("note props %v", n)
	}
	c := read("objects/collection/C2.json")
	if fmt.Sprint(c["parent"]) != "[any://C1]" || fmt.Sprint(c["kind"]) != "[rule]" {
		t.Fatalf("collection props %v", c)
	}
	if _, ok := c["rule"].(map[string]any); !ok {
		t.Fatalf("rule should be an object: %v", c["rule"])
	}
}

func TestMarkdownRulesFromAnyDocs(t *testing.T) {
	cases := []struct {
		md     string
		blocks []string
	}{
		{"alpha\n\nbeta", []string{"alpha", "beta"}},
		{"alpha\n\n\nbeta", []string{"alpha", "", "beta"}},
		{"\n\nalpha", []string{"", "", "alpha"}},
		{"alpha", []string{"alpha"}},
		{"alpha\n", []string{"alpha", ""}}, // after the terminator "alpha\n" is stripped from "alpha\n\n"
		{"alpha\n\n\n\nbeta", []string{"alpha", "", "", "beta"}},
		{"line one\nline two\n\nbeta", []string{"line one\nline two", "beta"}},
		{"", nil},
	}
	for _, c := range cases {
		got := ParseBlocks(c.md)
		if fmt.Sprintf("%q", got) != fmt.Sprintf("%q", c.blocks) {
			t.Errorf("ParseBlocks(%q) = %q, want %q", c.md, got, c.blocks)
		}
		if back := RenderBlocks(got); back != c.md {
			t.Errorf("RenderBlocks(ParseBlocks(%q)) = %q", c.md, back)
		}
	}
	// A file "alpha\n" is the same document as "alpha"; "alpha\n\n" has a trailing empty paragraph.
	if fmt.Sprintf("%q", ParseBlocks(DecodeBody("alpha\n"))) != `["alpha"]` ||
		fmt.Sprintf("%q", ParseBlocks(DecodeBody("alpha\n\n"))) != `["alpha" ""]` {
		t.Fatal("terminator rule")
	}
	for _, b := range append(bodies, "a\n\n\n\n\nb", "\n\n\n") {
		if got := DecodeBody(EncodeBody(b)); got != b {
			t.Errorf("body %q came back as %q", b, got)
		}
		if got := RenderBlocks(ParseBlocks(b)); got != b && b != "" {
			t.Errorf("blocks of %q render as %q", b, got)
		}
	}
}

func TestBackupOfOpenDatabase(t *testing.T) {
	s, dir := openStore(t)
	seed(t, s)
	reg := registry(dir)
	// Keep writing while backups are made.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			s.Write(context.Background(), "test.write", func(tx *store.Tx) error {
				return tx.Create("documents", store.Row{"id": fmt.Sprintf("W%05d", i), "title": "w", "created_at": int64(i), "updated_at": int64(i)})
			})
		}
	}()
	var last BackupResult
	for i := 0; i < 5; i++ {
		last = run[BackupResult](t, reg, s, "backup.create", BackupParams{})
	}
	close(stop)
	wg.Wait()

	if last.Bytes == 0 || last.SchemaVersion != 1 {
		t.Fatalf("%+v", last)
	}
	copyDB, err := store.Open(last.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	var n int
	copyDB.QueryRow(context.Background(), "SELECT count(*) FROM notes").Scan(&n)
	if n != 4 {
		t.Fatalf("backup has %d notes", n)
	}
	list := run[[]BackupInfo](t, reg, s, "backup.list", BackupListParams{})
	if len(list) != 5 {
		t.Fatalf("listed %d backups", len(list))
	}
	if _, err := r(s, dir, "backup.create", BackupParams{Path: last.Path}); err == nil {
		t.Fatal("overwrote an existing backup")
	}
}

func TestScheduledBackupKeepsLastN(t *testing.T) {
	s, dir := openStore(t)
	reg := registry(dir)
	if sc := run[Schedule](t, reg, s, "backup.settings.get", struct{}{}); sc.Every != "off" || sc.Keep != 7 {
		t.Fatalf("default %+v", sc)
	}
	if res := run[AutoResult](t, reg, s, "backup.auto", struct{}{}); res.Ran {
		t.Fatal("ran while off")
	}
	if _, err := r(s, dir, "backup.settings.set", SettingsSetParams{Every: "daily", Keep: 500}); err == nil {
		t.Fatal("accepted keep=500")
	}
	run[Schedule](t, reg, s, "backup.settings.set", SettingsSetParams{Every: "daily", Keep: 2})
	if sc := run[Schedule](t, reg, s, "backup.settings.get", struct{}{}); sc.Every != "daily" || sc.Keep != 2 {
		t.Fatalf("saved %+v", sc)
	}
	hist, _ := s.History(context.Background(), "settings", `["backup.schedule"]`)
	if len(hist) == 0 || hist[0].Command != "backup.settings.set" {
		t.Fatalf("setting change not logged: %+v", hist)
	}

	clock := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	now = func() time.Time { return clock }
	t.Cleanup(func() { now = time.Now })

	for day := 0; day < 4; day++ {
		res := run[AutoResult](t, reg, s, "backup.auto", struct{}{})
		if !res.Ran {
			t.Fatalf("day %d: no backup", day)
		}
		if again := run[AutoResult](t, reg, s, "backup.auto", struct{}{}); again.Ran {
			t.Fatal("backed up twice in a day")
		}
		clock = clock.Add(25 * time.Hour)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "backups", "library-auto-*.db"))
	if len(files) != 2 {
		t.Fatalf("kept %d copies: %v", len(files), files)
	}
	// Manual copies are never pruned.
	run[BackupResult](t, reg, s, "backup.create", BackupParams{})
	run[AutoResult](t, reg, s, "backup.auto", struct{}{})
	if m, _ := filepath.Glob(filepath.Join(dir, "backups", "library-2*.db")); len(m) != 1 {
		t.Fatalf("manual copy count %d", len(m))
	}
}

func TestImportRejectsBadInput(t *testing.T) {
	s, dir := openStore(t)
	empty := t.TempDir()
	if _, err := r(s, dir, "import.data", ImportParams{Path: empty}); err == nil {
		t.Fatal("imported a folder without a manifest")
	}
	bad := t.TempDir()
	os.WriteFile(filepath.Join(bad, "manifest.json"), []byte(`{"format":"xegesis-export","format_version":99}`), 0o644)
	if _, err := r(s, dir, "import.data", ImportParams{Path: bad}); err == nil {
		t.Fatal("imported a newer format")
	}
	// A broken object rolls the whole import back.
	src, srcDir := openStore(t)
	seed(t, src)
	res := run[ExportResult](t, registry(srcDir), src, "export.data", ExportParams{Path: t.TempDir()})
	os.WriteFile(filepath.Join(res.Path, "objects", "document", "ZZ.json"), []byte(`{"id":"ZZ","created_at":"nope"}`), 0o644)
	if _, err := r(s, dir, "import.data", ImportParams{Path: res.Path}); err == nil {
		t.Fatal("imported a broken object")
	}
	var n int
	s.QueryRow(context.Background(), "SELECT count(*) FROM notes").Scan(&n)
	if n != 0 {
		t.Fatal("partial import was kept")
	}
	// A zip with the export in a folder (as written) and one with it at the root both work.
	zp := filepath.Join(t.TempDir(), "x.zip")
	f, _ := os.Create(zp)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("manifest.json")
	w.Write([]byte(`{"format":"xegesis-export","format_version":1}`))
	zw.Close()
	f.Close()
	if _, err := r(s, dir, "import.data", ImportParams{Path: zp}); err != nil {
		t.Fatal(err)
	}
}
