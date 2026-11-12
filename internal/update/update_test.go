//go:build !store

package update

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/update/updatesig"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"1.2.3", "1.2.2", true},
		{"1.2.3", "1.2.3", false},
		{"1.2.2", "1.2.3", false},
		{"1.10.0", "1.9.9", true},
		{"2.0.0", "1.99.99", true},
		{"v1.0.1", "1.0.0", true},
		{"1.0.0", "1.0.0-beta.1", true},
		{"1.0.0-beta.1", "1.0.0", false},
		{"1.0.0-beta.2", "1.0.0-beta.1", true},
		{"0.0.1", "0.0.0-dev+abc1234", true},
		{"1.0.0+build5", "1.0.0", false},
		{"junk", "1.0.0", false},
		{"1.0.0", "junk", true},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestParseManifest(t *testing.T) {
	h := strings.Repeat("ab", 32)
	good := `{"version":"1.2.3","url":"https://github.com/x/y/releases/download/v1.2.3/setup.exe","sha256":"` + strings.ToUpper(h) + `","signature":"c2ln","notes":"Fixes"}`
	m, err := ParseManifest([]byte(good), false)
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "1.2.3" || m.SHA256 != h || m.Notes != "Fixes" {
		t.Fatalf("manifest = %+v", m)
	}
	for name, bad := range map[string]string{
		"not json":   `nope`,
		"no version": `{"url":"https://a/b","sha256":"` + h + `","signature":"c2ln"}`,
		"http url":   `{"version":"1.0.0","url":"http://a/b","sha256":"` + h + `","signature":"c2ln"}`,
		"no sig":     `{"version":"1.0.0","url":"https://a/b","sha256":"` + h + `"}`,
		"bad hash":   `{"version":"1.0.0","url":"https://a/b","sha256":"12","signature":"c2ln"}`,
	} {
		if _, err := ParseManifest([]byte(bad), false); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := ParseManifest([]byte(strings.Replace(good, "https://github.com", "http://127.0.0.1", 1)), true); err != nil {
		t.Errorf("http allowed for tests: %v", err)
	}
}

// release is a fake release server: latest.json and the installer.
type release struct {
	srv      *httptest.Server
	hits     atomic.Int32
	manifest Manifest
	body     []byte
}

func newRelease(t *testing.T, version string, priv string, body []byte, tamper func(*Manifest)) *release {
	t.Helper()
	r := &release{body: body}
	h, _ := updatesig.Sum(strings.NewReader(string(body)))
	sig, err := updatesig.Sign(priv, version, h)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	r.srv = httptest.NewServer(mux)
	t.Cleanup(r.srv.Close)
	r.manifest = Manifest{Version: version, URL: r.srv.URL + "/Xegesis-" + version + "-setup.exe", SHA256: h, Signature: sig}
	if tamper != nil {
		tamper(&r.manifest)
	}
	mux.HandleFunc("/latest.json", func(w http.ResponseWriter, _ *http.Request) {
		r.hits.Add(1)
		json.NewEncoder(w).Encode(r.manifest)
	})
	mux.HandleFunc("/Xegesis-"+version+"-setup.exe", func(w http.ResponseWriter, _ *http.Request) {
		r.hits.Add(1)
		w.Write(r.body)
	})
	return r
}

func testConfig(t *testing.T, r *release, pubB64 string, launched *[]string) Config {
	pub, err := updatesig.ParsePublicKey(pubB64)
	if err != nil {
		t.Fatal(err)
	}
	return Config{
		Endpoint: r.srv.URL + "/latest.json", PubKey: pub, Current: "1.0.0", HTTP: r.srv.Client(),
		AllowHTTP: true, TempDir: t.TempDir(),
		Launch: func(p string) error { *launched = append(*launched, p); return nil },
	}
}

func TestDownloadVerifies(t *testing.T) {
	pubB64, priv, _ := updatesig.GenerateKey()
	_, otherPriv, _ := updatesig.GenerateKey()
	body := []byte("pretend installer")
	ctx := context.Background()

	t.Run("good", func(t *testing.T) {
		r := newRelease(t, "1.1.0", priv, body, nil)
		var launched []string
		cfg := testConfig(t, r, pubB64, &launched)
		m, err := cfg.Check(ctx)
		if err != nil {
			t.Fatal(err)
		}
		file, err := cfg.Download(ctx, m)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(file)
		if string(got) != string(body) || filepath.Base(file) != "Xegesis-1.1.0-setup.exe" {
			t.Fatalf("file %s holds %q", file, got)
		}
	})
	for name, c := range map[string]struct {
		priv   string
		tamper func(*Manifest)
		want   string
	}{
		"wrong hash": {priv, func(m *Manifest) {
			m.SHA256 = strings.Repeat("0", 64)
			m.Signature, _ = updatesig.Sign(priv, m.Version, m.SHA256) // validly signed, but not this file
		}, "does not match its SHA-256"},
		"hash changed after signing": {priv, func(m *Manifest) { m.SHA256 = strings.Repeat("0", 64) }, "signature"},
		"wrong signature":            {priv, func(m *Manifest) { m.Signature = "AAAA" + m.Signature[4:] }, "signature"},
		"signed by another key":      {otherPriv, nil, "signature"},
		"version changed":            {priv, func(m *Manifest) { m.Version = "9.9.9" }, "signature"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRelease(t, "1.1.0", c.priv, body, c.tamper)
			var launched []string
			cfg := testConfig(t, r, pubB64, &launched)
			m, err := cfg.Check(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = cfg.Download(ctx, m)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want one containing %q", err, c.want)
			}
			if left, _ := filepath.Glob(filepath.Join(cfg.TempDir, "*")); len(left) != 0 {
				t.Errorf("rejected download left files behind: %v", left)
			}
		})
	}
}

func TestDownloadNeedsKey(t *testing.T) {
	_, priv, _ := updatesig.GenerateKey()
	r := newRelease(t, "1.1.0", priv, []byte("x"), nil)
	cfg := Config{HTTP: r.srv.Client(), TempDir: t.TempDir()}
	if _, err := cfg.Download(context.Background(), r.manifest); err == nil {
		t.Fatal("download without a key accepted")
	}
	if r.hits.Load() != 0 {
		t.Fatal("something was fetched")
	}
	if k, err := updatesig.ParsePublicKey(pubkeyText); err != nil || len(k) != ed25519.PublicKeySize {
		t.Fatalf("embedded pubkey.txt is not a key: %v", err)
	}
}

func openEnv(t *testing.T) *command.Env {
	s, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return &command.Env{Store: s}
}

func run(t *testing.T, reg *command.Registry, env *command.Env, id, params string) (any, error) {
	t.Helper()
	return reg.Run(context.Background(), env, id, json.RawMessage(params))
}

func TestOffByDefaultAndOnlyChecksWhenOn(t *testing.T) {
	pubB64, priv, _ := updatesig.GenerateKey()
	r := newRelease(t, "1.1.0", priv, []byte("installer"), nil)
	var launched []string
	cfg := testConfig(t, r, pubB64, &launched)
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	cfg.Now = func() time.Time { return now }
	reg := command.NewRegistry()
	RegisterWith(reg, cfg)
	env := openEnv(t)

	res, err := run(t, reg, env, "update.settings.get", "")
	if err != nil {
		t.Fatal(err)
	}
	if si := res.(SettingsInfo); si.Enabled || si.Asked || si.Current != "1.0.0" {
		t.Fatalf("default settings = %+v", si)
	}
	// Off: nothing may touch the network, however it is asked.
	for _, id := range []string{"update.check", "update.install"} {
		if _, err := run(t, reg, env, id, ""); err == nil {
			t.Errorf("%s ran while updates were off", id)
		}
	}
	if res, err := run(t, reg, env, "update.auto", ""); err != nil || res.(CheckResult).Ran {
		t.Fatalf("update.auto while off: %v %+v", err, res)
	}
	if r.hits.Load() != 0 || len(launched) != 0 {
		t.Fatalf("the server was hit %d times and %d installers launched while off", r.hits.Load(), len(launched))
	}
	// Dismissing the question does not turn updates on.
	if res, _ := run(t, reg, env, "update.settings.set", `{"asked":true}`); res.(SettingsInfo).Enabled || !res.(SettingsInfo).Asked {
		t.Fatalf("after dismissing: %+v", res)
	}

	if _, err := run(t, reg, env, "update.settings.set", `{"enabled":true}`); err != nil {
		t.Fatal(err)
	}
	res, err = run(t, reg, env, "update.check", "")
	if err != nil {
		t.Fatal(err)
	}
	if cr := res.(CheckResult); !cr.Ran || !cr.Newer || cr.Latest != "1.1.0" {
		t.Fatalf("check = %+v", cr)
	}
	// Auto: a check within a day is not repeated; after a day it is.
	hits := r.hits.Load()
	if _, err := run(t, reg, env, "update.auto", ""); err != nil || r.hits.Load() != hits {
		t.Fatalf("auto repeated a recent check (%v)", err)
	}
	now = now.Add(21 * time.Hour)
	if _, err := run(t, reg, env, "update.auto", ""); err != nil || r.hits.Load() != hits+1 {
		t.Fatalf("auto did not check after a day (%v)", err)
	}
	si, _ := run(t, reg, env, "update.settings.get", "")
	if si.(SettingsInfo).LastChecked == "" || si.(SettingsInfo).Latest != "1.1.0" {
		t.Fatalf("settings after checks: %+v", si)
	}

	res, err = run(t, reg, env, "update.install", "")
	if err != nil {
		t.Fatal(err)
	}
	if ir := res.(InstallResult); !ir.Launched || ir.Version != "1.1.0" || len(launched) != 1 {
		t.Fatalf("install = %+v, launched %v", ir, launched)
	}
	if got, _ := os.ReadFile(launched[0]); string(got) != "installer" {
		t.Fatalf("launched file holds %q", got)
	}
}

func TestInstallRefusesBadSignatureAndOldVersions(t *testing.T) {
	pubB64, priv, _ := updatesig.GenerateKey()
	_, otherPriv, _ := updatesig.GenerateKey()
	for name, c := range map[string]struct{ version, priv string }{
		"signed by another key": {"1.1.0", otherPriv},
		"not newer":             {"1.0.0", priv},
	} {
		t.Run(name, func(t *testing.T) {
			env := openEnv(t)
			r := newRelease(t, c.version, c.priv, []byte("x"), nil)
			var launched []string
			cfg := testConfig(t, r, pubB64, &launched)
			reg := command.NewRegistry()
			RegisterWith(reg, cfg)
			if _, err := run(t, reg, env, "update.settings.set", `{"enabled":true}`); err != nil {
				t.Fatal(err)
			}
			if _, err := run(t, reg, env, "update.install", ""); err == nil {
				t.Fatal("install succeeded")
			}
			if len(launched) != 0 {
				t.Fatal("an installer was launched")
			}
		})
	}
}
