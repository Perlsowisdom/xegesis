package sword_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/bible/sword"
	"github.com/Perlsowisdom/xegesis/internal/bible/sword/swordtest"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

func parse(t *testing.T, s string) sword.Conf {
	t.Helper()
	c, err := sword.ParseConf(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// testdata/asv.conf is CrossWire's own conf file for the public-domain ASV, as published.
func TestParseRecordedConf(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "asv.conf"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c, err := sword.ParseConf(f)
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{"ModDrv": "zText", "Versification": "KJV", "Lang": "en",
		"DistributionLicense": "Public Domain", "DataPath": "./modules/texts/ztext/asv/", "Version": "2.0"}
	for k, want := range checks {
		if got := c.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if c.Name != "ASV" || c.Kind() != "bible" || c.Size() != 3491824 || c.Locked() {
		t.Errorf("conf %q kind %s size %d locked %v", c.Name, c.Kind(), c.Size(), c.Locked())
	}
	if n := len(c.All("GlobalOptionFilter")); n != 7 {
		t.Errorf("GlobalOptionFilter has %d values, want 7", n)
	}
	if strings.Contains(c.About(), `\par`) || !strings.Contains(c.About(), "\n") {
		t.Errorf("About not converted from RTF: %q", c.About()[:80])
	}
	if err := sword.Supported(c); err != nil {
		t.Errorf("ASV not supported: %v", err)
	}
}

func TestConfSyntax(t *testing.T) {
	c := parse(t, "# comment\n\n[Test_1]\nAbout=First line\\\n second line\\\n third\nCopyright=© 2001\n"+
		"Description_fr=Essai\nFeature=A\nFeature=B\nCipherKey=\nBad line without equals\n")
	if c.Name != "Test_1" || c.Get("About") != "First line\n second line\n third" {
		t.Fatalf("parsed %q about %q", c.Name, c.Get("About"))
	}
	if c.Get("Description_fr") != "Essai" || len(c.All("Feature")) != 2 || !c.Locked() {
		t.Fatalf("keys: %q %v locked=%v", c.Get("Description_fr"), c.All("Feature"), c.Locked())
	}
	if !errors.Is(sword.Supported(c), sword.ErrLocked) {
		t.Fatal("locked module reported as supported")
	}
	if _, err := sword.ParseConf(strings.NewReader("Description=no header\n")); err == nil {
		t.Fatal("conf without [ModName] accepted")
	}
	if got := sword.PlainRTF(`\qc Title\par\pard caf\u233?`); got != "Title\ncafé" {
		t.Fatalf("PlainRTF = %q", got)
	}
}

func TestSupported(t *testing.T) {
	for _, tc := range []struct{ extra, want string }{
		{"", ""},
		{"Versification=LXX\n", "LXX verse numbering"},
		{"CompressType=LZSS\n", "LZSS compression"},
		{"Encoding=SCSU\n", "encoding"},
	} {
		c := parse(t, fixFirst(swordtest.Conf("T", "zText", tc.extra)))
		err := sword.Supported(c)
		if (tc.want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%q: %v", tc.extra, err)
		}
	}
	c := parse(t, swordtest.Conf("T", "zLD", ""))
	if sword.Supported(c) != nil || c.Kind() != "dictionary" {
		t.Errorf("dictionary: %v", sword.Supported(c))
	}
	if c := parse(t, swordtest.Conf("B", "RawGenBook", "")); sword.Supported(c) == nil {
		t.Error("general book reported as supported")
	}
}

// fixFirst keeps the last value of each key, so test overrides replace the defaults.
func fixFirst(conf string) string {
	lines := strings.Split(conf, "\n")
	last := map[string]int{}
	for i, l := range lines {
		if k, _, ok := strings.Cut(l, "="); ok {
			last[k] = i
		}
	}
	var out []string
	for i, l := range lines {
		if k, _, ok := strings.Cut(l, "="); ok && last[k] != i {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

var tiny = map[swordtest.Key]string{
	swordtest.K(1, 0, 0):    `<title type="main">THE FIRST BOOK OF MOSES</title>`,
	swordtest.K(1, 1, 1):    `<w lemma="strong:H7225">In the beginning</w> God created the heaven and the earth.`,
	swordtest.K(1, 1, 2):    `And the earth was without form, and void.`,
	swordtest.K(19, 23, 1):  `<title type="psalm" canonical="true">A Psalm of David.</title>The LORD is my shepherd; I shall not want.`,
	swordtest.K(66, 22, 21): `The grace of our Lord Jesus Christ be with you all. Amen.`,
}

func walk(t *testing.T, dir string, c sword.Conf) map[verse.ID]string {
	t.Helper()
	text, err := sword.Open(dir, c)
	if err != nil {
		t.Fatal(err)
	}
	got := map[verse.ID]string{}
	err = text.Walk(func(id verse.ID, intro bool, m string) error {
		if intro {
			m = "intro:" + m
		}
		got[id] += m
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestReadTinyModules(t *testing.T) {
	for _, drv := range []string{"zText", "RawText"} {
		t.Run(drv, func(t *testing.T) {
			dir := t.TempDir()
			write := swordtest.ZText
			if drv == "RawText" {
				write = swordtest.RawText
			}
			if err := write(dir, tiny); err != nil {
				t.Fatal(err)
			}
			got := walk(t, dir, parse(t, swordtest.Conf("Tiny", drv, "")))
			if len(got) != 4 {
				t.Fatalf("got %d verses: %v", len(got), got)
			}
			if g := got[swordtest.ID(swordtest.K(1, 1, 1))]; !strings.HasPrefix(g, "intro:<title") || !strings.Contains(g, "In the beginning") {
				t.Errorf("Gen 1:1 = %q", g)
			}
			if g := got[swordtest.ID(swordtest.K(66, 22, 21))]; g != tiny[swordtest.K(66, 22, 21)] {
				t.Errorf("Rev 22:21 = %q", g)
			}
		})
	}
}

func TestWrongEntryCountRefused(t *testing.T) {
	dir := t.TempDir()
	if err := swordtest.ZText(dir, tiny); err != nil {
		t.Fatal(err)
	}
	idx := filepath.Join(dir, "ot.bzv")
	data, _ := os.ReadFile(idx)
	os.WriteFile(idx, data[:len(data)-10], 0o644)
	_, err := sword.Open(dir, parse(t, swordtest.Conf("Tiny", "zText", "")))
	if err == nil || !strings.Contains(err.Error(), "numbering needs") {
		t.Fatalf("short index: %v", err)
	}
}

func TestKJVCounts(t *testing.T) {
	v, ok := sword.LookupVersification("")
	if !ok || len(v.OT) != 39 || len(v.NT) != 27 {
		t.Fatal("KJV numbering missing")
	}
	// 2 + books + chapters + verses, as in CrossWire's KJV and ASV index files.
	if v.Entries(0) != 24115 || v.Entries(1) != 8246 {
		t.Fatalf("entries %d %d", v.Entries(0), v.Entries(1))
	}
}
