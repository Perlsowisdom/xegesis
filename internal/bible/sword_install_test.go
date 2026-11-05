package bible

import (
	"archive/zip"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/bible/sword/swordtest"
	"github.com/Perlsowisdom/xegesis/internal/licence"
)

var swordVerses = map[swordtest.Key]string{
	swordtest.K(43, 3, 16): `For God so loved the world.`,
	swordtest.K(1, 1, 1):   `In the beginning God created the heaven and the earth.`,
}

// swordTree writes a SWORD repository tree with the named modules; extra[name] is added to its .conf.
func swordTree(t *testing.T, root string, extra map[string]string) {
	t.Helper()
	for name, more := range extra {
		dir := filepath.Join(root, "modules", "texts", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := swordtest.ZText(dir, swordVerses); err != nil {
			t.Fatal(err)
		}
		conf := swordtest.Conf(name, "zText", more)
		if strings.Contains(more, "DistributionLicense=") {
			conf = strings.Replace(conf, "DistributionLicense=Public Domain\n", "", 1)
		}
		if err := os.MkdirAll(filepath.Join(root, "mods.d"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "mods.d", strings.ToLower(name)+".conf"), []byte(conf), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func zipDir(t *testing.T, dir, out string) {
	t.Helper()
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		w, _ := zw.Create(filepath.ToSlash(rel))
		b, _ := os.ReadFile(p)
		w.Write(b)
		return nil
	})
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestInstallSWORDZip(t *testing.T) {
	e := setup(t)
	tree := t.TempDir()
	swordTree(t, tree, map[string]string{
		"Free":   "",
		"Pricey": "DistributionLicense=Copyrighted; Permission to distribute granted to CrossWire\nCopyright=Copyright 2020 Some Publisher\n",
	})
	zipPath := filepath.Join(t.TempDir(), "mods.zip")
	zipDir(t, tree, zipPath)

	var res InstallResult
	e.must("bible.install", map[string]any{"path": zipPath}, &res)
	if len(res.Modules) != 2 {
		t.Fatalf("installed %d modules: %+v", len(res.Modules), res.Warnings)
	}
	byID := map[string]Meta{}
	for _, m := range res.Modules {
		byID[m.ID] = m
	}
	if byID["free"].Licence.Type != licence.PublicDomain {
		t.Errorf("free licence = %+v", byID["free"].Licence)
	}
	p := byID["pricey"].Licence
	if p.Type != licence.PersonalPurchase || p.Share || !strings.Contains(p.Attribution, "Some Publisher") {
		t.Errorf("copyrighted module should be a personal copy, got %+v", p)
	}
	if got := e.read("free", "John 3:16"); len(got.Verses) != 1 || !strings.Contains(got.Verses[0].Text, "God so loved") {
		t.Errorf("read = %+v", got)
	}
	// A folder works as well as a zip, and the user's licence overrides the .conf's.
	var again InstallResult
	e.must("bible.install", map[string]any{"path": tree, "replace": true, "licence_type": "own_work"}, &again)
	if len(again.Modules) != 2 || again.Modules[0].Licence.Type != licence.OwnWork {
		t.Errorf("folder install = %+v", again.Modules)
	}
}

func TestInstallSWORDLockedRefused(t *testing.T) {
	e := setup(t)
	tree := t.TempDir()
	swordTree(t, tree, map[string]string{"Locked": "CipherKey=abc\n", "Open": ""})
	zipPath := filepath.Join(t.TempDir(), "mods.zip")
	zipDir(t, tree, zipPath)

	var res InstallResult
	e.must("bible.install", map[string]any{"path": zipPath}, &res)
	if len(res.Modules) != 1 || res.Modules[0].ID != "open" {
		t.Fatalf("modules = %+v", res.Modules)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "locked") {
		t.Errorf("warnings do not mention the locked module: %v", res.Warnings)
	}

	// Only a locked module: refused with a clear message, nothing installed.
	only := t.TempDir()
	swordTree(t, only, map[string]string{"Locked": "CipherKey=abc\n"})
	err := e.run("bible.install", map[string]any{"path": only}, nil)
	if err == nil || !strings.Contains(err.Error(), "locked") || !strings.Contains(err.Error(), "unlock key") {
		t.Errorf("locked install error = %v", err)
	}
	var list []Meta
	e.must("bible.modules", map[string]any{"kind": "all"}, &list)
	for _, m := range list {
		if m.ID == "locked" {
			t.Errorf("locked module was installed")
		}
	}
}
