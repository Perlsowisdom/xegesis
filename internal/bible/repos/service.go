// Package repos fetches and caches catalogues of Bibles (CrossWire SWORD repositories and
// eBible.org) and installs texts from them. It uses the network only when the user has turned the
// Bible catalogues on (ADR 0009) and only for the requests docs/privacy.md lists; everything else
// works offline from the cache.
package repos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/command"
)

const (
	DefaultMasterList = "https://crosswire.org/ftpmirror/pub/sword/masterRepoList.conf"
	DefaultCrossWire  = "https://crosswire.org/ftpmirror/pub/sword/raw"
	DefaultEBible     = "https://ebible.org"
	userAgent         = "Xegesis"
	maxCatalogue      = 64 << 20
	maxDownload       = 1 << 30
)

// Service holds the catalogue cache and installs into a Library.
type Service struct {
	dir        string
	lib        *bible.Library
	http       *http.Client
	masterURL  string
	crosswire  string
	ebible     string
	stepbible  string
	openbible  string
	now        func() time.Time
	mu         sync.Mutex // guards the state file, memo and progress
	memo       map[string]memoEntry
	progress   map[string]*Progress
	stateCache *state
}

type memoEntry struct {
	mod   time.Time
	items []Item
	err   error
}

// Option customises a Service, mainly for tests.
type Option func(*Service)

func WithHTTPClient(c *http.Client) Option { return func(s *Service) { s.http = c } }

// WithURLs replaces the master repository list, the default SWORD repository and eBible.org.
func WithURLs(master, crosswire, ebible string) Option {
	return func(s *Service) { s.masterURL, s.crosswire, s.ebible = master, crosswire, ebible }
}

// WithDataURLs replaces the STEPBible and OpenBible.info download folders.
func WithDataURLs(stepbible, openbible string) Option {
	return func(s *Service) { s.stepbible, s.openbible = stepbible, openbible }
}

// New makes a Service caching catalogues in dir and installing into lib.
func New(dir string, lib *bible.Library, opts ...Option) *Service {
	s := &Service{dir: dir, lib: lib, http: &http.Client{Timeout: 10 * time.Minute},
		masterURL: DefaultMasterList, crosswire: DefaultCrossWire, ebible: DefaultEBible,
		stepbible: DefaultSTEPBible, openbible: DefaultOpenBible,
		now: time.Now, memo: map[string]memoEntry{}, progress: map[string]*Progress{}}
	for _, o := range opts {
		o(s)
	}
	return s
}

// state is what the cache folder remembers: whether the user turned the catalogues on, and the
// sources with their refresh results.
type state struct {
	Enabled bool     `json:"enabled"`
	Sources []Source `json:"sources"`
}

func (s *Service) defaults() []Source {
	return append([]Source{
		{ID: "crosswire", Name: "CrossWire", Kind: KindSWORD, URL: s.crosswire},
		{ID: "ebible", Name: "eBible.org", Kind: KindEBible, URL: s.ebible},
	}, s.dataSources()...)
}

func (s *Service) statePath() string { return filepath.Join(s.dir, "state.json") }

// load returns the state; the caller holds s.mu.
func (s *Service) load() (*state, error) {
	if s.stateCache != nil {
		return s.stateCache, nil
	}
	st := &state{}
	data, err := os.ReadFile(s.statePath())
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, st); err != nil {
			return nil, fmt.Errorf("repos: %s: %w", s.statePath(), err)
		}
	}
	if len(st.Sources) == 0 {
		st.Sources = s.defaults()
	}
	for _, d := range s.dataSources() {
		if !hasSource(st.Sources, d.ID) {
			st.Sources = append(st.Sources, d)
		}
	}
	s.stateCache = st
	return st, nil
}

// save writes the state; the caller holds s.mu.
func (s *Service) save(st *state) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.statePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	s.stateCache = st
	return os.Rename(tmp, s.statePath())
}

func (s *Service) enabled() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.load()
	if err != nil {
		return false, err
	}
	return st.Enabled, nil
}

// errOff is returned by anything that would use the network while the catalogues are off.
func errOff() error {
	return command.Errorf("unavailable", "Bible catalogues are turned off; turn them on with "+
		"bible.sources {\"enable\": true} (see docs/privacy.md for what they send)")
}

func (s *Service) online() error {
	on, err := s.enabled()
	if err != nil {
		return err
	}
	if !on {
		return errOff()
	}
	return nil
}

// cacheFile is where a source's raw catalogue is kept.
func (s *Service) cacheFile(src Source) string {
	if src.Kind == KindEBible {
		return filepath.Join(s.dir, src.ID+".translations.csv")
	}
	return filepath.Join(s.dir, src.ID+".mods.d.tar.gz")
}

func (s *Service) catalogueURL(src Source) string {
	if src.Kind == KindEBible {
		return joinURL(src.URL, "Scriptures", "translations.csv")
	}
	return joinURL(src.URL, "mods.d.tar.gz")
}

// get fetches a URL into memory, refusing bodies over limit.
func (s *Service) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	resp, err := s.do(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", url, limit)
	}
	return data, nil
}

// errNotFound marks a 404, which for SWORD module files means a testament is absent.
var errNotFound = errors.New("not found")

func (s *Service) do(ctx context.Context, method, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%s: %w", url, errNotFound)
		}
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return resp, nil
}

// items reads a source's cached catalogue, parsing it again only when the file changed.
func (s *Service) items(src Source) ([]Item, error) {
	if src.Kind == KindData {
		return s.dataItems(src), nil
	}
	path := s.cacheFile(src)
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	m, ok := s.memo[src.ID]
	s.mu.Unlock()
	if ok && m.mod.Equal(info.ModTime()) {
		return m.items, m.err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	items, err := parse(src, data)
	s.mu.Lock()
	s.memo[src.ID] = memoEntry{mod: info.ModTime(), items: items, err: err}
	s.mu.Unlock()
	return items, err
}

func parse(src Source, data []byte) ([]Item, error) {
	if src.Kind == KindEBible {
		return parseEBible(src, data)
	}
	return parseSWORD(src, data)
}

// writeCache stores a downloaded catalogue after checking that it parses.
func (s *Service) writeCache(src Source, data []byte) (int, error) {
	items, err := parse(src, data)
	if err != nil {
		return 0, fmt.Errorf("the catalogue could not be read: %w", err)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return 0, err
	}
	path := s.cacheFile(src)
	if err := os.WriteFile(path+".tmp", data, 0o644); err != nil {
		return 0, err
	}
	s.mu.Lock()
	delete(s.memo, src.ID)
	s.mu.Unlock()
	return len(items), os.Rename(path+".tmp", path)
}
