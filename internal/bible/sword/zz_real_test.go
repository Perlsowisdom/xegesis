package sword

import (
	"os"
	"strings"
	"testing"
)

func TestZZDict(t *testing.T) {
	dir := os.Getenv("SAMPLES")
	if dir == "" {
		t.Skip()
	}
	for _, tc := range [][3]string{{"hitch", "zLD", "dict"}, {"sg", "zLD", "dict"}, {"sh", "RawLD", "strongshebrew"}} {
		c, _ := ParseConf(strings.NewReader("[x]\nModDrv=" + tc[1] + "\nCompressType=ZIP\nSourceType=TEI\nDataPath=./a/b/" + tc[2] + "\n"))
		d, err := OpenDict(dir+"/"+tc[0], c)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		d.Walk(func(k, s string) error {
			n++
			if n < 3 || k == "00026" || k == "07225" {
				if len(s) > 1400 {
					s = s[:1400]
				}
				t.Logf("%s %q: %s", tc[0], k, s)
			}
			return nil
		})
		t.Logf("%s: %d entries, folder %q files %v", tc[0], n, c.DataPath(), DataFiles(c))
	}
}
