package bible

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/command"
)

// Fixtures are a few public-domain verses (KJV and World English Bible), typed by hand.

const osisKJV = `<?xml version="1.0" encoding="UTF-8"?>
<osis xmlns="http://www.bibletechnologies.net/2003/OSIS/namespace">
<osisText osisIDWork="KJVtest" xml:lang="en">
<header><work osisWork="KJVtest"><title>King James Version (test)</title></work></header>
<div type="book" osisID="Gen"><chapter osisID="Gen.1">
<title type="main">The Creation</title>
<verse osisID="Gen.1.1" sID="Gen.1.1"/><w lemma="strong:H07225">In the beginning</w> <w lemma="strong:H0430" morph="x:N">God</w> created the heaven and the earth.<note type="study">Hebrew: bara</note><verse eID="Gen.1.1"/>
<verse sID="Gen.1.2" osisID="Gen.1.2"/>And the earth was without form, and void.<verse eID="Gen.1.2"/>
</chapter></div>
<div type="book" osisID="John"><chapter osisID="John.3">
<verse osisID="John.3.16">For God so loved the world, that he gave his only begotten Son.<note type="crossReference"><reference osisRef="Rom.5.8">Rom 5:8</reference></note></verse>
<verse osisID="John.3.17 John.3.18">For God sent not his Son into the world to condemn the world; he that believeth on him is not condemned.</verse>
<verse osisID="John.3.19"><q who="Jesus" marker="">And this is the condemnation, that light is come</q>, and men loved darkness.<foo>odd</foo></verse>
<verse osisID="John.99.1">bad</verse>
</chapter></div>
<div type="book" osisID="Mal"><chapter osisID="Mal.4">
<verse osisID="Mal.4.5">Behold, I will send you Elijah the prophet before the coming of the great and dreadful day of the LORD:</verse>
</chapter></div>
</osisText></osis>`

const usfmMal = `\id MAL World English Bible test
\h Malachi
\mt1 Malachi
\c 3
\s1 The day of Yahweh
\p
\v 23 Behold, I will send you Elijah the prophet before the great and terrible day of Yahweh comes.\f + \fr 3:23 \ft Some count this as 4:5.\f*
\v 24 \wj He will turn\wj* the hearts of the fathers to the children, \w and|strong="H5921"\w* the hearts of the children to their fathers.
`

const usfmJhn = `\id JHN
\c 3
\p
\v 16-17 For God so loved the world, that he gave his one and only Son. \zz odd\zz*
\q1
\v 18 He who believes in him is not judged.
`

const usxGen = `<?xml version="1.0" encoding="utf-8"?>
<usx version="3.0">
<book code="GEN" style="id">World English Bible test</book>
<para style="h">Genesis</para>
<chapter number="1" style="c" sid="GEN 1"/>
<para style="s1">Creation</para>
<para style="p"><verse number="1" style="v" sid="GEN 1:1"/>In the beginning, <char style="w" strong="H430">God</char> created the heavens and the earth.<note caller="+" style="f"><char style="fr">1:1 </char><char style="ft">Or, sky</char></note><verse eid="GEN 1:1"/></para>
<chapter eid="GEN 1"/>
</usx>`

type env struct {
	t   *testing.T
	r   *command.Registry
	dir string
	src string
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{t: t, r: command.NewRegistry(), dir: filepath.Join(t.TempDir(), "modules"), src: t.TempDir()}
	lib := NewLibrary(e.dir)
	t.Cleanup(lib.Close)
	RegisterLibrary(e.r, lib)
	return e
}

func (e *env) file(name, content string) string {
	p := filepath.Join(e.src, name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
	return p
}

func (e *env) run(id string, params any, out any) error {
	e.t.Helper()
	raw, _ := json.Marshal(params)
	res, err := e.r.Run(context.Background(), &command.Env{}, id, raw)
	if err != nil {
		return err
	}
	if out != nil {
		b, _ := json.Marshal(res)
		if err := json.Unmarshal(b, out); err != nil {
			e.t.Fatal(err)
		}
	}
	return nil
}

func (e *env) must(id string, params any, out any) {
	e.t.Helper()
	if err := e.run(id, params, out); err != nil {
		e.t.Fatalf("%s: %v", id, err)
	}
}

func (e *env) install(path string, extra map[string]any) InstallResult {
	e.t.Helper()
	p := map[string]any{"path": path, "licence_type": "public_domain"}
	for k, v := range extra {
		p[k] = v
	}
	var res InstallResult
	e.must("bible.install", p, &res)
	return res
}

// installBoth installs the KJV (English numbering) and a WEB sample (Hebrew numbering).
func (e *env) installBoth() {
	e.install(e.file("kjv.xml", osisKJV), map[string]any{"abbreviation": "KJV"})
	e.file("web/MAL.usfm", usfmMal)
	e.file("web/JHN.sfm", usfmJhn)
	e.install(filepath.Join(e.src, "web"), map[string]any{"id": "web", "name": "World English Bible",
		"language": "en", "versification": "hebrew"})
}

func (e *env) read(module, ref string) Passage {
	e.t.Helper()
	var p Passage
	e.must("bible.read", map[string]any{"module": module, "ref": ref, "markup": true}, &p)
	return p
}

func code(err error) string {
	var ce *command.Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func TestOSISImport(t *testing.T) {
	e := setup(t)
	res := e.install(e.file("kjv.xml", osisKJV), map[string]any{"abbreviation": "KJV"})
	m := res.Module
	if m.ID != "kjv" || m.Name != "King James Version (test)" || m.Language != "en" || m.VerseCount != 6 {
		t.Fatalf("meta = %+v", m)
	}
	if strings.Join(m.Books, " ") != "Gen Mal John" || !m.Licence.Confirmed || m.Versification != "english" {
		t.Fatalf("meta = %+v", m)
	}
	warnings := strings.Join(res.Warnings, "\n")
	if !strings.Contains(warnings, "<foo>") || !strings.Contains(warnings, "John.99.1") {
		t.Errorf("warnings = %q", warnings)
	}

	gen := e.read("kjv", "Gen 1:1").Verses
	if len(gen) != 1 || gen[0].Text != "In the beginning God created the heaven and the earth." {
		t.Fatalf("Gen 1:1 = %+v", gen)
	}
	for _, want := range []string{`<h>The Creation</h>`, `<w s="H7225">In the beginning</w>`,
		`<w s="H430" m="x:N">God</w>`, `<f>Hebrew: bara</f>`} {
		if !strings.Contains(gen[0].Markup, want) {
			t.Errorf("Gen 1:1 markup %q lacks %q", gen[0].Markup, want)
		}
	}
	if n := len(e.read("kjv", "Gen 1").Verses); n != 2 {
		t.Errorf("Gen 1 has %d verses", n)
	}
	if n := len(e.read("kjv", "John 3:16-19").Verses); n != 3 {
		t.Errorf("John 3:16-19 has %d rows", n)
	}

	v := e.read("kjv", "John 3:18").Verses
	if len(v) != 1 || v[0].ID != 43003017 || v[0].End != 43003018 || v[0].Ref != "John 3:17-18" {
		t.Errorf("combined verse = %+v", v)
	}
	v = e.read("kjv", "John 3:16").Verses
	if !strings.Contains(v[0].Markup, `<x><r o="Rom.5.8">Rom 5:8</r></x>`) || strings.Contains(v[0].Text, "Rom") {
		t.Errorf("cross-reference = %+v", v[0])
	}
	v = e.read("kjv", "John 3:19").Verses
	if !strings.HasPrefix(v[0].Markup, `<j>And this is the condemnation, that light is come</j>, and`) {
		t.Errorf("words of Christ = %q", v[0].Markup)
	}
	if !strings.HasSuffix(v[0].Text, "loved darkness.odd") {
		t.Errorf("unknown element text not kept: %q", v[0].Text)
	}
}

func TestUSFMImport(t *testing.T) {
	e := setup(t)
	e.file("web/MAL.usfm", usfmMal)
	e.file("web/JHN.sfm", usfmJhn)
	e.file("web/readme.txt", "not scripture")
	res := e.install(filepath.Join(e.src, "web"), map[string]any{"id": "web", "name": "World English Bible",
		"versification": "hebrew"})
	if res.Module.VerseCount != 4 || res.Module.Language != "und" {
		t.Fatalf("meta = %+v", res.Module)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), `\zz`) {
		t.Errorf("warnings = %q", res.Warnings)
	}

	v := e.read("web", "Mal 3:23").Verses[0]
	if !strings.HasPrefix(v.Markup, `<h>The day of Yahweh</h><p/>Behold`) {
		t.Errorf("heading = %q", v.Markup)
	}
	if !strings.Contains(v.Markup, `<f>Some count this as 4:5.</f>`) || strings.Contains(v.Text, "4:5") {
		t.Errorf("footnote = %+v", v)
	}
	v = e.read("web", "Mal 3:24").Verses[0]
	if !strings.Contains(v.Markup, `<j>He will turn</j>`) || !strings.Contains(v.Markup, `<w s="H5921">and</w>`) {
		t.Errorf("char styles = %q", v.Markup)
	}
	if v.Text != "He will turn the hearts of the fathers to the children, and the hearts of the children to their fathers." {
		t.Errorf("text = %q", v.Text)
	}
	john := e.read("web", "John 3").Verses
	if len(john) != 2 || john[0].End != 43003017 || john[1].Markup != "<p/>He who believes in him is not judged." {
		t.Errorf("John 3 = %+v", john)
	}
}

func TestUSXImport(t *testing.T) {
	e := setup(t)
	res := e.install(e.file("gen.usx", usxGen), map[string]any{"id": "webusx", "name": "WEB USX", "language": "en"})
	if len(res.Warnings) != 0 {
		t.Errorf("warnings = %q", res.Warnings)
	}
	v := e.read("webusx", "Gen 1:1").Verses[0]
	if v.Text != "In the beginning, God created the heavens and the earth." {
		t.Errorf("text = %q", v.Text)
	}
	if v.Markup != `<h>Creation</h><p/>In the beginning, <w s="H430">God</w> created the heavens and the earth.<f>Or, sky</f>` {
		t.Errorf("markup = %q", v.Markup)
	}
}

func TestParallelAcrossNumberings(t *testing.T) {
	e := setup(t)
	e.installBoth()
	var res ParallelResult
	e.must("bible.parallel", map[string]any{"modules": []string{"kjv", "web"}, "ref": "Mal 4:5"}, &res)
	if len(res.Columns) != 2 || res.Columns[1].Ref != "Malachi 3:23" {
		t.Fatalf("parallel = %+v", res)
	}
	for _, c := range res.Columns {
		if len(c.Verses) != 1 || c.Verses[0].BaseID != 39004005 || !strings.Contains(c.Verses[0].Text, "Elijah") {
			t.Errorf("%s column = %+v", c.Module, c.Verses)
		}
	}
	e.must("bible.parallel", map[string]any{"modules": []string{"web", "kjv"}, "ref": "Mal 3:23",
		"versification": "hebrew"}, &res)
	if res.Columns[1].Ref != "Malachi 4:5" || len(res.Columns[1].Verses) != 1 {
		t.Errorf("hebrew base = %+v", res)
	}
	if err := e.run("bible.parallel", map[string]any{"modules": []string{"kjv", "nope"}, "ref": "Mal 4:5"}, nil); code(err) != "not_found" {
		t.Errorf("missing module: %v", err)
	}
}

func TestMappingTable(t *testing.T) {
	e := setup(t)
	mapping := e.file("map.tsv", "# test table\nenglish\tMal.3.23\tMal.4.5\n")
	e.file("custom/MAL.usfm", usfmMal)
	e.install(filepath.Join(e.src, "custom"), map[string]any{"id": "custom", "name": "Custom",
		"versification": "x-custom", "mapping_file": mapping})
	var res ParallelResult
	e.must("bible.parallel", map[string]any{"modules": []string{"custom"}, "ref": "Mal 4:5"}, &res)
	v := res.Columns[0].Verses
	if len(v) != 1 || v[0].ID != 39003023 || v[0].BaseID != 39004005 {
		t.Errorf("mapped = %+v", res)
	}
}

func TestSearch(t *testing.T) {
	e := setup(t)
	e.installBoth()
	var hits []Hit
	e.must("bible.search", map[string]any{"query": "world"}, &hits)
	if len(hits) != 3 {
		t.Fatalf("hits = %+v", hits)
	}
	e.must("bible.search", map[string]any{"query": "world", "modules": []string{"kjv"}}, &hits)
	if len(hits) != 2 || hits[0].ID != 43003017 || hits[0].End != 43003018 {
		t.Fatalf("hits = %+v", hits)
	}
	if !strings.Contains(hits[0].Snippet, "<mark>world</mark>") || hits[0].Score <= hits[1].Score {
		t.Errorf("ranking = %+v", hits)
	}
	e.must("bible.search", map[string]any{"query": "elijah prophet", "ref": "Mal 4"}, &hits)
	if len(hits) != 2 || hits[0].Ref == hits[1].Ref {
		t.Errorf("filtered = %+v", hits)
	}
	e.must("bible.search", map[string]any{"query": "Elijah", "modules": []string{"web"}, "ref": "John"}, &hits)
	if len(hits) != 0 {
		t.Errorf("book filter = %+v", hits)
	}
	e.must("bible.search", map[string]any{"query": "loved the world", "mode": "phrase"}, &hits)
	if len(hits) != 2 {
		t.Errorf("phrase = %+v", hits)
	}
	e.must("bible.search", map[string]any{"query": "darkness judged", "mode": "any", "limit": 1}, &hits)
	if len(hits) != 1 {
		t.Errorf("limit = %+v", hits)
	}
	e.must("bible.search", map[string]any{"strong": "H0430"}, &hits)
	if len(hits) != 1 || hits[0].Ref != "Genesis 1:1" {
		t.Errorf("strong = %+v", hits)
	}
	if err := e.run("bible.search", map[string]any{"query": "(", "mode": "fts"}, nil); code(err) != "bad_params" {
		t.Errorf("bad fts: %v", err)
	}
	if err := e.run("bible.search", map[string]any{}, nil); code(err) != "bad_params" {
		t.Errorf("empty search: %v", err)
	}
}

func TestModulesInstallRemove(t *testing.T) {
	e := setup(t)
	var list []Meta
	e.must("bible.modules", struct{}{}, &list)
	if len(list) != 0 {
		t.Fatalf("empty library lists %+v", list)
	}
	e.installBoth()
	e.must("bible.modules", struct{}{}, &list)
	if len(list) != 2 || list[0].ID != "kjv" || list[1].Versification != "hebrew" {
		t.Fatalf("list = %+v", list)
	}
	kjv := filepath.Join(e.src, "kjv.xml")
	if err := e.run("bible.install", map[string]any{"path": kjv, "licence_type": "public_domain", "id": "kjv"}, nil); code(err) != "failed" {
		t.Errorf("duplicate install: %v", err)
	}
	e.install(kjv, map[string]any{"id": "kjv", "replace": true})
	if err := e.run("bible.install", map[string]any{"path": kjv, "licence_type": "creative_commons", "spdx": "MIT", "id": "x"}, nil); code(err) != "bad_params" {
		t.Errorf("install with a bad licence: %v", err)
	}
	e.read("web", "Mal 3:23")
	e.must("bible.remove", map[string]any{"module": "web"}, nil)
	if _, err := os.Stat(filepath.Join(e.dir, "web.db")); !os.IsNotExist(err) {
		t.Errorf("module file still there: %v", err)
	}
	if err := e.run("bible.read", map[string]any{"module": "web", "ref": "Mal 3:23"}, nil); code(err) != "not_found" {
		t.Errorf("read removed module: %v", err)
	}
	for _, d := range e.r.List() {
		if d.Writes {
			t.Errorf("%s is declared as writing library.db", d.ID)
		}
	}
}

func TestOddMarkup(t *testing.T) {
	e := setup(t)
	bad := e.file("bad.xml", `<osis><osisText><div type="book" osisID="Gen"><verse osisID="Gen.1.1">x</div>`)
	if err := e.run("bible.install", map[string]any{"path": bad, "licence_type": "unknown", "name": "Bad"}, nil); err == nil {
		t.Error("broken XML should fail")
	}
	odd := e.file("odd.usfm", "stray text\n\\id XYZ\n\\c 1\n\\v 1 lost\n\\id GEN\n\\v 1 early\n\\c 1\n\\v 1 In \\nd the\\nd* beginning \\qt-s |who=\"x\"\\*said\\qt-e\\*\\fig a|src=\"x.png\"\\fig* end\n\\v 0 title\n")
	var res InstallResult
	e.must("bible.install", map[string]any{"path": odd, "licence_type": "unknown", "name": "Odd"}, &res)
	w := strings.Join(res.Warnings, "\n")
	if !strings.Contains(w, `"XYZ"`) || !strings.Contains(w, "before any chapter") {
		t.Errorf("warnings = %q", w)
	}
	v := e.read("odd", "Gen 1:1").Verses
	if len(v) != 1 || v[0].Text != "In the beginning said end" {
		t.Errorf("odd verse = %+v", v)
	}
}
