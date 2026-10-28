package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/licence"
)

func useBooksDir(t *testing.T) string {
	dir := filepath.Join(t.TempDir(), LibraryFolderName)
	old := booksDir
	booksDir = func() string { return dir }
	t.Cleanup(func() { booksDir = old })
	return dir
}

func TestAddMixedFolderAsksNothing(t *testing.T) {
	r, env := setup(t)
	useBooksDir(t)
	dir := t.TempDir()
	writeEPUB(t, filepath.Join(dir, "calvin.epub"), epubOPF)
	writeFile(t, filepath.Join(dir, "commentary.pdf"), pdfDoc)
	writeDOCX(t, filepath.Join(dir, "sermons", "romans8.docx"))

	var res AddResult
	must(t, r, env, "library.add", obj{"paths": []string{dir}}, &res)
	want := "3 books added: 1 public domain (from its own details), 2 personal copies."
	if res.Summary != want || res.Added != 3 || res.Copied != 0 || len(res.Folders) != 1 || len(res.Errors) != 0 {
		t.Fatalf("add: %+v", res)
	}
	var just ItemsResult
	must(t, r, env, "library.items", obj{"added_since": res.Since}, &just)
	if just.Total != 3 {
		t.Fatalf("just added: %d", just.Total)
	}

	// Adding it again finds nothing new and does not add a second folder.
	must(t, r, env, "library.add", obj{"paths": []string{dir, filepath.Join(dir, "calvin.epub")}}, &res)
	if res.Summary != "No new books found." || len(res.Folders) != 1 || res.Copied != 0 {
		t.Fatalf("again: %+v", res)
	}
	var fs []Folder
	must(t, r, env, "library.folders", obj{}, &fs)
	if len(fs) != 1 {
		t.Fatalf("folders: %+v", fs)
	}

	// The folder is watched: a new file is found by the rescan.
	writeFile(t, filepath.Join(dir, "notes.md"), "# Study on Hope\n")
	var rs RescanResult
	must(t, r, env, "library.rescan", obj{}, &rs)
	if rs.Scanned != 1 || rs.Added != 1 {
		t.Fatalf("rescan: %+v", rs)
	}
}

func TestAddFilesCopiesIntoTheLibraryFolder(t *testing.T) {
	r, env := setup(t)
	lib := useBooksDir(t)
	src := t.TempDir()
	a := filepath.Join(src, "a", "book.txt")
	b := filepath.Join(src, "b", "book.txt")
	writeFile(t, a, "Title: First\n")
	writeFile(t, b, "Title: Second\n")
	writeFile(t, filepath.Join(src, "photo.jpg"), "x")

	var res AddResult
	must(t, r, env, "library.add_files", obj{"files": []string{a, b, filepath.Join(src, "photo.jpg"), filepath.Join(src, "gone.pdf")}}, &res)
	if res.Added != 2 || res.Copied != 2 || len(res.Errors) != 2 ||
		res.Summary != "2 books added: 2 personal copies. 2 files could not be added; see the list for why." {
		t.Fatalf("add files: %+v", res)
	}
	for _, name := range []string{"book.txt", "book (2).txt", MarkerName} {
		if _, err := os.Stat(filepath.Join(lib, name)); err != nil {
			t.Errorf("library folder: %v", err)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(lib, "book.txt")); string(got) != "Title: First\n" {
		t.Errorf("first copy overwritten: %q", got)
	}

	// The same file again is not copied twice; a file already in the library folder is left where it is.
	must(t, r, env, "library.add_files", obj{"files": []string{a, filepath.Join(lib, "book (2).txt")}}, &res)
	if res.Copied != 0 || res.Added != 0 || len(res.Folders) != 1 {
		t.Fatalf("again: %+v", res)
	}
	entries, _ := os.ReadDir(lib)
	if len(entries) != 3 {
		t.Fatalf("library folder has %d entries", len(entries))
	}
	var it Item
	must(t, r, env, "library.item", obj{"id": itemID(t, r, env, "First")}, &it)
	if it.Licence.Type != licence.PersonalPurchase || it.Licence.Origin != licence.ByDefault || it.LicenceName != "Personal copy" {
		t.Fatalf("licence: %+v", it.Licence)
	}
	if err := run(t, r, env, "library.add", obj{"paths": []string{}}, nil); code(err) != "bad_params" {
		t.Errorf("no paths: %v", err)
	}
	must(t, r, env, "library.add", obj{"paths": []string{""}}, &res)
	if res.Added != 0 || len(res.Errors) != 1 || len(res.Folders) != 0 {
		t.Errorf("empty path: %+v", res)
	}
	if err := run(t, r, env, "library.add_folder", obj{"path": " "}, nil); code(err) != "bad_params" {
		t.Errorf("empty folder path: %v", err)
	}
}

func TestEPUBLicenceMetadata(t *testing.T) {
	opf := strings.Replace(epubOPF, "<dc:rights>Public domain in the USA.</dc:rights>",
		`<meta property="dcterms:license">https://creativecommons.org/licenses/by-sa/4.0/</meta>`, 1)
	path := filepath.Join(t.TempDir(), "b.epub")
	writeEPUB(t, path, opf)
	m, err := extract(path, "epub")
	if err != nil {
		t.Fatal(err)
	}
	if l := suggest(m.Rights); l.Type != licence.CreativeCommons || l.SPDX != "CC-BY-SA-4.0" || l.Origin != licence.FromFile {
		t.Fatalf("rights %q gave %+v", m.Rights, l)
	}
}

func TestSummary(t *testing.T) {
	r := AddResult{Added: 1, Licences: []LicenceCount{{Licence: "Free to share", Origin: licence.FromFile, Count: 1}}}
	if s := summary(r); s != "1 book added: 1 free to share (from its own details)." {
		t.Fatal(s)
	}
}
