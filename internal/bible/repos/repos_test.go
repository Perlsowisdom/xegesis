package repos

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/bible/sword"
	"github.com/Perlsowisdom/xegesis/internal/bible/sword/swordtest"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/licence"
)

// The test server stands in for CrossWire and eBible.org. Catalogue fixtures in testdata were
// recorded from the live sources; the module files are built here.

type fake struct {
	srv   *httptest.Server
	files map[string][]byte
	mu    sync.Mutex
	hits  map[string]int
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.hits[r.URL.Path]++
	data, ok := f.files[r.URL.Path]
	f.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", itoa(len(data)))
		return
	}
	w.Write(data)
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func (f *fake) hit(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for p, c := range f.hits {
		if strings.HasPrefix(p, prefix) {
			n += c
		}
	}
	return n
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

const usfmJohn = "\\id JHN\n\\c 3\n\\p\n\\v 16 For God so loved the world.\n"

var tinyVerses = map[swordtest.Key]string{
	swordtest.K(43, 3, 16): `For God so <w lemma="strong:G25">loved</w> the world.`,
	swordtest.K(1, 1, 1):   `In the beginning God created the heaven and the earth.`,
}

func newFake(t *testing.T, tinyVersion string) *fake {
	t.Helper()
	f := &fake{files: map[string][]byte{}, hits: map[string]int{}}
	f.srv = httptest.NewTLSServer(f)
	t.Cleanup(f.srv.Close)
	host := strings.TrimPrefix(f.srv.URL, "https://")

	moddir := t.TempDir()
	if err := swordtest.ZText(moddir, tinyVerses); err != nil {
		t.Fatal(err)
	}
	size := 0
	for _, n := range []string{"ot.bzs", "ot.bzv", "ot.bzz", "nt.bzs", "nt.bzv", "nt.bzz"} {
		data, _ := os.ReadFile(filepath.Join(moddir, n))
		f.files["/sword/raw/modules/texts/Tiny/"+n] = data
		size += len(data)
	}
	tiny := swordtest.Conf("Tiny", "zText", "InstallSize="+itoa(size)+"\n")
	tiny = strings.Replace(tiny, "Version=1.0", "Version="+tinyVersion, 1)
	f.files["/sword/raw/mods.d.tar.gz"] = tarGz(t, map[string]string{
		"mods.d/asv.conf":    string(readFixture(t, "../../sword/testdata/asv.conf")),
		"mods.d/tiny.conf":   tiny,
		"mods.d/locked.conf": swordtest.Conf("Locked", "zText", "CipherKey=\nDistributionLicense=Copyrighted; Permission to distribute granted to CrossWire\n"),
		"mods.d/nrsv.conf":   strings.Replace(swordtest.Conf("Other", "zText", ""), "Versification=KJV", "Versification=LXX", 1),
		"mods.d/unclear.conf": strings.Replace(swordtest.Conf("Unclear", "zText", ""),
			"DistributionLicense=Public Domain", "DistributionLicense=See README file", 1),
		"mods.d/dict.conf":    strings.Replace(swordtest.Conf("Dict", "zLD", "Feature=GreekDef\n"), "DataPath=./modules/texts/Dict/", "DataPath=./modules/lexdict/zld/dict/dict", 1),
		"mods.d/tinycom.conf": strings.Replace(swordtest.Conf("TinyCom", "zCom", ""), "/texts/TinyCom/", "/comments/zcom/tinycom/", 1),
	})
	addReferenceFiles(t, f)
	f.files["/master.conf"] = []byte("[Repos]\n" +
		"1=HTTPSPackagePreference=CrossWire|" + host + "|/sword/raw\n" +
		"2=FTPSource=CrossWire|ftp.example.org|/pub/sword/raw\n" +
		"3=HTTPSPackagePreference=Test Beta|" + host + "|/beta\n")
	f.files["/ebible/Scriptures/translations.csv"] = readFixture(t, "translations.csv")
	f.files["/ebible/Scriptures/engwebp_usfm.zip"] = zipOf(t, map[string]string{"44-JHNengwebp.usfm": usfmJohn, "copr.htm": "public domain"})
	f.files["/ebible/Scriptures/aai_usfm.zip"] = zipOf(t, map[string]string{"44-JHNaai.usfm": usfmJohn})
	f.files["/ebible/aai/copr.htm"] = []byte(`<p>© 2009 Wycliffe</p><a href="http://creativecommons.org/licenses/by-nc-nd/4.0/">CC</a>`)
	return f
}

type env struct {
	t   *testing.T
	f   *fake
	reg *command.Registry
	lib *bible.Library
	dir string
}

func setup(t *testing.T) *env {
	t.Helper()
	tmp := t.TempDir()
	for _, k := range []string{"TMP", "TEMP", "TMPDIR"} {
		t.Setenv(k, tmp)
	}
	f := newFake(t, "1.0")
	e := &env{t: t, f: f, reg: command.NewRegistry(), dir: t.TempDir()}
	e.lib = bible.NewLibrary(filepath.Join(e.dir, "modules"))
	t.Cleanup(e.lib.Close)
	bible.RegisterLibrary(e.reg, e.lib)
	Register(e.reg, filepath.Join(e.dir, "catalogues"), e.lib, WithHTTPClient(f.srv.Client()),
		WithURLs(f.srv.URL+"/master.conf", f.srv.URL+"/sword/raw", f.srv.URL+"/ebible"),
		WithDataURLs(f.srv.URL+"/step", f.srv.URL+"/ob"))
	return e
}

func (e *env) run(id string, params any, out any) error {
	e.t.Helper()
	raw, _ := json.Marshal(params)
	res, err := e.reg.Run(context.Background(), &command.Env{}, id, raw)
	if err != nil {
		return err
	}
	if out != nil {
		data, _ := json.Marshal(res)
		if err := json.Unmarshal(data, out); err != nil {
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

func code(err error) string {
	var ce *command.Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func (e *env) enable() {
	e.t.Helper()
	var res SourcesResult
	e.must("bible.sources", map[string]any{"enable": true, "refresh": true}, &res)
	if !res.Enabled {
		e.t.Fatal("not enabled")
	}
}

func (e *env) item(source, id string) Item {
	e.t.Helper()
	for _, kind := range []string{"bible", "commentary", "dictionary"} {
		var res AvailableResult
		e.must("bible.available", map[string]any{"source": source, "text": id, "limit": 1000, "kind": kind}, &res)
		for _, it := range res.Items {
			if it.ID == id {
				return it
			}
		}
	}
	e.t.Fatalf("%s/%s not listed", source, id)
	return Item{}
}

func TestOffUntilEnabled(t *testing.T) {
	e := setup(t)
	var res AvailableResult
	e.must("bible.available", map[string]any{}, &res)
	if res.Enabled || res.Cached || len(res.Items) != 0 {
		t.Fatalf("fresh catalogue: %+v", res)
	}
	if err := e.run("bible.sources", map[string]any{"refresh": true}, nil); code(err) != "unavailable" {
		t.Fatalf("refresh while off: %v", err)
	}
	if n := e.f.hit("/"); n != 0 {
		t.Fatalf("%d requests while off", n)
	}
}

func TestRefreshAndList(t *testing.T) {
	e := setup(t)
	e.enable()
	var src SourcesResult
	e.must("bible.sources", map[string]any{}, &src)
	ids := map[string]Source{}
	for _, s := range src.Sources {
		ids[s.ID] = s
	}
	if len(src.Sources) != 5 || ids["stepbible"].Items != 4 || ids["crosswire"].Items != 7 || ids["ebible"].Items != 3 || ids["test-beta"].Error == "" {
		t.Fatalf("sources: %+v", src.Sources)
	}

	var res AvailableResult
	e.must("bible.available", map[string]any{"language": "en"}, &res)
	if res.Total != 7 { // 5 SWORD Bibles (not the dictionary) and 2 English eBible.org translations
		t.Errorf("English items: %d, want 7", res.Total)
	}
	if asv := e.item("crosswire", "ASV"); !asv.Supported || asv.Licence.Type != licence.PublicDomain || !asv.LicenceClear || asv.Size != 3491824 {
		t.Errorf("ASV: %+v", asv)
	}
	if l := e.item("crosswire", "Locked"); !l.Locked || l.Supported {
		t.Errorf("locked module: %+v", l)
	}
	if o := e.item("crosswire", "Other"); o.Supported || !strings.Contains(o.Reason, "LXX") {
		t.Errorf("LXX module: %+v", o)
	}
	if a := e.item("ebible", "aai"); a.Language != "aai" || a.LicenceClear || !strings.Contains(a.Rights, "Wyclyffe") {
		t.Errorf("aai: %+v", a)
	}
	e.must("bible.available", map[string]any{"licence": "public_domain", "language": "en"}, &res)
	for _, it := range res.Items {
		if it.Licence.Type != licence.PublicDomain || !strings.HasPrefix(it.Language, "en") {
			t.Errorf("filter let through %s/%s", it.Source, it.ID)
		}
	}
	e.must("bible.available", map[string]any{"kind": "dictionary", "source": "crosswire"}, &res)
	if res.Total != 1 || res.Items[0].ID != "Dict" {
		t.Errorf("dictionaries: %+v", res.Items)
	}
	found := false
	for _, l := range res.Languages {
		found = found || l.Code == "en" && l.Name == "English"
	}
	if !found {
		t.Errorf("languages: %+v", res.Languages)
	}
}

func TestGetSWORD(t *testing.T) {
	e := setup(t)
	e.enable()
	var pre GetResult
	e.must("bible.get", map[string]any{"source": "crosswire", "item": "Tiny", "preview": true}, &pre)
	if pre.Module != nil || pre.Size == 0 || e.f.hit("/sword/raw/modules") != 0 {
		t.Fatalf("preview downloaded or installed: %+v", pre)
	}
	var res GetResult
	e.must("bible.get", map[string]any{"source": "crosswire", "item": "Tiny", "accept_licence": true}, &res)
	if res.Module == nil || res.Module.ID != "tiny" || res.Module.VerseCount != 2 || !res.Module.Licence.Confirmed {
		t.Fatalf("installed: %+v", res.Module)
	}
	var p bible.Passage
	e.must("bible.read", map[string]any{"module": "tiny", "ref": "John 3:16", "markup": true}, &p)
	if len(p.Verses) != 1 || p.Verses[0].Text != "For God so loved the world." {
		t.Fatalf("John 3:16: %+v", p)
	}
	if it := e.item("crosswire", "Tiny"); !it.Installed || it.Module != "tiny" || it.Update {
		t.Errorf("not marked installed: %+v", it)
	}
	var prog []Progress
	e.must("bible.progress", struct{}{}, &prog)
	if len(prog) != 0 {
		t.Errorf("progress left over: %+v", prog)
	}
	if left, _ := filepath.Glob(filepath.Join(os.Getenv("TMP"), "xegesis-*")); len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}
	if err := e.run("bible.get", map[string]any{"source": "crosswire", "item": "Tiny"}, nil); code(err) != "bad_params" {
		t.Errorf("second install: %v", err)
	}

	// A newer version in the catalogue is an update.
	newer := newFake(t, "1.1")
	e.f.mu.Lock()
	e.f.files["/sword/raw/mods.d.tar.gz"] = newer.files["/sword/raw/mods.d.tar.gz"]
	e.f.mu.Unlock()
	e.must("bible.sources", map[string]any{"refresh": true, "source": "crosswire"}, nil)
	var ups []Update
	e.must("bible.updates", struct{}{}, &ups)
	if len(ups) != 1 || ups[0].Module != "tiny" || ups[0].InstalledVersion != "1.0" || ups[0].Version != "1.1" {
		t.Fatalf("updates: %+v", ups)
	}
	e.must("bible.get", map[string]any{"source": "crosswire", "item": "Tiny", "replace": true}, &res)
	if e.must("bible.updates", struct{}{}, &ups); len(ups) != 0 {
		t.Errorf("update not applied: %+v", ups)
	}
}

func TestLockedAndUnclear(t *testing.T) {
	e := setup(t)
	e.enable()
	err := e.run("bible.get", map[string]any{"source": "crosswire", "item": "Locked", "licence_type": "personal_purchase"}, nil)
	if code(err) != "unavailable" || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("locked module: %v", err)
	}
	if err := e.run("bible.get", map[string]any{"source": "crosswire", "item": "Other"}, nil); code(err) != "unavailable" {
		t.Fatalf("unsupported module: %v", err)
	}
	if n := e.f.hit("/sword/raw/modules"); n != 0 {
		t.Fatalf("%d module downloads for refused items", n)
	}
	// Unclear licence: the user must state one. (The module files are missing, so the install
	// itself then fails; the licence check comes first.)
	err = e.run("bible.get", map[string]any{"source": "crosswire", "item": "Unclear"}, nil)
	if code(err) != "bad_params" || !strings.Contains(err.Error(), "See README file") {
		t.Fatalf("unclear licence: %v", err)
	}
}

func TestGetEBible(t *testing.T) {
	e := setup(t)
	e.enable()
	var res GetResult
	e.must("bible.get", map[string]any{"source": "ebible", "item": "engwebp"}, &res)
	if res.Module == nil || res.Module.ID != "engwebp" || res.Module.Licence.Type != licence.PublicDomain || res.Module.Licence.Confirmed {
		t.Fatalf("engwebp: %+v", res.Module)
	}
	// aai's licence comes from its copyright page.
	var pre GetResult
	e.must("bible.get", map[string]any{"source": "ebible", "item": "aai", "preview": true}, &pre)
	if !pre.Item.LicenceClear || pre.Item.Licence.SPDX != "CC-BY-NC-ND-4.0" || pre.Item.Licence.Commercial || pre.Size == 0 {
		t.Fatalf("aai preview: %+v size %d", pre.Item.Licence, pre.Size)
	}
	if !strings.Contains(pre.Item.Rights, "creativecommons.org/licenses/by-nc-nd/4.0") {
		t.Errorf("rights: %q", pre.Item.Rights)
	}
}

func TestOfflineCache(t *testing.T) {
	e := setup(t)
	e.enable()
	e.f.srv.Close()
	var res AvailableResult
	e.must("bible.available", map[string]any{"text": "Tiny"}, &res)
	if res.Total != 1 || !res.Cached {
		t.Fatalf("offline listing: %+v", res)
	}
	var src SourcesResult
	e.must("bible.sources", map[string]any{"refresh": true}, &src)
	for _, s := range src.Sources {
		if s.ID == "crosswire" && (s.Error == "" || s.Items != 7) {
			t.Errorf("failed refresh lost the cache: %+v", s)
		}
	}
	e.must("bible.available", map[string]any{"text": "Tiny"}, &res)
	if res.Total != 1 {
		t.Fatalf("cache gone after a failed refresh: %+v", res)
	}
	if err := e.run("bible.get", map[string]any{"source": "crosswire", "item": "Tiny"}, nil); code(err) != "failed" {
		t.Fatalf("offline install: %v", err)
	}
	// Turning the catalogues off keeps the cache but stops downloads.
	e.must("bible.sources", map[string]any{"enable": false}, nil)
	if err := e.run("bible.get", map[string]any{"source": "crosswire", "item": "Tiny"}, nil); code(err) != "unavailable" {
		t.Fatalf("install while off: %v", err)
	}
}

func TestLicenceMapping(t *testing.T) {
	for dl, want := range map[string]struct {
		typ   licence.Type
		spdx  string
		clear bool
		share bool
	}{
		"Public Domain":                     {licence.PublicDomain, "", true, true},
		"Creative Commons: BY-SA 4.0":       {licence.CreativeCommons, "CC-BY-SA-4.0", true, true},
		"Creative Commons: BY-NC-ND 4.0":    {licence.CreativeCommons, "CC-BY-NC-ND-4.0", true, true},
		"Creative Commons: CC0":             {licence.CreativeCommons, "CC0-1.0", true, true},
		"Creative Commons: by-nd 3.0":       {licence.CreativeCommons, "CC-BY-ND-3.0", true, true},
		"Creative Commons: by-sa":           {licence.CreativeCommons, "", false, true},
		"GPL":                               {licence.OpenOther, "", true, true},
		"Copyrighted; Freely distributable": {licence.OpenOther, "", true, true},
		"Copyrighted; Permission to distribute granted to CrossWire": {licence.OpenOther, "", true, false},
		"Copyrighted":     {licence.OpenOther, "", false, false},
		"See README file": {licence.OpenOther, "", false, false},
		"":                {licence.OpenOther, "", false, false},
	} {
		c, err := sword.ParseConf(strings.NewReader("[M]\nDistributionLicense=" + dl + "\nCopyright=© 2001 Someone\n"))
		if err != nil {
			t.Fatal(err)
		}
		m := swordLicence(c)
		if m.Licence.Type != want.typ || m.Licence.SPDX != want.spdx || m.Clear != want.clear || m.Licence.Share != want.share {
			t.Errorf("%q: %+v clear=%v", dl, m.Licence, m.Clear)
		}
		if !strings.Contains(m.Rights, dl) || (want.typ != licence.PublicDomain && m.Licence.Attribution != "© 2001 Someone") {
			t.Errorf("%q: rights %q attribution %q", dl, m.Rights, m.Licence.Attribution)
		}
	}
	if l, _, ok := ccFromPage(`<a href="https://creativecommons.org/licenses/by-sa/4.0/">`); !ok || l.SPDX != "CC-BY-SA-4.0" {
		t.Errorf("copyright page: %+v", l)
	}
	if _, _, ok := ccFromPage("All rights reserved."); ok {
		t.Error("found a licence on a page without one")
	}
}

func TestMasterListAndVersions(t *testing.T) {
	got := parseMasterList(bytes.NewReader(readFixture(t, "masterRepoList.conf")))
	if len(got) == 0 || got[0].ID != "crosswire" || got[0].URL != "https://crosswire.org/ftpmirror/pub/sword/raw" {
		t.Fatalf("master list: %+v", got)
	}
	for _, s := range got {
		if !strings.HasPrefix(s.URL, "https://") {
			t.Errorf("non-HTTPS source %+v", s)
		}
	}
	for _, c := range [][3]string{{"1.10", "1.9", "y"}, {"2.0", "2.0", ""}, {"2026-10-20", "2026-09-01", "y"}, {"1.0", "1.0.1", ""}} {
		if newer(c[0], c[1]) != (c[2] == "y") {
			t.Errorf("newer(%s, %s) = %v", c[0], c[1], !(c[2] == "y"))
		}
	}
}
