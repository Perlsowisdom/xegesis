package repos

import (
	"bytes"
	"context"
	"sort"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/command"
)

// Register adds the catalogue commands, caching catalogues in dir and installing into lib.
func Register(r *command.Registry, dir string, lib *bible.Library, opts ...Option) *Service {
	s := New(dir, lib, opts...)
	r.Add(
		command.Define("bible.sources", "List the Bible catalogues (CrossWire SWORD repositories and eBible.org), "+
			"turn them on or off, and refresh them. They are off until turned on; refreshing downloads the catalogues.", false, s.sources),
		command.Define("bible.available", "List Bibles the catalogues offer, from the cached catalogues (works offline). "+
			"Marks installed, locked and unsupported items.", false, s.available),
		command.Define("bible.get", "Download and install a Bible from a catalogue. With preview, report its size and "+
			"licence without downloading the text.", false, s.getCmd),
		command.Define("bible.progress", "Show downloads and installs in progress, for progress bars.", false, s.progressCmd),
		command.Define("bible.updates", "List installed Bibles whose catalogue offers a newer version (from the cache).", false, s.updates),
	)
	return s
}

type SourcesParams struct {
	Enable  *bool  `json:"enable,omitempty" desc:"Turn the Bible catalogues on (true) or off (false)"`
	Refresh bool   `json:"refresh,omitempty" desc:"Download the repository list and the catalogues again"`
	Source  string `json:"source,omitempty" desc:"Refresh only this source"`
}

type SourcesResult struct {
	Enabled bool     `json:"enabled" desc:"Whether the catalogues may use the network"`
	Sources []Source `json:"sources"`
}

func (s *Service) sources(ctx context.Context, _ *command.Env, p SourcesParams) (SourcesResult, error) {
	if p.Enable != nil {
		if err := s.setEnabled(*p.Enable); err != nil {
			return SourcesResult{}, err
		}
	}
	if p.Refresh {
		if err := s.online(); err != nil {
			return SourcesResult{}, err
		}
		if err := s.refresh(ctx, p.Source); err != nil {
			return SourcesResult{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.load()
	if err != nil {
		return SourcesResult{}, err
	}
	return SourcesResult{Enabled: st.Enabled, Sources: append([]Source{}, st.Sources...)}, nil
}

func (s *Service) setEnabled(on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.load()
	if err != nil {
		return err
	}
	st.Enabled = on
	return s.save(st)
}

// refresh downloads the master repository list (when refreshing everything) and the catalogues.
// A source that fails keeps its previous cache and records the error.
func (s *Service) refresh(ctx context.Context, only string) error {
	s.mu.Lock()
	st, err := s.load()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	list := append([]Source{}, st.Sources...)
	s.mu.Unlock()

	if only == "" {
		if data, err := s.get(ctx, s.masterURL, maxCatalogue); err == nil {
			list = mergeSources(list, parseMasterList(bytes.NewReader(data)))
		} else if ctx.Err() != nil {
			return ctx.Err()
		}
	} else if !hasSource(list, only) {
		return command.Errorf("not_found", "no catalogue %q", only)
	}
	for i := range list {
		if only != "" && list[i].ID != only {
			continue
		}
		data, err := s.get(ctx, s.catalogueURL(list[i]), maxCatalogue)
		if err == nil {
			list[i].Items, err = s.writeCache(list[i], data)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			list[i].Error = err.Error()
			continue
		}
		list[i].Error, list[i].Refreshed = "", s.now().UnixMilli()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	st, err = s.load()
	if err != nil {
		return err
	}
	st.Sources = list
	return s.save(st)
}

func hasSource(list []Source, id string) bool {
	for _, s := range list {
		if s.ID == id {
			return true
		}
	}
	return false
}

// mergeSources keeps known sources (and their refresh results) and adds new SWORD repositories
// from the master list. A repository that shares the URL of a known source is not added twice.
func mergeSources(known, master []Source) []Source {
	out := append([]Source{}, known...)
	for _, m := range master {
		dup := false
		for _, k := range known {
			if k.ID == m.ID || strings.TrimRight(k.URL, "/") == strings.TrimRight(m.URL, "/") {
				dup = true
			}
		}
		if !dup {
			out = append(out, m)
		}
	}
	return out
}

type AvailableParams struct {
	Language  string `json:"language,omitempty" desc:"BCP 47 language, e.g. en or de; matches regional variants too"`
	Kind      string `json:"kind,omitempty" enum:"bible,commentary,dictionary,book" desc:"Kind of text (default bible)"`
	Text      string `json:"text,omitempty" desc:"Words to find in the name, abbreviation, ID or description"`
	Licence   string `json:"licence,omitempty" enum:"public_domain,creative_commons,open_other,personal_purchase,own_work,unknown" desc:"Only items with this licence type"`
	Source    string `json:"source,omitempty" desc:"Only items from this source"`
	Supported bool   `json:"supported,omitempty" desc:"Only items Xegesis can install"`
	Limit     int    `json:"limit,omitempty" desc:"Most items to return (default 200)"`
	Offset    int    `json:"offset,omitempty"`
}

type Language struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type AvailableResult struct {
	Items     []Item     `json:"items"`
	Total     int        `json:"total" desc:"Matching items before limit and offset"`
	Languages []Language `json:"languages" desc:"Languages of the items of this kind, for a language picker"`
	Enabled   bool       `json:"enabled" desc:"Whether the catalogues may use the network"`
	Cached    bool       `json:"cached" desc:"False when no catalogue has been downloaded yet"`
}

func (s *Service) available(_ context.Context, _ *command.Env, p AvailableParams) (AvailableResult, error) {
	all, on, cached, err := s.allItems()
	if err != nil {
		return AvailableResult{}, err
	}
	if p.Kind == "" {
		p.Kind = "bible"
	}
	if p.Limit <= 0 {
		p.Limit = 200
	}
	out := AvailableResult{Items: []Item{}, Languages: []Language{}, Enabled: on, Cached: cached}
	langs := map[string]*Language{}
	text := strings.ToLower(strings.TrimSpace(p.Text))
	for _, it := range all {
		if it.Kind != p.Kind || (p.Source != "" && it.Source != p.Source) || (p.Supported && !it.Supported) {
			continue
		}
		base := baseLang(it.Language)
		if l := langs[base]; l != nil {
			l.Count++
		} else {
			langs[base] = &Language{Code: base, Name: baseName(it), Count: 1}
		}
		if p.Language != "" && baseLang(p.Language) != base {
			continue
		}
		if p.Licence != "" && string(it.Licence.Type) != p.Licence {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(it.Name+" "+it.Abbreviation+" "+it.ID+" "+it.About), text) {
			continue
		}
		out.Total++
		if out.Total > p.Offset && len(out.Items) < p.Limit {
			out.Items = append(out.Items, it)
		}
	}
	for _, l := range langs {
		out.Languages = append(out.Languages, *l)
	}
	sort.Slice(out.Languages, func(i, j int) bool { return out.Languages[i].Name < out.Languages[j].Name })
	return out, nil
}

func baseLang(tag string) string {
	b, _, _ := strings.Cut(strings.ToLower(tag), "-")
	return b
}

func baseName(it Item) string {
	if b, _, _ := strings.Cut(it.LanguageName, " ("); b != "" {
		return b
	}
	return it.LanguageName
}

// allItems returns every cached item, marked against the installed modules.
func (s *Service) allItems() ([]Item, bool, bool, error) {
	s.mu.Lock()
	st, err := s.load()
	var srcs []Source
	on := false
	if err == nil {
		srcs, on = append([]Source{}, st.Sources...), st.Enabled
	}
	s.mu.Unlock()
	if err != nil {
		return nil, false, false, err
	}
	installed, err := s.lib.List()
	if err != nil {
		return nil, false, false, err
	}
	bySource := map[string]bible.Meta{}
	for _, m := range installed {
		if m.Source != "" {
			bySource[m.Source] = m
		}
	}
	var all []Item
	cached := false
	for _, src := range srcs {
		items, err := s.items(src)
		if err != nil {
			continue // an unreadable cache is refreshed by bible.sources
		}
		cached = cached || items != nil
		for _, it := range items {
			if m, ok := bySource[it.Origin]; ok {
				it.Installed, it.Module, it.Update = true, m.ID, newer(it.Version, m.Version)
			}
			all = append(all, it)
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].LanguageName != all[j].LanguageName {
			return all[i].LanguageName < all[j].LanguageName
		}
		return strings.ToLower(all[i].Name) < strings.ToLower(all[j].Name)
	})
	return all, on, cached, nil
}

func (s *Service) find(source, id string) (Item, error) {
	all, _, _, err := s.allItems()
	if err != nil {
		return Item{}, err
	}
	for _, it := range all {
		if it.Source == source && it.ID == id {
			return it, nil
		}
	}
	return Item{}, command.Errorf("not_found", "catalogue %q has no item %q; refresh it with bible.sources", source, id)
}

type Update struct {
	Module           string `json:"module"`
	Name             string `json:"name"`
	InstalledVersion string `json:"installed_version"`
	Version          string `json:"version" desc:"The version the catalogue offers"`
	Source           string `json:"source"`
	Item             string `json:"item" desc:"Pass source and item to bible.get with replace to update"`
}

func (s *Service) updates(context.Context, *command.Env, struct{}) ([]Update, error) {
	all, _, _, err := s.allItems()
	if err != nil {
		return nil, err
	}
	installed, err := s.lib.List()
	if err != nil {
		return nil, err
	}
	version := map[string]string{}
	for _, m := range installed {
		version[m.ID] = m.Version
	}
	out := []Update{}
	for _, it := range all {
		if it.Update {
			out = append(out, Update{Module: it.Module, Name: it.Name, InstalledVersion: version[it.Module],
				Version: it.Version, Source: it.Source, Item: it.ID})
		}
	}
	return out, nil
}
