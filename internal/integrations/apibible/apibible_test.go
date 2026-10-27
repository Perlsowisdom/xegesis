package apibible

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/integrations/secrets"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

const testKey = "test-key-123"

type fake struct {
	mu       sync.Mutex
	srv      *httptest.Server
	requests []*http.Request
	fums     []string // raw queries received by the usage-reporting endpoint
}

func (f *fake) hits(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if strings.HasPrefix(r.URL.Path, prefix) {
			n++
		}
	}
	return n
}

func (f *fake) last() *http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func newFake(t *testing.T) *fake {
	t.Helper()
	f := &fake{}
	mux := http.NewServeMux()
	mux.HandleFunc("/f3", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.fums = append(f.fums, r.URL.RawQuery)
		f.mu.Unlock()
	})
	mux.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r)
		f.mu.Unlock()
		if r.Header.Get("api-key") != testKey {
			w.WriteHeader(401)
			w.Write([]byte(`{"statusCode":401,"error":"Unauthorized","message":"bad key"}`))
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/v1")
		switch {
		case p == "/bibles":
			w.Write([]byte(`{"data":[{"id":"b1-01","abbreviation":"KJV","name":"King James","language":{"id":"eng","name":"English"},"copyright":"Public domain"}]}`))
		case p == "/bibles/b1-01":
			w.Write([]byte(`{"data":{"id":"b1-01","abbreviation":"KJV","name":"King James","language":{"id":"eng"},"copyright":"Copyright Example Society."}}`))
		case p == "/bibles/b1-01/books":
			w.Write([]byte(`{"data":[{"id":"GEN","abbreviation":"Gen","name":"Genesis","nameLong":"Book of Genesis"}]}`))
		case strings.HasPrefix(p, "/bibles/b1-01/passages/"):
			id := strings.TrimPrefix(p, "/bibles/b1-01/passages/")
			w.Write([]byte(`{"data":{"id":"` + id + `","bibleId":"b1-01","content":"[28] And we know...","reference":"Romans 8:28-30","verseCount":3,"copyright":"Copyright Example Society."},"meta":{"fumsToken":"tok-passage","fumsId":"old-id"}}`))
		case strings.HasPrefix(p, "/bibles/b1-01/chapters/"):
			id := strings.TrimPrefix(p, "/bibles/b1-01/chapters/")
			w.Write([]byte(`{"data":{"id":"` + id + `","content":"chapter ` + id + `","verseCount":2,"copyright":"Copyright Example Society."},"meta":{"fumsToken":"tok-` + id + `"}}`))
		case p == "/bibles/b1-01/search":
			w.Write([]byte(`{"data":{"query":"love","limit":2,"offset":0,"total":2,"verseCount":2,"verses":[{"id":"JHN.3.16","bookId":"JHN","chapterId":"JHN.3","text":"For God so loved","reference":"John 3:16"}]},"meta":{"fumsToken":"tok-search"}}`))
		case p == "/bibles/denied-01/passages/ROM.8.28-ROM.8.30":
			w.WriteHeader(403)
		case p == "/bibles/limited/books":
			w.Header().Set("Retry-After", "42")
			w.WriteHeader(429)
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"message":"nothing here"}`))
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) client(key string) *Client {
	c := New(key, NewCache(0))
	c.BaseURL, c.FUMSURL = f.srv.URL+"/v1", f.srv.URL+"/f3"
	return c
}

func rng(t *testing.T, ref string) verse.Range {
	t.Helper()
	r, err := verse.Parse(ref)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestTargets(t *testing.T) {
	cases := []struct {
		ref  string
		want []Target
		err  error
	}{
		{"Rom 8:28-30", []Target{{ID: "ROM.8.28-ROM.8.30"}}, nil},
		{"John 3:16", []Target{{ID: "JHN.3.16-JHN.3.16"}}, nil},
		{"1 John 4:7-8", []Target{{ID: "1JN.4.7-1JN.4.8"}}, nil},
		{"Song 2:1-4", []Target{{ID: "SNG.2.1-SNG.2.4"}}, nil},
		{"Ps 23:1-6", []Target{{ID: "PSA.23.1-PSA.23.6"}}, nil},
		{"Ps 23", []Target{{Chapter: true, ID: "PSA.23"}}, nil},
		{"Ps 23-24", []Target{{Chapter: true, ID: "PSA.23"}, {Chapter: true, ID: "PSA.24"}}, nil},
		{"Jude 5", []Target{{ID: "JUD.1.5-JUD.1.5"}}, nil},
		{"Jude 3-5", []Target{{ID: "JUD.1.3-JUD.1.5"}}, nil},
		{"Phlm 1-5", []Target{{ID: "PHM.1.1-PHM.1.5"}}, nil},
		{"3 John 4", []Target{{ID: "3JN.1.4-3JN.1.4"}}, nil},
		{"Phil 4:4-9", []Target{{ID: "PHP.4.4-PHP.4.9"}}, nil},
		{"Ezek 1:1-3", []Target{{ID: "EZK.1.1-EZK.1.3"}}, nil},
		{"Matt 5:3-7:29", []Target{{ID: "MAT.5.3-MAT.7.29"}}, nil},
		{"Sir 1:1-4", []Target{{ID: "SIR.1.1-SIR.1.4"}}, nil},
		{"Rev 22", []Target{{Chapter: true, ID: "REV.22"}}, nil},
		{"Gen 1-9", nil, nil}, // too many chapters; checked below
	}
	for _, c := range cases {
		got, err := Targets(rng(t, c.ref))
		if c.ref == "Gen 1-9" {
			if err == nil {
				t.Errorf("%s: want chapter-limit error", c.ref)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.ref, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v want %v", c.ref, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v want %v", c.ref, got, c.want)
			}
		}
	}
}

func TestOpenEndedRangeRejected(t *testing.T) {
	r := verse.Range{Start: 45008028, End: 45008999}
	if _, err := Targets(r); !errors.Is(err, ErrOpenEnd) {
		t.Fatalf("got %v", err)
	}
	if _, err := PassageID(rng(t, "Ps 23")); !errors.Is(err, ErrOpenEnd) {
		t.Fatalf("whole chapter has no passage ID: %v", err)
	}
	if id, err := PassageID(rng(t, "Rom 8:28-30")); err != nil || id != "ROM.8.28-ROM.8.30" {
		t.Fatalf("%q %v", id, err)
	}
}

func TestEveryBookHasACode(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range verse.Books() {
		c, err := BookCode(b.Number)
		if err != nil || len(c) != 3 {
			t.Errorf("%s: %q %v", b.Name, c, err)
		}
		if seen[c] {
			t.Errorf("duplicate code %s", c)
		}
		seen[c] = true
	}
}

func TestBiblesSendsKeyAndFilters(t *testing.T) {
	f := newFake(t)
	got, err := f.client(testKey).Bibles(context.Background(), BibleFilter{Language: "eng", Abbreviation: "KJV"})
	if err != nil || len(got) != 1 || got[0].Abbreviation != "KJV" {
		t.Fatalf("%v %v", got, err)
	}
	q := f.last().URL.Query()
	if q.Get("language") != "eng" || q.Get("abbreviation") != "KJV" || q.Get("include-full-details") != "true" || q.Has("name") {
		t.Errorf("query = %v", q)
	}
	if f.last().Header.Get("api-key") != testKey {
		t.Error("api-key header missing")
	}
}

func TestBibleAndBooks(t *testing.T) {
	f := newFake(t)
	c := f.client(testKey)
	b, err := c.Bible(context.Background(), "b1-01")
	if err != nil || b.Copyright != "Copyright Example Society." {
		t.Fatalf("%+v %v", b, err)
	}
	books, err := c.Books(context.Background(), "b1-01")
	if err != nil || len(books) != 1 || books[0].ID != "GEN" || books[0].NameLong == "" {
		t.Fatalf("%+v %v", books, err)
	}
}

func TestPassagePassageEndpoint(t *testing.T) {
	f := newFake(t)
	p, err := f.client(testKey).Passage(context.Background(), "b1-01", rng(t, "Rom 8:28-30"), PassageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if f.last().URL.Path != "/v1/bibles/b1-01/passages/ROM.8.28-ROM.8.30" {
		t.Errorf("path %s", f.last().URL.Path)
	}
	q := f.last().URL.Query()
	if q.Get("content-type") != "text" || q.Get("include-titles") != "false" {
		t.Errorf("query %v", q)
	}
	if p.Copyright == "" || len(p.FUMS) != 1 || p.FUMS[0].Token != "tok-passage" || p.Cached {
		t.Errorf("%+v", p)
	}
}

func TestPassageChaptersJoined(t *testing.T) {
	f := newFake(t)
	p, err := f.client(testKey).Passage(context.Background(), "b1-01", rng(t, "Ps 23-24"), PassageOptions{ContentType: "html"})
	if err != nil {
		t.Fatal(err)
	}
	if f.hits("/v1/bibles/b1-01/chapters/") != 2 || f.last().URL.Query().Get("content-type") != "html" {
		t.Errorf("hits %d, query %v", f.hits("/v1/bibles/b1-01/chapters/"), f.last().URL.Query())
	}
	if p.VerseCount != 4 || len(p.FUMS) != 2 || !strings.Contains(p.Content, "PSA.23") || !strings.Contains(p.Content, "PSA.24") {
		t.Errorf("%+v", p)
	}
}

func TestCacheServesRepeatsAndExpires(t *testing.T) {
	f := newFake(t)
	c := f.client(testKey)
	now := time.Now()
	c.Cache.now = func() time.Time { return now }
	ctx := context.Background()
	c.Books(ctx, "b1-01")
	c.Books(ctx, "b1-01")
	if n := f.hits("/v1/bibles/b1-01/books"); n != 1 {
		t.Fatalf("second call should be cached, hits=%d", n)
	}
	now = now.Add(DefaultCacheAge + time.Hour)
	c.Books(ctx, "b1-01")
	if n := f.hits("/v1/bibles/b1-01/books"); n != 2 {
		t.Fatalf("expired entry should be refetched, hits=%d", n)
	}
	// Search is never cached.
	c.Search(ctx, "b1-01", SearchOptions{Query: "love"})
	c.Search(ctx, "b1-01", SearchOptions{Query: "love"})
	if n := f.hits("/v1/bibles/b1-01/search"); n != 2 {
		t.Fatalf("searches should not be cached, hits=%d", n)
	}
}

func TestCacheTTLNeverExceedsThirtyDays(t *testing.T) {
	if c := NewCache(90 * 24 * time.Hour); c.ttl > MaxCacheAge {
		t.Fatalf("ttl %v", c.ttl)
	}
	if c := NewCache(7 * 24 * time.Hour); c.ttl != 7*24*time.Hour {
		t.Fatalf("ttl %v", c.ttl)
	}
}

func TestSearch(t *testing.T) {
	f := newFake(t)
	res, err := f.client(testKey).Search(context.Background(), "b1-01", SearchOptions{Query: "love", Limit: 2, Offset: 4, Sort: "canonical"})
	if err != nil {
		t.Fatal(err)
	}
	q := f.last().URL.Query()
	if q.Get("query") != "love" || q.Get("limit") != "2" || q.Get("offset") != "4" || q.Get("sort") != "canonical" {
		t.Errorf("query %v", q)
	}
	if res.Total != 2 || len(res.Verses) != 1 || res.Verses[0].Reference != "John 3:16" || res.FUMS[0].Token != "tok-search" {
		t.Errorf("%+v", res)
	}
}

func TestErrorMapping(t *testing.T) {
	f := newFake(t)
	ctx := context.Background()
	if _, err := f.client("wrong").Bibles(ctx, BibleFilter{}); !errors.Is(err, ErrBadKey) {
		t.Errorf("401: %v", err)
	} else if strings.Contains(err.Error(), "wrong") && !strings.Contains(err.Error(), "bad key") {
		t.Errorf("unexpected: %v", err)
	}
	c := f.client(testKey)
	if _, err := c.Passage(ctx, "denied-01", rng(t, "Rom 8:28-30"), PassageOptions{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("403: %v", err)
	}
	if _, err := c.Books(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("404: %v", err)
	}
	_, err := c.Books(ctx, "limited")
	var ae *APIError
	if !errors.Is(err, ErrRateLimit) || !errors.As(err, &ae) || ae.RetryAfter != 42*time.Second {
		t.Errorf("429: %v", err)
	}
	if _, err := New("", nil).Bibles(ctx, BibleFilter{}); !errors.Is(err, ErrNoKey) {
		t.Errorf("no key: %v", err)
	}
}

func TestContextCancellationAndTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer slow.Close()
	c := New(testKey, nil)
	c.BaseURL = slow.URL
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Bibles(ctx, BibleFilter{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want deadline exceeded, got %v", err)
	}
	c.HTTP.Timeout = 50 * time.Millisecond
	if _, err := c.Bibles(context.Background(), BibleFilter{}); err == nil {
		t.Error("client timeout should fail the request")
	}
}

func TestReportFUMS(t *testing.T) {
	f := newFake(t)
	if err := f.client(testKey).ReportFUMS(context.Background(), "dev1", "sess1", []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	q := f.fums[0]
	for _, want := range []string{"t=a", "t=b", "dId=dev1", "sId=sess1"} {
		if !strings.Contains(q, want) {
			t.Errorf("%q missing %q", q, want)
		}
	}
	if strings.Contains(q, "uId") || strings.Contains(q, testKey) {
		t.Errorf("report must carry no user ID or key: %q", q)
	}
}

// Commands -------------------------------------------------------------------------------------

func newRig(t *testing.T) (*command.Registry, *secrets.Memory, *fake) {
	t.Helper()
	f := newFake(t)
	s := secrets.NewMemory()
	r := command.NewRegistry()
	Register(r, s, WithBaseURL(f.srv.URL+"/v1"), WithFUMSURL(f.srv.URL+"/f3"))
	return r, s, f
}

func run(t *testing.T, r *command.Registry, id, params string, out any) error {
	t.Helper()
	res, err := r.Run(context.Background(), &command.Env{}, id, json.RawMessage(params))
	if err != nil {
		return err
	}
	b, _ := json.Marshal(res)
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(string(b), testKey) {
		t.Fatalf("%s result leaks the API key: %s", id, b)
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
	r, s, f := newRig(t)

	var st StatusResult
	if err := run(t, r, "apibible.status", `{}`, &st); err != nil || st.KeySet || st.Commercial {
		t.Fatalf("%+v %v", st, err)
	}
	if err := run(t, r, "apibible.bibles", `{}`, nil); code(err) != "unavailable" {
		t.Fatalf("no key should be unavailable: %v", err)
	}

	var kr KeyResult
	if err := run(t, r, "apibible.set_key", `{"key":" `+testKey+` "}`, &kr); err != nil || !kr.Stored {
		t.Fatalf("%+v %v", kr, err)
	}
	if v, _ := s.Get("apibible.key"); v != testKey {
		t.Fatalf("keychain holds %q", v)
	}

	if err := run(t, r, "apibible.status", `{"verify":true}`, &st); err != nil || !st.KeySet || st.Valid == nil || !*st.Valid {
		t.Fatalf("%+v %v", st, err)
	}

	var bibles []BibleInfo
	if err := run(t, r, "apibible.bibles", `{"language":"eng"}`, &bibles); err != nil || len(bibles) != 1 || bibles[0].ID != "b1-01" {
		t.Fatalf("%+v %v", bibles, err)
	}

	var books []Book
	if err := run(t, r, "apibible.books", `{"bible":"b1-01"}`, &books); err != nil || len(books) != 1 {
		t.Fatalf("%+v %v", books, err)
	}

	var ps PassageResult
	if err := run(t, r, "apibible.passage", `{"bible":"b1-01","ref":"Rom 8:28-30"}`, &ps); err != nil {
		t.Fatal(err)
	}
	if ps.Content == "" || ps.Reference != "Romans 8:28-30" || ps.VerseCount != 3 {
		t.Errorf("%+v", ps)
	}
	if ps.Licence.Commercial || ps.Licence.Share || ps.Licence.Store || ps.Licence.AttributionLine() == "" {
		t.Errorf("licence %+v", ps.Licence)
	}
	if !strings.Contains(ps.Notice, "Copyright Example Society.") || !strings.Contains(ps.Notice, "api.bible") {
		t.Errorf("notice %q", ps.Notice)
	}
	if !ps.Usage.Reported || len(ps.Usage.Tokens) != 1 || ps.Usage.Tokens[0] != "tok-passage" || len(f.fums) != 1 {
		t.Errorf("usage %+v fums %v", ps.Usage, f.fums)
	}
	if d := licenceDenies(ps); !d {
		t.Error("licence should refuse commercial use")
	}

	// A repeat is served from the cache but still reported.
	if err := run(t, r, "apibible.passage", `{"bible":"b1-01","ref":"Rom 8:28-30"}`, &ps); err != nil || !ps.Cached || len(f.fums) != 2 {
		t.Errorf("cached=%v fums=%d err=%v", ps.Cached, len(f.fums), err)
	}
	if strings.Contains(strings.Join(f.fums, " "), "uId") {
		t.Error("user ID must not be reported")
	}

	var sr SearchResponse
	if err := run(t, r, "apibible.search", `{"bible":"b1-01","query":"love","limit":2}`, &sr); err != nil || sr.Total != 2 || !sr.Usage.Reported || sr.Notice == "" {
		t.Fatalf("%+v %v", sr, err)
	}

	if err := run(t, r, "apibible.passage", `{"bible":"b1-01","ref":"Rom 8:28-end"}`, nil); code(err) != "bad_params" {
		t.Errorf("bad ref: %v", err)
	}
	if err := run(t, r, "apibible.passage", `{"bible":"denied-01","ref":"Rom 8:28-30"}`, nil); code(err) != "unavailable" || !strings.Contains(err.Error(), "plan") {
		t.Errorf("403: %v", err)
	}
	if err := run(t, r, "apibible.passage", `{"bible":"b1-01"}`, nil); code(err) != "bad_params" {
		t.Errorf("missing ref: %v", err)
	}
	if err := run(t, r, "apibible.passage", `{"bible":"b1-01","ref":"Rom 8","content_type":"pdf"}`, nil); code(err) != "bad_params" {
		t.Errorf("enum: %v", err)
	}

	if err := run(t, r, "apibible.clear_key", `{}`, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("apibible.key"); !errors.Is(err, secrets.ErrNotFound) {
		t.Error("key should be gone")
	}
}

func licenceDenies(ps PassageResult) bool {
	d := ps.Licence.Check(licence.Request{Action: licence.Export, Commercial: true})
	return !d.Allowed
}

func TestBadKeyReported(t *testing.T) {
	r, s, _ := newRig(t)
	s.Set("apibible.key", "wrong")
	err := run(t, r, "apibible.bibles", `{}`, nil)
	if code(err) != "unavailable" || !strings.Contains(err.Error(), "rejected the API key") {
		t.Fatalf("%v", err)
	}
	var st StatusResult
	if err := run(t, r, "apibible.status", `{"verify":true}`, &st); err != nil || st.Valid == nil || *st.Valid {
		t.Fatalf("%+v %v", st, err)
	}
}

func TestSetKeyRejectsBlank(t *testing.T) {
	r, _, _ := newRig(t)
	if err := run(t, r, "apibible.set_key", `{"key":"   "}`, nil); code(err) != "bad_params" {
		t.Fatalf("%v", err)
	}
	if err := run(t, r, "apibible.set_key", `{}`, nil); code(err) != "bad_params" {
		t.Fatalf("%v", err)
	}
}

func TestCommandsDescribeThemselves(t *testing.T) {
	r, _, _ := newRig(t)
	for _, d := range r.List() {
		if !strings.HasPrefix(d.ID, "apibible.") || d.Summary == "" || d.Params == nil || d.Result == nil {
			t.Errorf("%+v", d)
		}
	}
	d, _ := r.Describe("apibible.passage")
	if d.Params.Properties["content_type"].Enum[0] != "text" || len(d.Params.Required) != 2 {
		t.Errorf("%+v", d.Params)
	}
	if d.Result.Properties["licence"] == nil || d.Result.Properties["usage"] == nil {
		t.Errorf("result schema lacks licence/usage: %+v", d.Result.Properties)
	}
}
