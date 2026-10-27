package bible

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	f.Close()
}

func TestInstallFromZip(t *testing.T) {
	e := setup(t)
	path := filepath.Join(e.src, "web_usfm.zip")
	writeZip(t, path, map[string]string{"web/MAL.usfm": usfmMal, "web/JHN.usfm": usfmJhn})
	res := e.install(path, map[string]any{"id": "web", "name": "World English Bible", "versification": "hebrew"})
	if res.Module.ID != "web" {
		t.Fatalf("installed %+v", res.Module)
	}
	if p := e.read("web", "John 3:16"); len(p.Verses) != 1 {
		t.Fatalf("John 3:16 from zip: %+v", p)
	}
}

func TestZipRefusesEscapingEntries(t *testing.T) {
	e := setup(t)
	path := filepath.Join(e.src, "bad.zip")
	writeZip(t, path, map[string]string{"../escape.usfm": usfmMal})
	err := e.run("bible.install", map[string]any{"path": path, "licence_type": "public_domain"}, nil)
	if code(err) != "bad_params" {
		t.Fatalf("escaping entry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(os.TempDir()), "escape.usfm")); err == nil {
		t.Fatal("entry was written outside the archive folder")
	}
}
