// Package dbl is a client for the Digital Bible Library API 4.0, written from its public reference
// (https://app.thedigitalbiblelibrary.org/static/docs/). What an individual can do is limited:
//
//   - Listing public and open-access entries needs no account.
//   - Everything else (owned and publishable entries, entry details with licences, downloading a
//     text bundle) needs an organisation's API token and secret, which only an organisation
//     administrator can create, and a licence the organisation holds.
//
// The older email-and-key login is deprecated and is not supported: its key is equivalent to the
// account password. See docs/integrations.md.
package dbl

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const DefaultBaseURL = "https://thedigitalbiblelibrary.org/api"

var (
	ErrNoCredentials = errors.New("this needs a DBL organisation API token and secret: add them with the dbl.set_credentials command")
	ErrUnauthorized  = errors.New("the Digital Bible Library refused the request: check the token, secret and licence")
	ErrNotFound      = errors.New("not found in the Digital Bible Library")
	ErrRateLimit     = errors.New("Digital Bible Library rate limit reached")
	ErrTooLarge      = errors.New("download is larger than the allowed size")
)

// ListKind selects an entry list.
type ListKind string

const (
	Public      ListKind = "public_entries_list"      // non-confidential entries, visible without an account
	OpenAccess  ListKind = "open_access_entries_list" // entries licensed for unrestricted public access
	Owned       ListKind = "owned_entries_list"       // entries owned by the credentials' organisation
	Publishable ListKind = "publishable_entries_list" // entries the organisation is licensed to publish
)

// Anonymous reports whether the list can be read without credentials.
func (k ListKind) Anonymous() bool { return k == Public || k == OpenAccess }

// Credentials are an organisation's API token and secret key.
type Credentials struct {
	Token  string
	Secret string
}

// Client talks to the DBL API. Creds may be empty for anonymous lists.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	Creds   Credentials
	Now     func() time.Time
}

func New(creds Credentials) *Client {
	return &Client{BaseURL: DefaultBaseURL, HTTP: &http.Client{Timeout: 60 * time.Second}, Creds: creds, Now: time.Now}
}

// Entry is one item in an entry list, using the fields the reference documents for lists.
type Entry struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type,omitempty"`
	LanguageName string `json:"language_name,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
	CountryName  string `json:"country_name,omitempty"`
	RightsHolder string `json:"rights_holder,omitempty"`
	Href         string `json:"href,omitempty"`
	Latest       bool   `json:"latest"`
	Obsolete     bool   `json:"obsolete"`
	Confidential bool   `json:"confidential"`
}

// ListOptions narrow a list. Type is "text" or "audio" (or empty for both).
type ListOptions struct {
	Kind   ListKind
	Type   string
	Latest bool
}

// Entries lists entries. Public and open-access lists are requested without credentials.
func (c *Client) Entries(ctx context.Context, o ListOptions) ([]Entry, error) {
	path := "/" + string(o.Kind)
	if o.Latest {
		path += "/latest"
	}
	if o.Type != "" {
		path += "/type/" + url.PathEscape(o.Type)
	}
	body, err := c.get(ctx, path, !o.Kind.Anonymous())
	if err != nil {
		return nil, err
	}
	return decodeEntries(body)
}

// decodeEntries accepts either a bare JSON array or an object holding the array, because the
// reference does not show the list's envelope.
func decodeEntries(body []byte) ([]Entry, error) {
	var list []Entry
	if err := json.Unmarshal(body, &list); err == nil {
		return list, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("dbl: unreadable entry list: %w", err)
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := json.Unmarshal(obj[k], &list); err == nil {
			return list, nil
		}
	}
	return nil, errors.New("dbl: no entry list in response")
}

// EntryDetail returns an entry's full record as the API sent it (revisions and, with credentials,
// the licences available to the organisation).
func (c *Client) EntryDetail(ctx context.Context, id string) (json.RawMessage, error) {
	return c.get(ctx, "/entries/"+url.PathEscape(id), c.Creds != (Credentials{}))
}

// Download streams a revision's zip bundle to w. licenseID is a licence ID the organisation holds
// with publisher rights, or "owner" for entries the organisation owns. At most maxBytes are written.
func (c *Client) Download(ctx context.Context, entryID, licenseID string, w io.Writer, maxBytes int64) (int64, error) {
	if c.Creds == (Credentials{}) {
		return 0, ErrNoCredentials
	}
	path := "/entries/" + url.PathEscape(entryID) + "/revision/latest/license/" + url.PathEscape(licenseID) + ".zip"
	resp, err := c.do(ctx, path, true)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	n, err := io.Copy(w, io.LimitReader(resp.Body, maxBytes+1))
	if err == nil && n > maxBytes {
		return n, ErrTooLarge
	}
	return n, err
}

func (c *Client) get(ctx context.Context, path string, signed bool) ([]byte, error) {
	resp, err := c.do(ctx, path, signed)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

func (c *Client) do(ctx context.Context, path string, signed bool) (*http.Response, error) {
	if signed && c.Creds == (Credentials{}) {
		return nil, ErrNoCredentials
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if signed {
		c.sign(req)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	resp.Body.Close()
	switch resp.StatusCode {
	case 401, 403:
		return nil, ErrUnauthorized
	case 404:
		return nil, ErrNotFound
	case 429:
		return nil, ErrRateLimit
	}
	return nil, fmt.Errorf("dbl: HTTP %d", resp.StatusCode)
}

// sign adds the X-DBL-Authorization header following the reference's description of token and
// secret signing (HMAC-SHA1 over the method, path and selected headers). It has been checked only
// against that description, not against a live organisation account.
func (c *Client) sign(req *http.Request) {
	req.Header.Set("Date", c.Now().UTC().Format(http.TimeFormat))
	sig := Signature(req.Method, req.URL.Path, req.Header, c.Creds)
	req.Header.Set("X-DBL-Authorization", "version=v1,token="+c.Creds.Token+",signature="+sig)
}

// Signature computes the request signature: the signing string is "METHOD path", then one line for
// each of content-type, date and x-dbl-* headers (except X-DBL-Authorization) in sorted name order
// (x-dbl-* as name:value, others as the value alone). HMAC-SHA1 is keyed with the lowercased token
// and fed the string and then the lowercased secret.
func Signature(method, path string, h http.Header, creds Credentials) string {
	lines := map[string]string{"content-type": "", "date": ""}
	for name, vals := range h {
		n := strings.ToLower(name)
		if n == "x-dbl-authorization" || (n != "content-type" && n != "date" && !strings.HasPrefix(n, "x-dbl-")) {
			continue
		}
		lines[n] = strings.TrimSpace(strings.Join(vals, ","))
	}
	names := make([]string, 0, len(lines))
	for n := range lines {
		names = append(names, n)
	}
	sort.Strings(names)
	var sb strings.Builder
	sb.WriteString(method + " " + path + "\n")
	for _, n := range names {
		if strings.HasPrefix(n, "x-dbl-") {
			sb.WriteString(n + ":")
		}
		sb.WriteString(lines[n] + "\n")
	}
	mac := hmac.New(sha1.New, []byte(strings.ToLower(creds.Token)))
	mac.Write([]byte(sb.String()))
	mac.Write([]byte(strings.ToLower(creds.Secret)))
	return hex.EncodeToString(mac.Sum(nil))
}
