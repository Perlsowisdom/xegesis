// Command gen writes a small SWORD module .zip for the real-app checks: one copyrighted Bible and
// one locked module (which the app must refuse). Usage: go run ./internal/bible/sword/swordtest/gen out.zip
package main

import (
	"archive/zip"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Perlsowisdom/xegesis/internal/bible/sword/swordtest"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gen out.zip")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(out string) error {
	root, err := os.MkdirTemp("", "xegesis-swordgen-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	verses := map[swordtest.Key]string{
		swordtest.K(43, 3, 16):  "For God so loved the world, that he gave his one and only Son.",
		swordtest.K(45, 12, 11): "Do not be slothful in zeal, be fervent in spirit.",
		swordtest.K(1, 1, 1):    "In the beginning God created the heavens and the earth.",
	}
	mods := map[string]string{
		"SwordE2E":  "DistributionLicense=Copyrighted; Permission to distribute granted to CrossWire\nCopyright=Copyright 2020 Test Publisher\nAbbreviation=SE2E\n",
		"LockedE2E": "CipherKey=\n",
	}
	if err := os.MkdirAll(filepath.Join(root, "mods.d"), 0o755); err != nil {
		return err
	}
	for name, extra := range mods {
		dir := filepath.Join(root, "modules", "texts", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := swordtest.ZText(dir, verses); err != nil {
			return err
		}
		conf := swordtest.Conf(name, "zText", extra)
		if extra != "" && extra[:4] == "Dist" {
			conf = replaceFirst(conf, "DistributionLicense=Public Domain\n", "")
		}
		if err := os.WriteFile(filepath.Join(root, "mods.d", name+".conf"), []byte(conf), 0o644); err != nil {
			return err
		}
	}
	return zipDir(root, out)
}

func replaceFirst(s, old, new string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}

func zipDir(dir, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	})
	if err != nil {
		return err
	}
	return zw.Close()
}
