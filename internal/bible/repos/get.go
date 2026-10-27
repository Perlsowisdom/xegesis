package repos

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/bible/sword"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

type GetParams struct {
	Source  string `json:"source" required:"true" desc:"Source ID from bible.sources"`
	Item    string `json:"item" required:"true" desc:"Item ID from bible.available"`
	Preview bool   `json:"preview,omitempty" desc:"Only report the download size and licence; nothing is installed"`
	ID      string `json:"id,omitempty" desc:"Module ID to install as; made from the item ID when omitted"`
	Replace bool   `json:"replace,omitempty" desc:"Replace the installed copy (for updates)"`

	AcceptLicence bool `json:"accept_licence,omitempty" desc:"The user has seen the licence and accepts it; recorded as confirmed_by_user"`

	// The user's own statement, needed when the catalogue's licence is unclear.
	LicenceType  string `json:"licence_type,omitempty" enum:"public_domain,creative_commons,open_other,personal_purchase,own_work,unknown" desc:"Licence the user states; overrides the catalogue's"`
	SPDX         string `json:"spdx,omitempty"`
	RightsHolder string `json:"rights_holder,omitempty"`
	LicenceURL   string `json:"licence_url,omitempty"`
	Attribution  string `json:"attribution,omitempty"`
}

type GetResult struct {
	Item     Item        `json:"item" desc:"The item, with its licence resolved"`
	Size     int64       `json:"size" desc:"Bytes to download (0 when the server does not say)"`
	Preview  bool        `json:"preview"`
	Module   *bible.Meta `json:"module,omitempty" desc:"The installed module"`
	Warnings []string    `json:"warnings"`
}

// Progress is one download or install under way.
type Progress struct {
	Source string `json:"source"`
	Item   string `json:"item"`
	Phase  string `json:"phase" desc:"downloading, checking or converting"`
	Done   int64  `json:"done" desc:"Bytes downloaded"`
	Total  int64  `json:"total" desc:"Bytes expected (0 when unknown)"`
}

func (s *Service) progressCmd(context.Context, *command.Env, struct{}) ([]Progress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Progress{}
	for _, p := range s.progress {
		out = append(out, *p)
	}
	return out, nil
}

func (s *Service) setProgress(key string, f func(*Progress)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.progress[key]; ok {
		f(p)
	}
}

func (s *Service) getCmd(ctx context.Context, _ *command.Env, p GetParams) (GetResult, error) {
	it, err := s.find(p.Source, p.Item)
	if err != nil {
		return GetResult{}, err
	}
	switch {
	case it.Locked:
		return GetResult{}, command.Errorf("unavailable", "%s is locked: it needs the publisher's unlock key, "+
			"which Xegesis does not support", it.Name)
	case !it.Supported:
		return GetResult{}, command.Errorf("unavailable", "%s cannot be installed: %s", it.Name, it.Reason)
	}
	if err := s.online(); err != nil {
		return GetResult{}, err
	}
	if it.conf == nil {
		s.resolveEBible(ctx, &it)
	}
	res := GetResult{Item: it, Preview: p.Preview, Warnings: []string{}}
	if res.Size, err = s.size(ctx, it); err != nil {
		return res, command.Errorf("failed", "%v", err)
	}
	if p.Preview {
		return res, nil
	}
	lic, err := chooseLicence(it, p)
	if err != nil {
		return res, err
	}
	id, err := s.moduleID(it, p)
	if err != nil {
		return res, err
	}
	key := it.Source + "/" + it.ID
	s.mu.Lock()
	if _, busy := s.progress[key]; busy {
		s.mu.Unlock()
		return res, command.Errorf("unavailable", "%s is already being installed", it.Name)
	}
	s.progress[key] = &Progress{Source: it.Source, Item: it.ID, Phase: "downloading", Total: res.Size}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.progress, key)
		s.mu.Unlock()
	}()

	opts := bible.InstallOptions{ID: id, Name: it.Name, Abbreviation: it.Abbreviation, Language: it.Language,
		Source: it.Origin, Version: it.Version, Licence: lic, Replace: p.Replace}
	var m bible.Meta
	var warnings []string
	if it.conf != nil {
		m, warnings, err = s.installSWORD(ctx, key, it, opts)
	} else {
		m, warnings, err = s.installEBible(ctx, key, it, res.Size, opts)
	}
	if err != nil {
		var ce *command.Error
		if errors.As(err, &ce) {
			return res, ce
		}
		return res, command.Errorf("failed", "%s: %v", it.Name, err)
	}
	if warnings != nil {
		res.Warnings = warnings
	}
	res.Module = &m
	return res, nil
}

// chooseLicence takes the user's statement when given, else the catalogue's mapping when clear.
func chooseLicence(it Item, p GetParams) (licence.Licence, error) {
	if p.LicenceType != "" {
		l, err := licence.Defaults(licence.Type(p.LicenceType), p.SPDX)
		if err != nil {
			return l, command.Errorf("bad_params", "%v", err)
		}
		l.RightsHolder = firstNonEmpty(p.RightsHolder, it.Licence.RightsHolder)
		l.URL = firstNonEmpty(p.LicenceURL, it.Licence.URL)
		l.Attribution = firstNonEmpty(p.Attribution, it.Licence.Attribution)
		l.Confirmed = true
		return l, nil
	}
	if !it.LicenceClear {
		return licence.Licence{}, command.Errorf("bad_params", "the licence of %s is unclear (the source says: %s); "+
			"state it with licence_type", it.Name, strings.ReplaceAll(it.Rights, "\n", "; "))
	}
	l := it.Licence
	l.Confirmed = p.AcceptLicence
	return l, nil
}

var notID = regexp.MustCompile(`[^a-z0-9_-]+`)

// moduleID picks the module ID: the given one, or the item's ID, with the source added when
// another source's module already has that ID.
func (s *Service) moduleID(it Item, p GetParams) (string, error) {
	if p.ID != "" {
		return p.ID, nil
	}
	id := strings.Trim(notID.ReplaceAllString(strings.ToLower(it.ID), "-"), "-")
	if len(id) > 48 {
		id = id[:48]
	}
	m, err := s.lib.Get(id)
	if errors.Is(err, bible.ErrNoModule) {
		return id, nil
	}
	if err != nil {
		return "", err
	}
	if m.Source == it.Origin {
		if !p.Replace {
			return "", command.Errorf("bad_params", "%s is already installed as %q; pass replace to update it", it.Name, id)
		}
		return id, nil
	}
	return id + "-" + it.Source, nil
}

// size asks the server for the download size when the catalogue does not give it.
func (s *Service) size(ctx context.Context, it Item) (int64, error) {
	if it.Size > 0 || it.conf != nil {
		return it.Size, nil
	}
	resp, err := s.do(ctx, http.MethodHead, s.ebibleZip(it))
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return max(resp.ContentLength, 0), nil
}

func (s *Service) ebibleZip(it Item) string {
	return joinURL(it.base, "Scriptures", it.ID+"_usfm.zip")
}

// resolveEBible reads a translation's copyright page for a Creative Commons licence when the
// catalogue only names the copyright holder. Failures leave the licence unclear.
func (s *Service) resolveEBible(ctx context.Context, it *Item) {
	if it.LicenceClear {
		return
	}
	page := joinURL(it.base, it.ID, "copr.htm")
	data, err := s.get(ctx, page, 1<<20)
	if err != nil {
		return
	}
	l, link, ok := ccFromPage(string(data))
	if !ok {
		it.Rights += "\nCopyright page: " + page + " (no Creative Commons licence found)"
		return
	}
	l.Attribution, l.RightsHolder = it.Licence.Attribution, it.Licence.RightsHolder
	it.Licence, it.LicenceClear = l, true
	it.Rights += "\nCopyright page: " + page + "\nLicence: " + link
}

// counter reports download progress.
type counter struct {
	r    io.Reader
	note func(int64)
	n    int64
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	c.note(c.n)
	return n, err
}

// download saves a URL to a file, adding its bytes to the progress count.
func (s *Service) download(ctx context.Context, key, url, dest string, base int64) (int64, error) {
	resp, err := s.do(ctx, http.MethodGet, url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	f, err := os.Create(dest)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	c := &counter{r: resp.Body, note: func(n int64) { s.setProgress(key, func(p *Progress) { p.Done = base + n }) }}
	n, err := io.Copy(f, io.LimitReader(c, maxDownload-base+1))
	if err == nil && base+n > maxDownload {
		err = fmt.Errorf("download is larger than %d bytes", int64(maxDownload))
	}
	if err == nil && resp.ContentLength >= 0 && n != resp.ContentLength {
		err = fmt.Errorf("%s: received %d of %d bytes", url, n, resp.ContentLength)
	}
	return n, err
}

func (s *Service) installSWORD(ctx context.Context, key string, it Item, o bible.InstallOptions) (bible.Meta, []string, error) {
	dir, err := os.MkdirTemp("", "xegesis-sword-*")
	if err != nil {
		return bible.Meta{}, nil, err
	}
	defer os.RemoveAll(dir)
	var total int64
	for _, name := range sword.DataFiles(*it.conf) {
		n, err := s.download(ctx, key, joinURL(it.base, it.conf.DataPath(), name), filepath.Join(dir, name), total)
		if errors.Is(err, errNotFound) {
			os.Remove(filepath.Join(dir, name))
			continue // this testament is not in the module
		}
		if err != nil {
			return bible.Meta{}, nil, err
		}
		total += n
	}
	s.setProgress(key, func(p *Progress) { p.Phase = "checking" })
	text, err := sword.Open(dir, *it.conf)
	if err != nil {
		return bible.Meta{}, nil, err
	}
	s.setProgress(key, func(p *Progress) { p.Phase = "converting" })
	o.Versification = string(text.System())
	return s.lib.InstallEntries(ctx, o, bible.EntryMarkup(it.conf.Get("SourceType")), func(emit func(bible.Entry) error) error {
		return text.Walk(func(id verse.ID, intro bool, markup string) error {
			return emit(bible.Entry{ID: id, Intro: intro, Markup: markup})
		})
	})
}

func (s *Service) installEBible(ctx context.Context, key string, it Item, size int64, o bible.InstallOptions) (bible.Meta, []string, error) {
	dir, err := os.MkdirTemp("", "xegesis-ebible-*")
	if err != nil {
		return bible.Meta{}, nil, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, it.ID+"_usfm.zip")
	if _, err := s.download(ctx, key, s.ebibleZip(it), path, 0); err != nil {
		return bible.Meta{}, nil, err
	}
	s.setProgress(key, func(p *Progress) { p.Phase = "checking" })
	if err := checkZip(path); err != nil {
		return bible.Meta{}, nil, err
	}
	s.setProgress(key, func(p *Progress) { p.Phase = "converting" })
	o.Path, o.Format = path, "usfm"
	return s.lib.Install(ctx, o)
}

// checkZip confirms a download is a zip archive holding USFM files.
func checkZip(path string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("the download is not a zip archive: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		switch strings.ToLower(filepath.Ext(f.Name)) {
		case ".usfm", ".sfm":
			return nil
		}
	}
	return errors.New("the download holds no USFM files")
}
