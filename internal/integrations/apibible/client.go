// Package apibible is a client for API.Bible (https://docs.api.bible), written from its public
// documentation. Each user supplies their own API key; Xegesis ships none.
package apibible

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

const (
	DefaultBaseURL = "https://rest.api.bible/v1"
	DefaultFUMSURL = "https://fums.api.bible/f3"
	maxBody        = 8 << 20
)

// Client talks to API.Bible with one user's key.
type Client struct {
	BaseURL string
	FUMSURL string
	Key     string
	HTTP    *http.Client
	Cache   *Cache // optional
}

// New makes a client with a 30 second timeout. Use contexts for shorter deadlines.
func New(key string, cache *Cache) *Client {
	return &Client{BaseURL: DefaultBaseURL, FUMSURL: DefaultFUMSURL, Key: key,
		HTTP: &http.Client{Timeout: 30 * time.Second}, Cache: cache}
}

// Meta is the usage-reporting (FUMS) information API.Bible returns with Scripture. The token is what
// current documentation asks apps to report; the older fields are kept for completeness.
type Meta struct {
	Token string `json:"fumsToken,omitempty"`
	ID    string `json:"fumsId,omitempty"`
	Fums  string `json:"fums,omitempty"`
}

type Language struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Bible struct {
	ID           string   `json:"id"`
	DBLID        string   `json:"dblId,omitempty"`
	Abbreviation string   `json:"abbreviation"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Language     Language `json:"language"`
	Type         string   `json:"type,omitempty"`
	Copyright    string   `json:"copyright,omitempty"`
	Info         string   `json:"info,omitempty"`
}

type Book struct {
	ID           string `json:"id"`
	Abbreviation string `json:"abbreviation"`
	Name         string `json:"name"`
	NameLong     string `json:"nameLong,omitempty"`
}

// BibleFilter narrows a list of Bibles. Empty fields are not sent.
type BibleFilter struct {
	Language     string // ISO 639-3, e.g. "eng"
	Abbreviation string
	Name         string
}

// Bibles lists the Bibles the key can use, with copyright details.
func (c *Client) Bibles(ctx context.Context, f BibleFilter) ([]Bible, error) {
	q := url.Values{"include-full-details": {"true"}}
	set(q, "language", f.Language)
	set(q, "abbreviation", f.Abbreviation)
	set(q, "name", f.Name)
	var out struct {
		Data []Bible `json:"data"`
	}
	_, err := c.get(ctx, "/bibles", q, &out)
	return out.Data, err
}

// Bible returns one Bible's details, including its copyright.
func (c *Client) Bible(ctx context.Context, id string) (Bible, error) {
	var out struct {
		Data Bible `json:"data"`
	}
	_, err := c.get(ctx, "/bibles/"+url.PathEscape(id), nil, &out)
	return out.Data, err
}

// Books lists a Bible's books.
func (c *Client) Books(ctx context.Context, bibleID string) ([]Book, error) {
	var out struct {
		Data []Book `json:"data"`
	}
	_, err := c.get(ctx, "/bibles/"+url.PathEscape(bibleID)+"/books", nil, &out)
	return out.Data, err
}

// PassageOptions control the returned text.
type PassageOptions struct {
	ContentType  string // "text" (default) or "html"
	IncludeNotes bool
}

// Passage is Scripture text with everything that must be shown or reported with it.
type Passage struct {
	BibleID    string
	Reference  string
	Content    string
	VerseCount int
	Copyright  string
	FUMS       []Meta // one per request made; report each token with every display
	Cached     bool
}

type passageData struct {
	ID         string `json:"id"`
	BibleID    string `json:"bibleId"`
	Content    string `json:"content"`
	Reference  string `json:"reference"`
	VerseCount int    `json:"verseCount"`
	Copyright  string `json:"copyright"`
}

type passageResponse struct {
	Data passageData `json:"data"`
	Meta Meta        `json:"meta"`
}

// Passage fetches the text of a Xegesis range from one Bible.
func (c *Client) Passage(ctx context.Context, bibleID string, r verse.Range, o PassageOptions) (*Passage, error) {
	targets, err := Targets(r)
	if err != nil {
		return nil, err
	}
	q := url.Values{
		"content-type":          {"text"},
		"include-notes":         {strconv.FormatBool(o.IncludeNotes)},
		"include-titles":        {"false"},
		"include-verse-numbers": {"true"},
	}
	if o.ContentType != "" {
		q.Set("content-type", o.ContentType)
	}
	p := &Passage{BibleID: bibleID, Reference: r.String(), Cached: true}
	var parts []string
	for _, t := range targets {
		kind := "/passages/"
		if t.Chapter {
			kind = "/chapters/"
		}
		var out passageResponse
		cached, err := c.get(ctx, "/bibles/"+url.PathEscape(bibleID)+kind+url.PathEscape(t.ID), q, &out)
		if err != nil {
			return nil, err
		}
		p.Cached = p.Cached && cached
		parts = append(parts, strings.TrimSpace(out.Data.Content))
		p.VerseCount += out.Data.VerseCount
		if p.Copyright == "" {
			p.Copyright = out.Data.Copyright
		}
		if out.Meta != (Meta{}) {
			p.FUMS = append(p.FUMS, out.Meta)
		}
	}
	p.Content = strings.Join(parts, "\n\n")
	return p, nil
}

// SearchOptions control a search.
type SearchOptions struct {
	Query  string
	Limit  int
	Offset int
	Sort   string // relevance, canonical, reverse-canonical
}

type Verse struct {
	ID        string `json:"id"`
	BookID    string `json:"bookId"`
	ChapterID string `json:"chapterId"`
	Text      string `json:"text"`
	Reference string `json:"reference"`
}

type SearchResult struct {
	Query      string
	Total      int
	VerseCount int
	Verses     []Verse
	Passages   []Passage
	FUMS       []Meta
}

// Search looks for words or a reference in one Bible. Searches are never cached.
func (c *Client) Search(ctx context.Context, bibleID string, o SearchOptions) (*SearchResult, error) {
	q := url.Values{"query": {o.Query}}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}
	if o.Offset > 0 {
		q.Set("offset", strconv.Itoa(o.Offset))
	}
	set(q, "sort", o.Sort)
	var out struct {
		Data struct {
			Total      int           `json:"total"`
			VerseCount int           `json:"verseCount"`
			Verses     []Verse       `json:"verses"`
			Passages   []passageData `json:"passages"`
		} `json:"data"`
		Meta Meta `json:"meta"`
	}
	if _, err := c.do(ctx, "/bibles/"+url.PathEscape(bibleID)+"/search", q, &out, false); err != nil {
		return nil, err
	}
	res := &SearchResult{Query: o.Query, Total: out.Data.Total, VerseCount: out.Data.VerseCount, Verses: out.Data.Verses}
	for _, p := range out.Data.Passages {
		res.Passages = append(res.Passages, Passage{BibleID: p.BibleID, Reference: p.Reference, Content: p.Content,
			VerseCount: p.VerseCount, Copyright: p.Copyright})
	}
	if out.Meta != (Meta{}) {
		res.FUMS = []Meta{out.Meta}
	}
	return res, nil
}

// ReportFUMS tells API.Bible's usage-reporting service that Scripture was displayed, as the terms
// require. It sends the tokens with an anonymous device ID and session ID, and no user ID.
func (c *Client) ReportFUMS(ctx context.Context, deviceID, sessionID string, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	q := url.Values{"t": tokens, "dId": {deviceID}, "sId": {sessionID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.FUMSURL+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("apibible: usage reporting returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Verify makes one cheap request to check that the key works.
func (c *Client) Verify(ctx context.Context) error {
	q := url.Values{"language": {"eng"}, "abbreviation": {"KJV"}}
	_, err := c.do(ctx, "/bibles", q, &struct{}{}, false)
	return err
}

func set(q url.Values, k, v string) {
	if v != "" {
		q.Set(k, v)
	}
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) (cached bool, err error) {
	return c.do(ctx, path, q, out, true)
}

func (c *Client) do(ctx context.Context, path string, q url.Values, out any, useCache bool) (bool, error) {
	if c.Key == "" {
		return false, ErrNoKey
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	useCache = useCache && c.Cache != nil
	if useCache {
		if body, ok := c.Cache.get(u); ok {
			return true, json.Unmarshal(body, out)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("api-key", c.Key)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return false, err
	}
	if resp.StatusCode != http.StatusOK {
		return false, apiError(resp, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return false, fmt.Errorf("apibible: unreadable response: %w", err)
	}
	if useCache {
		c.Cache.put(u, body)
	}
	return false, nil
}

func apiError(resp *http.Response, body []byte) error {
	e := &APIError{Status: resp.StatusCode}
	var b struct {
		Message any `json:"message"`
		Error   any `json:"error"`
	}
	if json.Unmarshal(body, &b) == nil {
		if s, ok := b.Message.(string); ok {
			e.Message = s
		} else if s, ok := b.Error.(string); ok {
			e.Message = s
		}
	}
	if e.kind = kindFor(resp.StatusCode); e.kind == nil {
		e.kind = fmt.Errorf("API.Bible request failed")
	}
	if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
		e.RetryAfter = time.Duration(secs) * time.Second
	}
	return e
}
