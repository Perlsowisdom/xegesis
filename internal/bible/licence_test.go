package bible

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/licence"
)

func TestInstallTakesTheFilesOwnLicence(t *testing.T) {
	e := setup(t)
	withRights := strings.Replace(osisKJV, `<osisText osisIDWork="KJVtest" xml:lang="en">`,
		`<osisText osisIDWork="KJVtest" xml:lang="en"><header><work osisWork="KJVtest">`+
			`<rights>Public Domain</rights></work></header>`, 1)
	cases := []struct {
		name, file, body string
		want             licence.Type
		origin, credit   string
	}{
		{"osis rights", "pd.xml", withRights, licence.PublicDomain, licence.FromFile, ""},
		{"osis without", "plain.xml", osisKJV, licence.PersonalPurchase, licence.ByDefault, ""},
		{"usfm rem", "cc.usfm", "\\id MAL test\n\\rem Licensed under CC BY-SA 4.0. Copyright © 2020 Example Society.\n" +
			usfmMal[strings.Index(usfmMal, "\n")+1:], licence.CreativeCommons, licence.FromFile, "Copyright © 2020 Example Society"},
		{"usfm copyright", "c.usfm", "\\id MAL test\n\\h Malachi\n\\toc1 Copyright © 2001 Example Press. All rights reserved.\n" +
			usfmMal[strings.Index(usfmMal, "\n")+1:], licence.PersonalPurchase, licence.ByDefault, "Copyright © 2001 Example Press"},
	}
	for i, c := range cases {
		var res InstallResult
		e.must("bible.install", map[string]any{"path": e.file(c.file, c.body), "id": "m" + string(rune('a'+i)), "name": c.name}, &res)
		l := res.Module.Licence
		if l.Type != c.want || l.Origin != c.origin || l.Attribution != c.credit || l.Confirmed {
			t.Errorf("%s: %+v", c.name, l)
		}
	}

	// An eBible.org zip names its licence in copr.htm.
	zpath := filepath.Join(t.TempDir(), "eng-test_usfm.zip")
	f, _ := os.Create(zpath)
	zw := zip.NewWriter(f)
	for name, body := range map[string]string{
		"eng-test/copr.htm":      `<html><body><p>Copyright © 2010 Example. <a href="http://creativecommons.org/licenses/by-nd/4.0/">CC BY-ND</a></p></body></html>`,
		"eng-test/39MALtest.SFM": usfmMal,
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	f.Close()
	var res InstallResult
	e.must("bible.install", map[string]any{"path": zpath, "id": "zipped", "name": "Zipped"}, &res)
	if l := res.Module.Licence; l.Type != licence.CreativeCommons || l.SPDX != "CC-BY-ND-4.0" || l.Origin != licence.FromFile {
		t.Errorf("zip: %+v", l)
	}

	// The licence can be changed afterwards.
	var m Meta
	e.must("bible.set_licence", map[string]any{"module": "zipped", "type": "own_work"}, &m)
	if m.Licence.Type != licence.OwnWork || m.Licence.Origin != licence.ByUser || !m.Licence.Confirmed {
		t.Errorf("set licence: %+v", m.Licence)
	}
	var list []Meta
	e.must("bible.modules", struct{}{}, &list)
	for _, x := range list {
		if x.ID == "zipped" && x.Licence.Type != licence.OwnWork {
			t.Errorf("after reopening: %+v", x.Licence)
		}
	}
	e.must("bible.read", map[string]any{"module": "zipped", "ref": "Mal 3:23"}, nil)
	if err := e.run("bible.set_licence", map[string]any{"module": "nope", "type": "own_work"}, nil); code(err) != "not_found" {
		t.Errorf("unknown module: %v", err)
	}
}
