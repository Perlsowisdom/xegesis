package bible

import (
	"context"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

func installEntries(t *testing.T, markup EntryMarkup, entries []Entry) (*Module, []string) {
	t.Helper()
	l := NewLibrary(t.TempDir())
	t.Cleanup(l.Close)
	lic, _ := licence.Defaults(licence.PublicDomain, "")
	_, warnings, err := l.InstallEntries(context.Background(), InstallOptions{ID: "t", Name: "Test", Licence: lic},
		markup, func(emit func(Entry) error) error {
			for _, e := range entries {
				if err := emit(e); err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	m, err := l.Get("t")
	if err != nil {
		t.Fatal(err)
	}
	return m, warnings
}

func readAll(t *testing.T, m *Module) []Verse {
	t.Helper()
	vs, err := m.Read(context.Background(), verse.Range{Start: 0, End: 999_999_999})
	if err != nil {
		t.Fatal(err)
	}
	return vs
}

func TestOSISEntries(t *testing.T) {
	m, warnings := installEntries(t, "osis", []Entry{
		{ID: 1001001, Intro: true, Markup: `<div type="book" osisID="Gen" sID="b1"/><title type="main">Genesis</title>introduction prose`},
		{ID: 1001001, Markup: `<verse osisID="Gen.1.1"/><w lemma="strong:H07225">In the beginning</w> God created.`},
		{ID: 43003016, Markup: `<q who="Jesus" sID="q1"/>For God so loved`},
		{ID: 43003017, Markup: `the world.<q who="Jesus" eID="q1"/> <note>a note</note>`},
	})
	vs := readAll(t, m)
	if len(vs) != 3 {
		t.Fatalf("verses: %+v", vs)
	}
	if !strings.HasPrefix(vs[0].Markup, "<h>Genesis</h>") || strings.Contains(vs[0].Text, "introduction") {
		t.Errorf("Gen 1:1 = %q / %q", vs[0].Markup, vs[0].Text)
	}
	if !strings.Contains(vs[0].Markup, `<w s="H7225">`) {
		t.Errorf("Strong's lost: %q", vs[0].Markup)
	}
	if vs[1].Markup != "<j>For God so loved</j>" || !strings.HasPrefix(vs[2].Markup, "<j>the world.</j>") {
		t.Errorf("words of Christ across entries: %q, %q", vs[1].Markup, vs[2].Markup)
	}
	if vs[2].Text != "the world." {
		t.Errorf("note text in verse: %q", vs[2].Text)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings: %v", warnings)
	}
}

func TestThMLAndGBFEntries(t *testing.T) {
	m, _ := installEntries(t, "thml", []Entry{
		{ID: 1001001, Markup: `<div class="sechead">Creation</div>In the beginning<sync type="Strongs" value="H7225"/> God<note>Heb. Elohim</note> created.<br>`},
		{ID: 1001002, Markup: `See <scripRef passage="John 1:1">John 1:1</scripRef>.`},
	})
	vs := readAll(t, m)
	if vs[0].Text != "In the beginning God created." || !strings.HasPrefix(vs[0].Markup, "<h>Creation</h>") {
		t.Errorf("ThML verse: %q / %q", vs[0].Text, vs[0].Markup)
	}
	if !strings.Contains(vs[1].Markup, `<r o="John.1.1">John 1:1</r>`) {
		t.Errorf("ThML reference: %q", vs[1].Markup)
	}
	if hits, _ := m.SearchStrong(context.Background(), "H7225", verse.Range{End: 999_999_999}, 5); len(hits) != 1 {
		t.Errorf("ThML Strong's: %v", hits)
	}

	m, _ = installEntries(t, "gbf", []Entry{{ID: 40001001, Markup: `<FR>The book<WG976><Fr> of the generation<RF>a note<Rf>`}})
	if vs := readAll(t, m); vs[0].Text != "The book of the generation" || !strings.Contains(vs[0].Markup, "<f>a note</f>") {
		t.Errorf("GBF: %q / %q", vs[0].Text, vs[0].Markup)
	}
}
