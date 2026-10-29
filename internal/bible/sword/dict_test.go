package sword_test

import (
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/bible/sword"
	"github.com/Perlsowisdom/xegesis/internal/bible/sword/swordtest"
)

var dictEntries = [][2]string{
	{"00025", `<entryFree n="25"><orth>ἀγαπάω</orth> <def>to love</def></entryFree>`},
	{"00026", `<entryFree n="26"><orth>ἀγάπη</orth> <def>love</def></entryFree>`},
}

func TestReadDictionaries(t *testing.T) {
	for _, drv := range []string{"zLD", "RawLD"} {
		t.Run(drv, func(t *testing.T) {
			dir := t.TempDir()
			write := swordtest.ZLD
			if drv == "RawLD" {
				write = swordtest.RawLD
			}
			if err := write(dir, "dict", dictEntries); err != nil {
				t.Fatal(err)
			}
			c := parse(t, "[G]\nModDrv="+drv+"\nDataPath=./modules/lexdict/g/dict\nSourceType=TEI\nCompressType=ZIP\nFeature=GreekDef\n")
			if err := sword.Supported(c); err != nil || c.Kind() != "dictionary" || c.DataPath() != "modules/lexdict/g/" {
				t.Fatalf("supported %v kind %s path %q", err, c.Kind(), c.DataPath())
			}
			if f := sword.DataFiles(c); f[0] != "dict.idx" {
				t.Fatalf("files %v", f)
			}
			d, err := sword.OpenDict(dir, c)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			if err := d.Walk(func(k, s string) error { got[k] = s; return nil }); err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 || !strings.Contains(got["00026"], "ἀγάπη") {
				t.Fatalf("entries %v", got)
			}
		})
	}
}

func TestReadCommentary(t *testing.T) {
	dir := t.TempDir()
	if err := swordtest.ZText(dir, map[swordtest.Key]string{swordtest.K(43, 3, 16): "God's love for the world."}); err != nil {
		t.Fatal(err)
	}
	c := parse(t, fixFirst(swordtest.Conf("Com", "zCom", "")))
	if c.Kind() != "commentary" || sword.Supported(c) != nil {
		t.Fatalf("commentary kind %s: %v", c.Kind(), sword.Supported(c))
	}
	got := walk(t, dir, c)
	if got[swordtest.ID(swordtest.K(43, 3, 16))] != "God's love for the world." {
		t.Fatalf("got %v", got)
	}
}
