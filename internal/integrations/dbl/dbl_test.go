package dbl

import (
	"bytes"
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
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/integrations/secrets"
)

type server struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []*http.Request
}

func (s *server) last() *http.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reqs[len(s.reqs)-1]
}

func newServer(t *testing.T) *server {
	t.Helper()
	s := &server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.reqs = append(s.reqs, r)
		s.mu.Unlock()
		auth := r.Header.Get("X-DBL-Authorization")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/public_entries_list"), strings.HasPrefix(r.URL.Path, "/api/open_access_entries_list"):
			w.Write([]byte(`{"entries":[{"id":"e1","name":"Example Bible","type":"text","language_name":"English","language_code":"eng","rights_holder":"Example Society","latest":true}]}`))
		case auth == "":
			w.WriteHeader(401)
		case strings.HasPrefix(r.URL.Path, "/api/owned_entries_list"):
			w.Write([]byte(`[{"id":"e2","name":"Owned","type":"text"}]`))
		case r.URL.Path == "/api/entries/e1":
			w.Write([]byte(`{"licenses":[{"id":"7"}],"revisions":[{"revision":1}]}`))
		case r.URL.Path == "/api/entries/e1/revision/latest/license/7.zip":
			w.Write([]byte("PK-fake-zip"))
		case r.URL.Path == "/api/entries/e1/revision/latest/license/big.zip":
			w.Write(bytes.Repeat([]byte("x"), 100))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *server) client(c Credentials) *Client {
	cl := New(c)
	cl.BaseURL = s.URL + "/api"
	return cl
}

func TestAnonymousListsSendNoAuth(t *testing.T) {
	s := newServer(t)
	for _, k := range []ListKind{Public, OpenAccess} {
		es, err := s.client(Credentials{Token: "t", Secret: "s"}).Entries(context.Background(), ListOptions{Kind: k, Type: "text", Latest: true})
		if err != nil || len(es) != 1 || es[0].ID != "e1" || es[0].LanguageCode != "eng" || !es[0].Latest {
			t.Fatalf("%v %+v %v", k, es, err)
		}
		if s.last().Header.Get("X-DBL-Authorization") != "" {
			t.Error("anonymous list must not send credentials")
		}
		if want := "/api/" + string(k) + "/latest/type/text"; s.last().URL.Path != want {
			t.Errorf("path %s want %s", s.last().URL.Path, want)
		}
	}
}

func TestSignedRequests(t *testing.T) {
	s := newServer(t)
	creds := Credentials{Token: "TokenABC", Secret: "SecretXYZ"}
	c := s.client(creds)
	c.Now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	es, err := c.Entries(context.Background(), ListOptions{Kind: Owned})
	if err != nil || len(es) != 1 || es[0].ID != "e2" {
		t.Fatalf("%+v %v", es, err)
	}
	r := s.last()
	h := r.Header.Get("X-DBL-Authorization")
	if !strings.HasPrefix(h, "version=v1,token=TokenABC,signature=") {
		t.Fatalf("header %q", h)
	}
	if strings.Contains(h, "SecretXYZ") || strings.Contains(h, "secretxyz") {
		t.Error("secret must not be sent")
	}
	if r.Header.Get("Date") != "Fri, 02 Jan 2026 03:04:05 GMT" {
		t.Errorf("date %q", r.Header.Get("Date"))
	}
	hdr := http.Header{"Date": {r.Header.Get("Date")}, "X-DBL-Authorization": {h}, "Accept": {"x"}}
	want := Signature("GET", "/api/owned_entries_list", hdr, creds)
	if !strings.HasSuffix(h, "signature="+want) {
		t.Errorf("signature mismatch: %q vs %q", h, want)
	}
}

func TestSignatureShape(t *testing.T) {
	a := Signature("GET", "/api/x", http.Header{"Date": {"d"}}, Credentials{"TOK", "SEC"})
	b := Signature("GET", "/api/x", http.Header{"Date": {"d"}}, Credentials{"tok", "sec"})
	if a != b || len(a) != 40 {
		t.Errorf("token and secret are lowercased; hex SHA-1 expected: %q %q", a, b)
	}
	if a == Signature("GET", "/api/y", http.Header{"Date": {"d"}}, Credentials{"tok", "sec"}) {
		t.Error("path must affect the signature")
	}
	if a == Signature("GET", "/api/x", http.Header{"Date": {"d"}, "X-Dbl-Extra": {"1"}}, Credentials{"tok", "sec"}) {
		t.Error("x-dbl headers must affect the signature")
	}
}

func TestEntryDownloadAndErrors(t *testing.T) {
	s := newServer(t)
	ctx := context.Background()
	anon := s.client(Credentials{})
	if _, err := anon.Entries(ctx, ListOptions{Kind: Owned}); !errors.Is(err, ErrNoCredentials) {
		t.Errorf("owned without creds: %v", err)
	}
	if _, err := anon.Download(ctx, "e1", "7", &bytes.Buffer{}, 1000); !errors.Is(err, ErrNoCredentials) {
		t.Errorf("download without creds: %v", err)
	}
	c := s.client(Credentials{Token: "t", Secret: "s"})
	raw, err := c.EntryDetail(ctx, "e1")
	if err != nil || !strings.Contains(string(raw), "licenses") {
		t.Fatalf("%s %v", raw, err)
	}
	var buf bytes.Buffer
	if n, err := c.Download(ctx, "e1", "7", &buf, 1000); err != nil || n != 11 || buf.String() != "PK-fake-zip" {
		t.Fatalf("%d %q %v", n, buf.String(), err)
	}
	if _, err := c.Download(ctx, "e1", "big", &bytes.Buffer{}, 10); !errors.Is(err, ErrTooLarge) {
		t.Errorf("size cap: %v", err)
	}
	if _, err := c.EntryDetail(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("404: %v", err)
	}
}

func TestStatusMapping(t *testing.T) {
	for code, want := range map[int]error{401: ErrUnauthorized, 403: ErrUnauthorized, 404: ErrNotFound, 429: ErrRateLimit} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
		c := New(Credentials{})
		c.BaseURL = srv.URL
		if _, err := c.Entries(context.Background(), ListOptions{Kind: Public}); !errors.Is(err, want) {
			t.Errorf("%d: %v", code, err)
		}
		srv.Close()
	}
}

func TestCancellation(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer slow.Close()
	c := New(Credentials{})
	c.BaseURL = slow.URL
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := c.Entries(ctx, ListOptions{Kind: Public}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("%v", err)
	}
}

// Commands -------------------------------------------------------------------------------------

func rigUp(t *testing.T) (*command.Registry, *secrets.Memory) {
	t.Helper()
	s := newServer(t)
	mem := secrets.NewMemory()
	r := command.NewRegistry()
	Register(r, mem, WithBaseURL(s.URL+"/api"))
	return r, mem
}

func run(t *testing.T, r *command.Registry, id, params string, out any) error {
	t.Helper()
	res, err := r.Run(context.Background(), &command.Env{}, id, json.RawMessage(params))
	if err != nil {
		return err
	}
	b, _ := json.Marshal(res)
	if strings.Contains(string(b), "SecretXYZ") {
		t.Fatalf("%s leaks the secret", id)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatal(err)
		}
	}
	return nil
}

func code(err error) string {
	var ce *command.Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func TestCommandsEndToEnd(t *testing.T) {
	r, mem := rigUp(t)

	var st StatusResult
	if err := run(t, r, "dbl.status", `{}`, &st); err != nil || st.HasCredentials || len(st.Unavailable) == 0 {
		t.Fatalf("%+v %v", st, err)
	}
	var er EntriesResult
	if err := run(t, r, "dbl.entries", `{"list":"open_access"}`, &er); err != nil || len(er.Entries) != 1 {
		t.Fatalf("%+v %v", er, err)
	}
	e := er.Entries[0]
	if e.Licence.Commercial || e.Licence.RightsHolder != "Example Society" || e.Licence.Confirmed || e.Licence.AttributionLine() == "" {
		t.Errorf("licence %+v", e.Licence)
	}
	if err := run(t, r, "dbl.entries", `{"list":"owned"}`, nil); code(err) != "unavailable" {
		t.Errorf("owned without creds: %v", err)
	}
	if err := run(t, r, "dbl.entry", `{"id":"e1"}`, nil); code(err) != "unavailable" {
		t.Errorf("entry without creds: %v", err)
	}
	dest := filepath.Join(t.TempDir(), "bundle.zip")
	if err := run(t, r, "dbl.download", `{"entry":"e1","license":"7","dest":`+quote(dest)+`}`, nil); code(err) != "unavailable" {
		t.Errorf("download without creds: %v", err)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("no file should be created without credentials")
	}

	if err := run(t, r, "dbl.set_credentials", `{"token":"TokenABC","secret":"SecretXYZ"}`, nil); err != nil {
		t.Fatal(err)
	}
	if v, _ := mem.Get("dbl.secret"); v != "SecretXYZ" {
		t.Fatal("secret not in keychain")
	}
	if err := run(t, r, "dbl.status", `{}`, &st); err != nil || !st.HasCredentials {
		t.Fatalf("%+v %v", st, err)
	}
	if err := run(t, r, "dbl.entries", `{"list":"owned","type":"text"}`, &er); err != nil || er.Entries[0].ID != "e2" {
		t.Fatalf("%+v %v", er, err)
	}
	var en EntryResult
	if err := run(t, r, "dbl.entry", `{"id":"e1"}`, &en); err != nil || !strings.Contains(string(en.Detail), "revisions") {
		t.Fatalf("%+v %v", en, err)
	}
	var dr DownloadResult
	if err := run(t, r, "dbl.download", `{"entry":"e1","license":"7","dest":`+quote(dest)+`}`, &dr); err != nil || dr.Bytes != 11 {
		t.Fatalf("%+v %v", dr, err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "PK-fake-zip" {
		t.Errorf("file %q", b)
	}
	if err := run(t, r, "dbl.download", `{"entry":"e1","license":"7","dest":`+quote(dest)+`}`, nil); code(err) != "bad_params" {
		t.Errorf("must not overwrite: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "x.zip")
	if err := run(t, r, "dbl.download", `{"entry":"e1","license":"nope","dest":`+quote(missing)+`}`, nil); code(err) != "not_found" {
		t.Errorf("404: %v", err)
	}
	if _, err := os.Stat(missing); err == nil {
		t.Error("failed download must not leave a file")
	}
	if err := run(t, r, "dbl.entries", `{"list":"bogus"}`, nil); code(err) != "bad_params" {
		t.Errorf("enum: %v", err)
	}
	if err := run(t, r, "dbl.clear_credentials", `{}`, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Get("dbl.token"); !errors.Is(err, secrets.ErrNotFound) {
		t.Error("token should be gone")
	}
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestCommandsDescribeThemselves(t *testing.T) {
	r, _ := rigUp(t)
	for _, d := range r.List() {
		if !strings.HasPrefix(d.ID, "dbl.") || d.Summary == "" || d.Params == nil || d.Result == nil {
			t.Errorf("%+v", d)
		}
	}
}
