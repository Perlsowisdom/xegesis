package dbl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/integrations/secrets"
	"github.com/Perlsowisdom/xegesis/internal/licence"
)

const (
	tokenName  = "dbl.token"
	secretName = "dbl.secret"
	maxBundle  = 512 << 20
)

// Option customises Register, mainly for tests.
type Option func(*service)

func WithBaseURL(u string) Option          { return func(s *service) { s.baseURL = u } }
func WithHTTPClient(h *http.Client) Option { return func(s *service) { s.http = h } }

type service struct {
	secrets secrets.Store
	baseURL string
	http    *http.Client
}

// Register adds the dbl.* commands.
func Register(r *command.Registry, s secrets.Store, opts ...Option) {
	svc := &service{secrets: s, baseURL: DefaultBaseURL}
	for _, o := range opts {
		o(svc)
	}
	r.Add(
		command.Define("dbl.set_credentials", "Store a Digital Bible Library organisation API token and secret in the system keychain.", false, svc.setCredentials),
		command.Define("dbl.clear_credentials", "Remove the stored Digital Bible Library credentials.", false, svc.clearCredentials),
		command.Define("dbl.status", "Show which Digital Bible Library features are available with your credentials.", false, svc.status),
		command.Define("dbl.entries", "List Digital Bible Library entries (public and open-access need no account).", false, svc.entries),
		command.Define("dbl.entry", "Show one entry with its revisions and licences (needs organisation credentials).", false, svc.entry),
		command.Define("dbl.download", "Download an entry's latest text bundle (zip) to a file (needs organisation credentials and a licence).", false, svc.download),
	)
}

func (s *service) creds() Credentials {
	tok, err1 := s.secrets.Get(tokenName)
	sec, err2 := s.secrets.Get(secretName)
	if err1 != nil || err2 != nil {
		return Credentials{}
	}
	return Credentials{Token: tok, Secret: sec}
}

func (s *service) client() *Client {
	c := New(s.creds())
	c.BaseURL = s.baseURL
	if s.http != nil {
		c.HTTP = s.http
	}
	return c
}

func fail(err error) error {
	switch {
	case errors.Is(err, ErrNoCredentials):
		return command.Errorf("unavailable", "%v", err)
	case errors.Is(err, ErrNotFound):
		return command.Errorf("not_found", "%v", err)
	case errors.Is(err, ErrUnauthorized), errors.Is(err, ErrRateLimit):
		return command.Errorf("unavailable", "%v", err)
	}
	return err
}

type CredParams struct {
	Token  string `json:"token" required:"true" desc:"Organisation API token (created by an organisation administrator under Admin, Organizations, Roles)"`
	Secret string `json:"secret" required:"true" desc:"The secret key shown once when the token was created"`
}

type OKResult struct {
	Stored bool `json:"stored"`
}

func (s *service) setCredentials(_ context.Context, _ *command.Env, p CredParams) (OKResult, error) {
	if err := s.secrets.Set(tokenName, p.Token); err != nil {
		return OKResult{}, command.Errorf("failed", "could not save to the system keychain: %v", err)
	}
	if err := s.secrets.Set(secretName, p.Secret); err != nil {
		return OKResult{}, command.Errorf("failed", "could not save to the system keychain: %v", err)
	}
	return OKResult{Stored: true}, nil
}

func (s *service) clearCredentials(_ context.Context, _ *command.Env, _ struct{}) (OKResult, error) {
	for _, n := range []string{tokenName, secretName} {
		if err := s.secrets.Delete(n); err != nil {
			return OKResult{}, command.Errorf("failed", "could not update the system keychain: %v", err)
		}
	}
	return OKResult{}, nil
}

type StatusResult struct {
	HasCredentials bool     `json:"has_credentials"`
	Available      []string `json:"available" desc:"Commands that can work with what is configured"`
	Unavailable    []string `json:"unavailable,omitempty" desc:"Features that need an organisation account"`
}

func (s *service) status(_ context.Context, _ *command.Env, _ struct{}) (StatusResult, error) {
	res := StatusResult{HasCredentials: s.creds() != Credentials{},
		Available: []string{"dbl.entries (public, open_access)"}}
	if res.HasCredentials {
		res.Available = []string{"dbl.entries (all lists)", "dbl.entry", "dbl.download"}
		return res, nil
	}
	res.Unavailable = []string{"dbl.entries (owned, publishable)", "dbl.entry", "dbl.download"}
	return res, nil
}

type EntriesParams struct {
	List   string `json:"list,omitempty" enum:"public,open_access,owned,publishable" desc:"Which list; default public"`
	Type   string `json:"type,omitempty" enum:"text,audio"`
	Latest bool   `json:"latest,omitempty" desc:"Only the latest revision of each entry"`
}

type EntryItem struct {
	Entry
	Licence licence.Licence `json:"licence"`
}

type EntriesResult struct {
	List    string      `json:"list"`
	Entries []EntryItem `json:"entries"`
}

var kinds = map[string]ListKind{"": Public, "public": Public, "open_access": OpenAccess, "owned": Owned, "publishable": Publishable}

func (s *service) entries(ctx context.Context, _ *command.Env, p EntriesParams) (EntriesResult, error) {
	kind := kinds[p.List]
	list, err := s.client().Entries(ctx, ListOptions{Kind: kind, Type: p.Type, Latest: p.Latest})
	if err != nil {
		return EntriesResult{}, fail(err)
	}
	res := EntriesResult{List: string(kind), Entries: make([]EntryItem, len(list))}
	for i, e := range list {
		res.Entries[i] = EntryItem{Entry: e, Licence: entryLicence(e, kind)}
	}
	return res, nil
}

// entryLicence is a cautious starting licence. Open-access entries are free to read but their
// specific terms live in the entry's own metadata, so the user must confirm before reuse.
func entryLicence(e Entry, kind ListKind) licence.Licence {
	l, _ := licence.Defaults(licence.Unknown, "")
	if kind == OpenAccess {
		l, _ = licence.Defaults(licence.OpenOther, "")
	}
	l.RightsHolder = e.RightsHolder
	l.URL = "https://app.thedigitalbiblelibrary.org"
	l.Commercial = false
	return l
}

type EntryParams struct {
	ID string `json:"id" required:"true" desc:"DBL entry ID"`
}

type EntryResult struct {
	ID     string          `json:"id"`
	Detail json.RawMessage `json:"detail" desc:"The entry record as the API returned it, with revisions and licences"`
}

func (s *service) entry(ctx context.Context, _ *command.Env, p EntryParams) (EntryResult, error) {
	c := s.client()
	if c.Creds == (Credentials{}) {
		return EntryResult{}, fail(ErrNoCredentials)
	}
	raw, err := c.EntryDetail(ctx, p.ID)
	if err != nil {
		return EntryResult{}, fail(err)
	}
	return EntryResult{ID: p.ID, Detail: raw}, nil
}

type DownloadParams struct {
	Entry   string `json:"entry" required:"true" desc:"DBL entry ID"`
	License string `json:"license" required:"true" desc:"A licence ID your organisation holds with publisher rights, or \"owner\" for entries it owns"`
	Dest    string `json:"dest" required:"true" desc:"File to write the zip bundle to; it must not already exist"`
}

type DownloadResult struct {
	Path    string          `json:"path"`
	Bytes   int64           `json:"bytes"`
	Licence licence.Licence `json:"licence"`
}

func (s *service) download(ctx context.Context, _ *command.Env, p DownloadParams) (DownloadResult, error) {
	c := s.client()
	if c.Creds == (Credentials{}) {
		return DownloadResult{}, fail(ErrNoCredentials)
	}
	dest, err := filepath.Abs(p.Dest)
	if err != nil {
		return DownloadResult{}, command.Errorf("bad_params", "%v", err)
	}
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return DownloadResult{}, command.Errorf("bad_params", "cannot create %s: %v", dest, err)
	}
	n, err := c.Download(ctx, p.Entry, p.License, f, maxBundle)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dest)
		return DownloadResult{}, fail(err)
	}
	l, _ := licence.Defaults(licence.Unknown, "")
	l.Commercial = false
	return DownloadResult{Path: dest, Bytes: n, Licence: l}, nil
}
