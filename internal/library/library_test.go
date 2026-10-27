package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

func setup(t *testing.T) (*command.Registry, *command.Env) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	roots := mountedRoots
	mountedRoots = func() []string { return nil }
	t.Cleanup(func() { mountedRoots = roots })
	r := command.NewRegistry()
	Register(r)
	return r, &command.Env{Store: s}
}

func run(t *testing.T, r *command.Registry, env *command.Env, id string, params any, out any) error {
	t.Helper()
	raw, _ := json.Marshal(params)
	res, err := r.Run(context.Background(), env, id, raw)
	if err != nil {
		return err
	}
	if out != nil {
		b, _ := json.Marshal(res)
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatal(err)
		}
	}
	return nil
}

func must(t *testing.T, r *command.Registry, env *command.Env, id string, params any, out any) {
	t.Helper()
	if err := run(t, r, env, id, params, out); err != nil {
		t.Fatalf("%s: %v", id, err)
	}
}

func code(err error) string {
	var ce *command.Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

type obj = map[string]any

// books makes a folder with one file of each format.
func books(t *testing.T, dir string) {
	writeEPUB(t, filepath.Join(dir, "calvin", "institutes.epub"), epubOPF)
	writeDOCX(t, filepath.Join(dir, "sermons", "romans8.docx"))
	writeFile(t, filepath.Join(dir, "commentary.pdf"), pdfDoc)
	writeFile(t, filepath.Join(dir, "page.html"), `<html lang="de"><head><title>Der R&ouml;merbrief</title>
		<meta name="author" content="Karl Barth"><meta content="Theology" name="keywords"></head></html>`)
	writeFile(t, filepath.Join(dir, "notes.md"), "Intro text\n\n# Study on Hope\n\n## Part\n")
	writeFile(t, filepath.Join(dir, "pilgrim.txt"), "Title: The Pilgrim's Progress\nAuthor: John Bunyan\n\nText")
	writeFile(t, filepath.Join(dir, "ignored.jpg"), "not a book")
	writeFile(t, filepath.Join(dir, ".hidden", "secret.txt"), "hidden")
}

func addAndScan(t *testing.T, r *command.Registry, env *command.Env, dir string) (Folder, ScanResult) {
	t.Helper()
	var f Folder
	must(t, r, env, "library.add_folder", obj{"path": dir, "label": "Books"}, &f)
	var res ScanResult
	must(t, r, env, "library.scan", obj{"folder_id": f.ID}, &res)
	return f, res
}

func TestCommandsDescribeThemselves(t *testing.T) {
	r, _ := setup(t)
	n := 0
	for _, d := range r.List() {
		if !strings.HasPrefix(d.ID, "library.") {
			t.Errorf("command %s outside library namespace", d.ID)
		}
		if d.Summary == "" || d.Params == nil {
			t.Errorf("%s: incomplete description", d.ID)
		}
		n++
	}
	if n != 13 {
		t.Errorf("%d commands", n)
	}
	d, _ := r.Describe("library.set_licence")
	if len(d.Params.Properties["type"].Enum) != len(licence.Types) {
		t.Errorf("set_licence type enum: %+v", d.Params.Properties["type"])
	}
}

func TestMetadataExtraction(t *testing.T) {
	dir := t.TempDir()
	books(t, dir)
	cases := []struct {
		file, format string
		check        func(Meta) string
	}{
		{"calvin/institutes.epub", "epub", func(m Meta) string {
			if m.Title != "Institutes of the Christian Religion" || m.Subtitle != "Volume One" || m.Language != "en" ||
				m.Year != 1845 || m.Publisher != "Calvin Translation Society" || m.ISBN != "9780000000002" ||
				m.Series != "Institutes" || m.SeriesNo != "1" || m.Rights != "Public domain in the USA." {
				return fmt.Sprintf("%+v", m)
			}
			want := []Person{{"John Calvin", "Calvin, Jean", "author"}, {"Henry Beveridge", "Beveridge, Henry", "translator"}}
			if fmt.Sprint(m.People) != fmt.Sprint(want) || fmt.Sprint(m.Subjects) != "[Theology Reformation]" {
				return fmt.Sprintf("people %v subjects %v", m.People, m.Subjects)
			}
			return ""
		}},
		{"sermons/romans8.docx", "docx", func(m Meta) string {
			if m.Title != "Sermon on Romans 8" || m.Year != 2024 || m.Language != "en-GB" || len(m.People) != 2 ||
				m.People[1].Name != "Joe Bloggs" || fmt.Sprint(m.Subjects) != "[Romans hope suffering]" {
				return fmt.Sprintf("%+v", m)
			}
			return ""
		}},
		{"commentary.pdf", "pdf", func(m Meta) string {
			if m.Title != "Commentary on (Romans)" || len(m.People) != 1 || m.People[0].Name != "John Calvin" || m.Year != 1540 {
				return fmt.Sprintf("%+v", m)
			}
			return ""
		}},
		{"page.html", "html", func(m Meta) string {
			if m.Title != "Der Römerbrief" || m.Language != "de" || len(m.People) != 1 || m.People[0].SortName != "Barth, Karl" ||
				fmt.Sprint(m.Subjects) != "[Theology]" {
				return fmt.Sprintf("%+v", m)
			}
			return ""
		}},
		{"notes.md", "md", func(m Meta) string {
			if m.Title != "Study on Hope" {
				return fmt.Sprintf("%+v", m)
			}
			return ""
		}},
		{"pilgrim.txt", "txt", func(m Meta) string {
			if m.Title != "The Pilgrim's Progress" || len(m.People) != 1 || m.People[0].Name != "John Bunyan" {
				return fmt.Sprintf("%+v", m)
			}
			return ""
		}},
	}
	for _, c := range cases {
		if f := formatOf(c.file); f != c.format {
			t.Errorf("%s: format %q", c.file, f)
		}
		m, err := extract(filepath.Join(dir, c.file), c.format)
		if err != nil {
			t.Errorf("%s: %v", c.file, err)
		}
		if msg := c.check(m); msg != "" {
			t.Errorf("%s: %s", c.file, msg)
		}
	}
	writeFile(t, filepath.Join(dir, "front.md"), "---\ntitle: \"Grace\"\nauthor: [A One, B Two]\ndate: 2001\n---\n# Heading\n")
	if m, _ := extract(filepath.Join(dir, "front.md"), "md"); m.Title != "Grace" || len(m.People) != 2 || m.Year != 2001 {
		t.Errorf("front matter: %+v", m)
	}
	writeFile(t, filepath.Join(dir, "broken_book.epub"), "not a zip")
	if m, err := extract(filepath.Join(dir, "broken_book.epub"), "epub"); err == nil || m.Title != "broken book" {
		t.Errorf("broken epub: %+v, %v", m, err)
	}
}

func TestSuggestedLicences(t *testing.T) {
	cases := map[string]string{
		"Public domain in the USA.":                         "public_domain",
		"Licensed CC BY-SA 4.0":                             "creative_commons/CC-BY-SA-4.0",
		"https://creativecommons.org/licenses/by-nc/3.0/":   "creative_commons/CC-BY-NC-3.0",
		"https://creativecommons.org/publicdomain/zero/1.0": "creative_commons/CC0-1.0",
		"© 2020 Some Publisher. All rights reserved.":       "unknown",
		"": "unknown",
	}
	for rights, want := range cases {
		l := suggest(rights)
		got := string(l.Type)
		if l.SPDX != "" {
			got += "/" + l.SPDX
		}
		if got != want || l.Confirmed {
			t.Errorf("suggest(%q) = %s (confirmed %v), want %s", rights, got, l.Confirmed, want)
		}
	}
}

func TestScanImportsAndIsIdempotent(t *testing.T) {
	r, env := setup(t)
	dir := t.TempDir()
	books(t, dir)
	f, res := addAndScan(t, r, env, dir)
	if res.Added != 6 || len(res.Errors) != 0 || res.NeedsLicenceTotal != 6 || len(res.NeedsLicence) != 6 {
		t.Fatalf("first scan: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, MarkerName)); err != nil {
		t.Fatalf("no marker: %v", err)
	}
	for _, q := range res.NeedsLicence {
		want := licence.Unknown
		if q.Title == "Institutes of the Christian Religion" {
			want = licence.PublicDomain
		}
		if q.Suggested.Type != want || q.Suggested.Confirmed {
			t.Errorf("%s: suggested %+v", q.Title, q.Suggested)
		}
	}
	must(t, r, env, "library.scan", obj{"folder_id": f.ID}, &res)
	if res.Added != 0 || res.Unchanged != 6 || res.Changed+res.Removed+res.Moved != 0 || res.NeedsLicenceTotal != 6 {
		t.Fatalf("rescan: %+v", res)
	}
	must(t, r, env, "library.scan", obj{"folder_id": f.ID, "verify": true}, &res)
	if res.Unchanged != 6 {
		t.Fatalf("verified rescan: %+v", res)
	}
	var items ItemsResult
	must(t, r, env, "library.items", obj{}, &items)
	if items.Total != 6 {
		t.Fatalf("items after rescans: %d", items.Total)
	}
}

func TestScanFindsChangedMovedAndRemovedFiles(t *testing.T) {
	r, env := setup(t)
	dir := t.TempDir()
	books(t, dir)
	f, _ := addAndScan(t, r, env, dir)
	var before ItemsResult
	must(t, r, env, "library.items", obj{"text": "Pilgrim"}, &before)

	later := time.Now().Add(time.Minute)
	writeFile(t, filepath.Join(dir, "notes.md"), "# Study on Hope, revised\n")
	os.Chtimes(filepath.Join(dir, "notes.md"), later, later)
	os.MkdirAll(filepath.Join(dir, "classics"), 0o755)
	if err := os.Rename(filepath.Join(dir, "pilgrim.txt"), filepath.Join(dir, "classics", "pilgrim.txt")); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, "page.html"))
	writeFile(t, filepath.Join(dir, "new.txt"), "Title: Fresh\n")

	var res ScanResult
	must(t, r, env, "library.scan", obj{"folder_id": f.ID}, &res)
	if res.Changed != 1 || res.Moved != 1 || res.Removed != 1 || res.Added != 1 || res.Unchanged != 3 {
		t.Fatalf("scan after changes: %+v", res)
	}
	var after ItemsResult
	must(t, r, env, "library.items", obj{"text": "Pilgrim"}, &after)
	if after.Total != 1 || after.Items[0].ID != before.Items[0].ID {
		t.Fatalf("moved file got a new item: %+v vs %+v", after, before)
	}
	var it Item
	must(t, r, env, "library.item", obj{"id": after.Items[0].ID}, &it)
	if len(it.Files) != 1 || it.Files[0].RelPath != "classics/pilgrim.txt" || !it.Files[0].Online {
		t.Fatalf("moved file: %+v", it.Files)
	}
	var all ItemsResult
	must(t, r, env, "library.items", obj{}, &all)
	if all.Total != 6 {
		t.Fatalf("items = %d, want 6 (one removed, one added)", all.Total)
	}

	writeFile(t, filepath.Join(dir, "page.html"), `<html lang="de"><head><title>Der R&ouml;merbrief</title>
		<meta name="author" content="Karl Barth"><meta content="Theology" name="keywords"></head></html>`)
	must(t, r, env, "library.scan", obj{"folder_id": f.ID}, &res)
	if res.Restored != 1 || res.Added != 0 {
		t.Fatalf("returning file: %+v", res)
	}
	must(t, r, env, "library.items", obj{"text": "Römer"}, &all)
	if all.Total != 1 {
		t.Fatalf("restored item not listed: %+v", all)
	}
}

func TestMarkerSurvivesMovingTheFolder(t *testing.T) {
	r, env := setup(t)
	base := t.TempDir()
	dir := filepath.Join(base, "old", "Books")
	books(t, dir)
	f, _ := addAndScan(t, r, env, dir)

	moved := filepath.Join(base, "new", "Shelf")
	os.MkdirAll(filepath.Dir(moved), 0o755)
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	var folders []Folder
	must(t, r, env, "library.folders", obj{}, &folders)
	if len(folders) != 1 || folders[0].Online {
		t.Fatalf("moved folder should be offline: %+v", folders)
	}
	var items ItemsResult
	must(t, r, env, "library.items", obj{}, &items)
	if items.Items[0].Status != "offline" {
		t.Fatalf("item status = %s", items.Items[0].Status)
	}
	if err := run(t, r, env, "library.scan", obj{"folder_id": f.ID}, nil); code(err) != "unavailable" {
		t.Fatalf("scan of offline folder: %v", err)
	}

	must(t, r, env, "library.set_search_roots", obj{"roots": []string{filepath.Join(base, "new")}}, nil)
	must(t, r, env, "library.locate", obj{}, &folders)
	if !folders[0].Online || folders[0].Path != moved || folders[0].ID != f.ID {
		t.Fatalf("not found by marker: %+v", folders)
	}
	var res ScanResult
	must(t, r, env, "library.scan", obj{"folder_id": f.ID}, &res)
	if res.Unchanged != 6 || res.Added+res.Removed != 0 {
		t.Fatalf("scan after move: %+v", res)
	}
	h, _ := env.Store.History(context.Background(), "library_folders", f.ID)
	if len(h) < 2 || h[0].Command != "library.add_folder" || h[len(h)-2].Command != "library.locate" {
		t.Fatalf("folder history: %+v", h)
	}
}

func TestChangedDriveLetter(t *testing.T) {
	r, env := setup(t)
	base := t.TempDir()
	vol1, vol2 := filepath.Join(base, "vol1"), filepath.Join(base, "vol2")
	dir := filepath.Join(vol1, "Library", "Books")
	books(t, dir)
	vo := volumeOf
	volumeOf = func(p string) string {
		if strings.HasPrefix(p, vol1) {
			return vol1
		}
		return vo(p)
	}
	t.Cleanup(func() { volumeOf = vo })
	f, _ := addAndScan(t, r, env, dir)

	os.MkdirAll(vol2, 0o755)
	if err := os.Rename(filepath.Join(vol1, "Library"), filepath.Join(vol2, "Library")); err != nil {
		t.Fatal(err)
	}
	mountedRoots = func() []string { return []string{vol2} }
	var folders []Folder
	must(t, r, env, "library.folders", obj{}, &folders)
	want := filepath.Join(vol2, "Library", "Books")
	if !folders[0].Online || folders[0].Path != want {
		t.Fatalf("folder on remounted volume: %+v", folders)
	}
	var it ItemsResult
	must(t, r, env, "library.items", obj{"folder_id": f.ID}, &it)
	if it.Total != 6 || it.Items[0].Status != "online" {
		t.Fatalf("items on remounted volume: %+v", it)
	}
}

func TestRelocateAndAddRules(t *testing.T) {
	r, env := setup(t)
	base := t.TempDir()
	a, b, c := filepath.Join(base, "a"), filepath.Join(base, "b"), filepath.Join(base, "c")
	books(t, a)
	os.MkdirAll(c, 0o755)
	f, _ := addAndScan(t, r, env, a)
	if err := run(t, r, env, "library.add_folder", obj{"path": a}, nil); code(err) != "bad_params" {
		t.Fatalf("adding a folder twice: %v", err)
	}
	var other Folder
	must(t, r, env, "library.add_folder", obj{"path": c}, &other)
	if err := os.Rename(a, b); err != nil {
		t.Fatal(err)
	}
	if err := run(t, r, env, "library.relocate_folder", obj{"id": f.ID, "path": c}, nil); code(err) != "bad_params" {
		t.Fatalf("relocate onto another folder: %v", err)
	}
	var got Folder
	must(t, r, env, "library.relocate_folder", obj{"id": f.ID, "path": b}, &got)
	if !got.Online || got.Path != b {
		t.Fatalf("relocate: %+v", got)
	}
	os.Remove(filepath.Join(b, MarkerName))
	if err := run(t, r, env, "library.relocate_folder", obj{"id": f.ID, "path": b}, nil); code(err) != "bad_params" {
		t.Fatalf("relocate without marker: %v", err)
	}
	must(t, r, env, "library.relocate_folder", obj{"id": f.ID, "path": b, "force": true}, &got)
	if m, err := readMarker(b); err != nil || m.ID != got.MarkerID {
		t.Fatalf("forced relocate marker: %+v %v", m, err)
	}
	must(t, r, env, "library.rename_folder", obj{"id": f.ID, "label": "Theology"}, &got)
	if m, _ := readMarker(b); got.Label != "Theology" || m.Label != "Theology" {
		t.Fatalf("rename: %+v %+v", got, m)
	}
}

func TestRemoveAndReAddFolder(t *testing.T) {
	r, env := setup(t)
	dir := t.TempDir()
	books(t, dir)
	f, _ := addAndScan(t, r, env, dir)
	var rm RemoveResult
	must(t, r, env, "library.remove_folder", obj{"id": f.ID}, &rm)
	if rm.Files != 6 || rm.Items != 6 {
		t.Fatalf("remove: %+v", rm)
	}
	var folders []Folder
	must(t, r, env, "library.folders", obj{}, &folders)
	var items ItemsResult
	must(t, r, env, "library.items", obj{}, &items)
	if len(folders) != 0 || items.Total != 0 {
		t.Fatalf("after remove: %d folders, %d items", len(folders), items.Total)
	}
	var again Folder
	must(t, r, env, "library.add_folder", obj{"path": dir}, &again)
	if again.ID != f.ID {
		t.Fatalf("re-added folder has a new ID")
	}
	var res ScanResult
	must(t, r, env, "library.scan", obj{"folder_id": f.ID}, &res)
	if res.Restored != 6 || res.Added != 0 {
		t.Fatalf("scan after re-add: %+v", res)
	}
}

func TestLicenceFlow(t *testing.T) {
	r, env := setup(t)
	dir := t.TempDir()
	books(t, dir)
	f, res := addAndScan(t, r, env, dir)
	var epub, docx string
	for _, q := range res.NeedsLicence {
		switch q.Title {
		case "Institutes of the Christian Religion":
			epub = q.ItemID
		case "Sermon on Romans 8":
			docx = q.ItemID
		}
	}
	for _, bad := range []obj{
		{"type": "own_work"},
		{"item_ids": []string{docx}, "pending": true, "type": "own_work"},
		{"item_ids": []string{docx}},
		{"item_ids": []string{docx}, "type": "own_work", "use_suggested": true},
		{"item_ids": []string{docx}, "type": "creative_commons", "spdx": "MIT"},
	} {
		if err := run(t, r, env, "library.set_licence", bad, nil); code(err) != "bad_params" {
			t.Errorf("set_licence %v: %v", bad, err)
		}
	}
	if err := run(t, r, env, "library.set_licence", obj{"item_ids": []string{"nope"}, "type": "own_work"}, nil); code(err) != "not_found" {
		t.Errorf("unknown item: %v", err)
	}
	var out SetLicenceResult
	must(t, r, env, "library.set_licence", obj{"item_ids": []string{docx}, "type": "personal_purchase",
		"print_limit": obj{"pages": 20}, "rights_holder": "Jane Doe"}, &out)
	var it Item
	must(t, r, env, "library.item", obj{"id": docx}, &it)
	if it.Licence == nil || it.Licence.Type != licence.PersonalPurchase || !it.Licence.Confirmed ||
		it.Licence.Print.Pages != 20 || it.Licence.Export.Percent != 10 || it.Licence.RightsHolder != "Jane Doe" {
		t.Fatalf("licence: %+v", it.Licence)
	}
	var pending ItemsResult
	must(t, r, env, "library.items", obj{"needs_licence": true}, &pending)
	if pending.Total != 5 || pending.Items[0].Suggested == nil {
		t.Fatalf("pending: %+v", pending)
	}
	must(t, r, env, "library.set_licence", obj{"pending": true, "folder_id": f.ID, "use_suggested": true}, &out)
	if out.Updated != 5 {
		t.Fatalf("bulk updated %d", out.Updated)
	}
	must(t, r, env, "library.item", obj{"id": epub}, &it)
	if it.Licence == nil || it.Licence.Type != licence.PublicDomain || !it.Licence.Confirmed || it.Suggested != nil {
		t.Fatalf("epub licence: %+v", it.Licence)
	}
	must(t, r, env, "library.items", obj{"needs_licence": true}, &pending)
	if pending.Total != 0 {
		t.Fatalf("still pending: %d", pending.Total)
	}
	must(t, r, env, "library.set_licence", obj{"item_ids": []string{epub}, "type": "own_work"}, &out)
	var lid string
	env.Store.QueryRow(context.Background(), `SELECT id FROM licences WHERE item_id = ?`, epub).Scan(&lid)
	h, _ := env.Store.History(context.Background(), "licences", lid)
	if len(h) < 2 || h[0].Op != "create" || h[len(h)-1].Command != "library.set_licence" {
		t.Fatalf("licence history: %+v", h)
	}
}

func TestItemQueriesAndUpdates(t *testing.T) {
	r, env := setup(t)
	dir := t.TempDir()
	books(t, dir)
	f, _ := addAndScan(t, r, env, dir)
	counts := map[string]obj{
		"author":   {"author": "calvin"},
		"text":     {"text": "romans"},
		"facet":    {"facet": "subject", "value": "Theology"},
		"value":    {"value": "Reformation"},
		"kind":     {"kind": "book"},
		"no kind":  {"kind": "lexicon"},
		"folder":   {"folder_id": f.ID},
		"combined": {"author": "Calvin", "text": "institutes"},
	}
	want := map[string]int{"author": 2, "text": 2, "facet": 2, "value": 1, "kind": 6, "no kind": 0, "folder": 6, "combined": 1}
	for name, params := range counts {
		var res ItemsResult
		must(t, r, env, "library.items", params, &res)
		if res.Total != want[name] || len(res.Items) != want[name] {
			t.Errorf("%s: total %d, %d items, want %d", name, res.Total, len(res.Items), want[name])
		}
	}
	var page ItemsResult
	must(t, r, env, "library.items", obj{"limit": 2, "offset": 4}, &page)
	if page.Total != 6 || len(page.Items) != 2 {
		t.Fatalf("paging: %+v", page)
	}

	var res ItemsResult
	must(t, r, env, "library.items", obj{"text": "Institutes"}, &res)
	id := res.Items[0].ID
	if res.Items[0].Status != "online" || fmt.Sprint(res.Items[0].Authors) != "[John Calvin]" {
		t.Fatalf("summary: %+v", res.Items[0])
	}
	var it Item
	must(t, r, env, "library.item", obj{"id": id}, &it)
	if len(it.Files) != 1 || it.Files[0].Format != "epub" || len(it.Files[0].SHA256) != 64 ||
		it.Files[0].Path != filepath.Join(dir, "calvin", "institutes.epub") || len(it.People) != 2 || len(it.Facets) != 3 {
		t.Fatalf("item: %+v", it)
	}

	must(t, r, env, "library.update_item", obj{"id": id, "title": "Institutes", "kind": "monograph", "year": 1559,
		"authors": []string{"Jean Calvin", "John Calvin"}, "add_facets": []obj{{"facet": "tradition", "value": "Reformed"}},
		"remove_facets": []obj{{"facet": "subject", "value": "Reformation"}}}, &it)
	if it.Title != "Institutes" || it.Kind != "monograph" || it.Year != 1559 || len(it.People) != 3 {
		t.Fatalf("updated: %+v", it)
	}
	must(t, r, env, "library.items", obj{"facet": "tradition", "value": "Reformed"}, &res)
	if res.Total != 1 || fmt.Sprint(res.Items[0].Authors) != "[Jean Calvin John Calvin]" {
		t.Fatalf("after update: %+v", res)
	}
	if err := run(t, r, env, "library.update_item", obj{"id": id, "title": " "}, nil); code(err) != "bad_params" {
		t.Errorf("empty title: %v", err)
	}
	if err := run(t, r, env, "library.item", obj{"id": "nope"}, nil); code(err) != "not_found" {
		t.Errorf("unknown item: %v", err)
	}
	h, _ := env.Store.History(context.Background(), "items", id)
	cmds := map[string]bool{}
	for _, c := range h {
		cmds[c.Command+"/"+c.Field] = true
	}
	if !cmds["library.scan/"] || !cmds["library.update_item/title"] || !cmds["library.update_item/kind"] {
		t.Fatalf("item history: %v", cmds)
	}
}

func TestScanManyFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	r, env := setup(t)
	dir := t.TempDir()
	const n = 1200 // more than two write batches
	for i := 0; i < n; i++ {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("d%02d", i%20), fmt.Sprintf("book%04d.txt", i)),
			fmt.Sprintf("Title: Book %d\nAuthor: Author %d\n", i, i%50))
	}
	f, res := addAndScan(t, r, env, dir)
	if res.Added != n || res.NeedsLicenceTotal != n || len(res.NeedsLicence) != maxQuestions {
		t.Fatalf("scan: added %d, pending %d, questions %d", res.Added, res.NeedsLicenceTotal, len(res.NeedsLicence))
	}
	var people int
	env.Store.QueryRow(context.Background(), `SELECT COUNT(*) FROM people`).Scan(&people)
	if people != 50 {
		t.Fatalf("people = %d, want 50", people)
	}
	must(t, r, env, "library.scan", obj{"folder_id": f.ID}, &res)
	if res.Unchanged != n {
		t.Fatalf("rescan: %+v", res.Unchanged)
	}
	var out SetLicenceResult
	must(t, r, env, "library.set_licence", obj{"pending": true, "type": "personal_purchase"}, &out)
	if out.Updated != n {
		t.Fatalf("bulk licence: %d", out.Updated)
	}
}
