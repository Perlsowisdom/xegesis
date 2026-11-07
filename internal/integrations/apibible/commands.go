package apibible

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/integrations/secrets"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

const (
	keyName    = "apibible.key"
	deviceName = "apibible.device"
	siteURL    = "https://api.bible"
)

// Option customises Register, mainly for tests.
type Option func(*service)

func WithBaseURL(u string) Option { return func(s *service) { s.baseURL = u } }
func WithFUMSURL(u string) Option { return func(s *service) { s.fumsURL = u } }

type service struct {
	secrets   secrets.Store
	baseURL   string
	fumsURL   string
	http      *http.Client
	cache     *Cache
	sessionID string
}

// Register adds the apibible.* commands. The API key is read from the keychain on each call.
func Register(r *command.Registry, s secrets.Store, opts ...Option) {
	svc := &service{secrets: s, baseURL: DefaultBaseURL, fumsURL: DefaultFUMSURL, cache: NewCache(0), sessionID: randomID()}
	for _, o := range opts {
		o(svc)
	}
	r.Add(
		command.Define("apibible.set_key", "Store your own API.Bible key in the system keychain.", false, svc.setKey),
		command.Define("apibible.clear_key", "Remove the API.Bible key from the system keychain and clear cached content.", false, svc.clearKey),
		command.Define("apibible.status", "Show whether an API.Bible key is set and, optionally, whether it works.", false, svc.status),
		command.Define("apibible.bibles", "List Bibles available to your API.Bible key.", false, svc.bibles),
		command.Define("apibible.books", "List the books of an API.Bible Bible.", false, svc.books),
		command.Define("apibible.passage", "Fetch a passage from API.Bible, with its copyright and usage reporting.", false, svc.passage),
		command.Define("apibible.search", "Search an API.Bible Bible for words or a reference.", false, svc.search),
	)
}

func randomID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *service) client(ctx context.Context) (*Client, error) {
	key, err := s.secrets.Get(keyName)
	if errors.Is(err, secrets.ErrNotFound) || (err == nil && key == "") {
		return nil, command.Errorf("unavailable", "%v", ErrNoKey)
	}
	if err != nil {
		return nil, err
	}
	c := New(key, s.cache)
	c.BaseURL, c.FUMSURL = s.baseURL, s.fumsURL
	if s.http != nil {
		c.HTTP = s.http
	}
	return c, nil
}

// fail turns API.Bible errors into command errors people and agents can act on.
func fail(err error) error {
	var ae *APIError
	switch {
	case errors.As(err, &ae) && errors.Is(err, ErrNotFound):
		return command.Errorf("not_found", "%v", err)
	case errors.As(err, &ae) && errors.Is(err, ErrBadRequest):
		return command.Errorf("bad_params", "%v", err)
	case errors.As(err, &ae):
		return command.Errorf("unavailable", "%v", err)
	case errors.Is(err, ErrOpenEnd):
		return command.Errorf("bad_params", "%v", err)
	}
	return err
}

// Licence ----------------------------------------------------------------------------------------

// Terms describes the licence of Scripture fetched from API.Bible, in the shape internal/licence
// uses, plus the notice the terms ask to be shown with it.
type Terms struct {
	Licence licence.Licence `json:"licence"`
	Notice  string          `json:"notice" desc:"Show this copyright notice wherever the text is displayed"`
}

// termsFor builds licence info for text from one Bible. The free plan is non-commercial and cached
// text may not be kept, shared or exported without limit, so those stay at their strictest.
func termsFor(copyright string) Terms {
	l, _ := licence.Defaults(licence.Unknown, "")
	l.Commercial, l.Share, l.Store = false, false, false
	l.URL = siteURL
	notice := strings.TrimSpace(copyright)
	l.Attribution = notice
	if notice == "" {
		l.Attribution = "Scripture provided by API.Bible (" + siteURL + ")"
		notice = l.Attribution
	} else {
		notice += " Scripture provided by API.Bible (" + siteURL + ")."
	}
	return Terms{Licence: l, Notice: notice}
}

// Usage reporting ----------------------------------------------------------------------------------

// Usage is the usage-reporting (FUMS) information for displayed Scripture.
type Usage struct {
	Tokens   []string `json:"tokens,omitempty" desc:"FUMS tokens returned with the text"`
	Reported bool     `json:"reported" desc:"true when the tokens were sent to API.Bible's usage reporting"`
	Error    string   `json:"error,omitempty"`
}

func tokens(metas []Meta) []string {
	var out []string
	for _, m := range metas {
		if m.Token != "" {
			out = append(out, m.Token)
		}
	}
	return out
}

// report sends tokens to API.Bible. Failure is returned in the Usage, not as an error, so the text
// still reaches the user; the caller sees reported=false.
func (s *service) report(ctx context.Context, c *Client, metas []Meta) Usage {
	u := Usage{Tokens: tokens(metas)}
	if len(u.Tokens) == 0 {
		return u
	}
	if err := c.ReportFUMS(ctx, s.deviceID(), s.sessionID, u.Tokens); err != nil {
		u.Error = err.Error()
		return u
	}
	u.Reported = true
	return u
}

// deviceID is a random, anonymous ID made once per installation. It is not a secret, but the
// keychain is the one place the integrations keep state, so it lives there.
func (s *service) deviceID() string {
	if id, err := s.secrets.Get(deviceName); err == nil && id != "" {
		return id
	}
	id := randomID()
	s.secrets.Set(deviceName, id)
	return id
}

// Key commands ---------------------------------------------------------------------------------

type SetKeyParams struct {
	Key string `json:"key" required:"true" desc:"Your own API.Bible key from your account dashboard; it is never shown again"`
}

type KeyResult struct {
	Stored bool `json:"stored"`
}

func (s *service) setKey(_ context.Context, _ *command.Env, p SetKeyParams) (KeyResult, error) {
	key := strings.TrimSpace(p.Key)
	if key == "" {
		return KeyResult{}, command.Errorf("bad_params", "key is empty")
	}
	if err := s.secrets.Set(keyName, key); err != nil {
		return KeyResult{}, command.Errorf("failed", "could not save to the system keychain: %v", err)
	}
	s.cache.Clear()
	return KeyResult{Stored: true}, nil
}

func (s *service) clearKey(_ context.Context, _ *command.Env, _ struct{}) (KeyResult, error) {
	if err := s.secrets.Delete(keyName); err != nil {
		return KeyResult{}, command.Errorf("failed", "could not update the system keychain: %v", err)
	}
	s.cache.Clear()
	return KeyResult{}, nil
}

type StatusParams struct {
	Verify bool `json:"verify,omitempty" desc:"Make one request to API.Bible to check the key works (uses one of your monthly calls)"`
}

type StatusResult struct {
	KeySet     bool   `json:"key_set"`
	Valid      *bool  `json:"valid,omitempty" desc:"Set only when verify was requested"`
	Message    string `json:"message,omitempty"`
	Commercial bool   `json:"commercial_allowed" desc:"Always false: the free plan is for non-commercial use"`
	CacheItems int    `json:"cached_items"`
}

func (s *service) status(ctx context.Context, _ *command.Env, p StatusParams) (StatusResult, error) {
	res := StatusResult{CacheItems: s.cache.Len()}
	c, err := s.client(ctx)
	if err != nil {
		res.Message = ErrNoKey.Error()
		return res, nil
	}
	res.KeySet = true
	if !p.Verify {
		return res, nil
	}
	ok := true
	if err := c.Verify(ctx); err != nil {
		ok = false
		res.Message = err.Error()
	}
	res.Valid = &ok
	return res, nil
}

// Content commands ---------------------------------------------------------------------------------

type BiblesParams struct {
	Language     string `json:"language,omitempty" desc:"ISO 639-3 language code, e.g. \"eng\""`
	Abbreviation string `json:"abbreviation,omitempty" desc:"e.g. \"KJV\""`
	Name         string `json:"name,omitempty"`
}

type BibleInfo struct {
	ID           string `json:"id"`
	Abbreviation string `json:"abbreviation"`
	Name         string `json:"name"`
	Language     string `json:"language"`
	Description  string `json:"description,omitempty"`
	Copyright    string `json:"copyright,omitempty"`
}

func (s *service) bibles(ctx context.Context, _ *command.Env, p BiblesParams) ([]BibleInfo, error) {
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	list, err := c.Bibles(ctx, BibleFilter{Language: p.Language, Abbreviation: p.Abbreviation, Name: p.Name})
	if err != nil {
		return nil, fail(err)
	}
	out := make([]BibleInfo, len(list))
	for i, b := range list {
		out[i] = BibleInfo{ID: b.ID, Abbreviation: b.Abbreviation, Name: b.Name, Language: b.Language.ID,
			Description: b.Description, Copyright: b.Copyright}
	}
	return out, nil
}

type BibleParam struct {
	Bible string `json:"bible" required:"true" desc:"API.Bible Bible ID, from apibible.bibles"`
}

func (s *service) books(ctx context.Context, _ *command.Env, p BibleParam) ([]Book, error) {
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	b, err := c.Books(ctx, p.Bible)
	return b, fail(err)
}

type PassageParams struct {
	Bible        string `json:"bible" required:"true" desc:"API.Bible Bible ID, from apibible.bibles"`
	Ref          string `json:"ref" required:"true" desc:"A Bible reference, e.g. \"Rom 8:28-30\" or \"Ps 23\""`
	ContentType  string `json:"content_type,omitempty" enum:"text,html" desc:"Text format; default text"`
	IncludeNotes bool   `json:"include_notes,omitempty" desc:"Include footnotes"`
}

type PassageResult struct {
	Bible      string `json:"bible"`
	Reference  string `json:"reference"`
	Content    string `json:"content"`
	VerseCount int    `json:"verse_count"`
	Copyright  string `json:"copyright,omitempty"`
	Terms
	Usage  Usage `json:"usage" desc:"Usage reporting the terms require; reported=false means sending failed"`
	Cached bool  `json:"cached" desc:"Served from the in-memory cache (refreshed at least every 30 days)"`
}

func (s *service) passage(ctx context.Context, _ *command.Env, p PassageParams) (PassageResult, error) {
	r, err := verse.Parse(p.Ref)
	if err != nil {
		return PassageResult{}, command.Errorf("bad_params", "%v", err)
	}
	c, err := s.client(ctx)
	if err != nil {
		return PassageResult{}, err
	}
	psg, err := c.Passage(ctx, p.Bible, r, PassageOptions{ContentType: p.ContentType, IncludeNotes: p.IncludeNotes})
	if err != nil {
		return PassageResult{}, fail(err)
	}
	if psg.Copyright == "" {
		if b, err := c.Bible(ctx, p.Bible); err == nil {
			psg.Copyright = b.Copyright
		}
	}
	return PassageResult{Bible: p.Bible, Reference: psg.Reference, Content: psg.Content, VerseCount: psg.VerseCount,
		Copyright: psg.Copyright, Terms: termsFor(psg.Copyright), Usage: s.report(ctx, c, psg.FUMS), Cached: psg.Cached}, nil
}

type SearchParams struct {
	Bible  string `json:"bible" required:"true" desc:"API.Bible Bible ID, from apibible.bibles"`
	Query  string `json:"query" required:"true" desc:"Words to find, or a reference"`
	Limit  int    `json:"limit,omitempty" desc:"Most results to return (API.Bible default 10)"`
	Offset int    `json:"offset,omitempty"`
	Sort   string `json:"sort,omitempty" enum:"relevance,canonical,reverse-canonical"`
}

type SearchResponse struct {
	Query      string  `json:"query"`
	Total      int     `json:"total"`
	VerseCount int     `json:"verse_count"`
	Verses     []Verse `json:"verses,omitempty"`
	Passages   []struct {
		Reference string `json:"reference"`
		Content   string `json:"content"`
	} `json:"passages,omitempty"`
	Terms
	Usage Usage `json:"usage"`
}

func (s *service) search(ctx context.Context, _ *command.Env, p SearchParams) (SearchResponse, error) {
	c, err := s.client(ctx)
	if err != nil {
		return SearchResponse{}, err
	}
	res, err := c.Search(ctx, p.Bible, SearchOptions{Query: p.Query, Limit: p.Limit, Offset: p.Offset, Sort: p.Sort})
	if err != nil {
		return SearchResponse{}, fail(err)
	}
	copyright := ""
	if b, err := c.Bible(ctx, p.Bible); err == nil {
		copyright = b.Copyright
	}
	out := SearchResponse{Query: res.Query, Total: res.Total, VerseCount: res.VerseCount, Verses: res.Verses,
		Terms: termsFor(copyright), Usage: s.report(ctx, c, res.FUMS)}
	for _, ps := range res.Passages {
		out.Passages = append(out.Passages, struct {
			Reference string `json:"reference"`
			Content   string `json:"content"`
		}{ps.Reference, ps.Content})
	}
	return out, nil
}
