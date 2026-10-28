package scaletest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/search"
)

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	st, err := Generate(Config{Dir: dir, Books: 60, Seed: 3, MaxPages: 40})
	if err != nil {
		t.Fatal(err)
	}
	if st.Books != 60 || st.Formats["epub"] == 0 || st.Formats["epub"]+st.Formats["pdf"]+st.Formats["docx"] != 60 {
		t.Fatalf("stats %+v", st)
	}
	if st.RareBooks != 1 || st.PhraseBooks != 2 || st.TextBytes < 60*3*PageText {
		t.Fatalf("stats %+v", st)
	}
	var files int
	var text strings.Builder
	for _, root := range []string{st.Internal, st.External} {
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			files++
			secs, err := search.ReadSections(p, strings.TrimPrefix(filepath.Ext(p), "."))
			if err != nil || len(secs) == 0 {
				t.Errorf("%s: %v", p, err)
			}
			for _, s := range secs {
				for _, b := range s.Blocks {
					text.WriteString(b.Text + "\n")
				}
			}
			return nil
		})
	}
	if files != 60 {
		t.Fatalf("%d files", files)
	}
	all := text.String()
	if !strings.Contains(all, RareWord) || !strings.Contains(all, Phrase) || len(search.FindRefs(all)) == 0 {
		t.Error("planted text or references missing")
	}
	again, _ := Generate(Config{Dir: t.TempDir(), Books: 60, Seed: 3, MaxPages: 40})
	if again.TextBytes != st.TextBytes {
		t.Error("the same seed gave a different library")
	}
}
