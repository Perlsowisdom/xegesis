package bible

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Register adds the Bible commands to r, serving modules from dir. None of them writes library.db.
func Register(r *command.Registry, dir string) { RegisterLibrary(r, NewLibrary(dir)) }

// RegisterLibrary adds the Bible commands for an existing Library.
func RegisterLibrary(r *command.Registry, l *Library) {
	c := cmds{l}
	r.Add(
		command.Define("bible.modules", "List the installed Bibles with their licences; with kind, the installed commentaries, "+
			"cross-reference sets, dictionaries or lexicons, or all modules.", false, c.modules),
		command.Define("bible.install", "Import an OSIS, USFM or USX file or folder, or a SWORD module folder or .zip (every module in it), as a new module. "+
			"Its licence is the one the file states, or Personal copy; give licence_type to set another.", false, c.install),
		command.Define("bible.set_licence", "Change the licence recorded for an installed Bible.", false, c.setLicence),
		command.Define("bible.remove", "Delete an installed Bible module.", false, c.remove),
		command.Define("bible.read", "Read a passage from one Bible.", false, c.read),
		command.Define("bible.parallel", "Read the same passage from several Bibles, matching verse numbering.", false, c.parallel),
		command.Define("bible.search", "Search the text of one or more Bibles, best matches first.", false, c.search),
	)
	registerReference(r, c)
}

type cmds struct{ l *Library }

func (c cmds) module(id string) (*Module, error) {
	m, err := c.l.Get(id)
	if errors.Is(err, ErrNoModule) {
		return nil, command.Errorf("not_found", "no Bible module %q", id)
	}
	return m, err
}

type ModulesParams struct {
	Kind string `json:"kind,omitempty" enum:"bible,commentary,crossref,dictionary,lexicon,all" desc:"Kind of module (default bible)"`
}

func (c cmds) modules(_ context.Context, _ *command.Env, p ModulesParams) ([]Meta, error) {
	all, err := c.l.List()
	if err != nil || p.Kind == "all" {
		return all, err
	}
	out := []Meta{}
	for _, m := range all {
		if m.Kind == firstNonEmpty(p.Kind, KindBible) {
			out = append(out, m)
		}
	}
	return out, nil
}

type InstallParams struct {
	Path          string `json:"path" required:"true" desc:"An OSIS, USFM or USX file, a folder of them, or a .zip archive of them; or a SWORD module folder or .zip (holding mods.d/*.conf)"`
	Format        string `json:"format,omitempty" enum:"osis,usfm,usx" desc:"Source format; detected from the file when omitted"`
	ID            string `json:"id,omitempty" desc:"Module ID (lowercase letters, digits, - and _); made from the abbreviation when omitted"`
	Name          string `json:"name,omitempty" desc:"Full name; taken from the source when omitted"`
	Abbreviation  string `json:"abbreviation,omitempty" desc:"Short name, e.g. KJV"`
	Language      string `json:"language,omitempty" desc:"BCP 47 language tag; taken from the source when omitted"`
	Versification string `json:"versification,omitempty" desc:"Verse numbering of the text: english (default) or hebrew"`
	Source        string `json:"source,omitempty" desc:"Where the text came from, e.g. a URL"`
	Version       string `json:"version,omitempty" desc:"Edition or version of the text"`
	MappingFile   string `json:"mapping_file,omitempty" desc:"Optional versification table: lines \"system<TAB>module OSIS ref<TAB>system OSIS ref\""`
	Replace       bool   `json:"replace,omitempty" desc:"Replace a module with the same ID"`

	LicenceType  string `json:"licence_type,omitempty" enum:"public_domain,creative_commons,open_other,personal_purchase,own_work,unknown" desc:"Leave out to use the licence the file states, or Personal copy when it states none"`
	SPDX         string `json:"spdx,omitempty" desc:"SPDX or Creative Commons ID, e.g. CC-BY-SA-4.0"`
	RightsHolder string `json:"rights_holder,omitempty"`
	LicenceURL   string `json:"licence_url,omitempty"`
	Attribution  string `json:"attribution,omitempty" desc:"Credit line to show with copies"`
}

type InstallResult struct {
	Module   Meta     `json:"module"`
	Modules  []Meta   `json:"modules,omitempty" desc:"Every module installed, when an archive holds several"`
	Warnings []string `json:"warnings" desc:"Markup that was skipped or could not be placed"`
}

func (c cmds) install(ctx context.Context, _ *command.Env, p InstallParams) (InstallResult, error) {
	var lic licence.Licence
	if p.LicenceType != "" {
		var err error
		if lic, err = userLicence(LicenceParams{Type: p.LicenceType, SPDX: p.SPDX, RightsHolder: p.RightsHolder,
			URL: p.LicenceURL, Attribution: p.Attribution}); err != nil {
			return InstallResult{}, err
		}
	}
	if strings.EqualFold(filepath.Ext(p.Path), ".zip") {
		dir, err := unzipTemp(p.Path)
		if err != nil {
			return InstallResult{}, command.Errorf("bad_params", "%s: %v", p.Path, err)
		}
		defer os.RemoveAll(dir)
		p.Path = contentDir(dir)
	}
	if root := SWORDRoot(p.Path); root != "" {
		var user *licence.Licence
		if p.LicenceType != "" {
			user = &lic
		}
		res, err := c.installSWORDFolder(ctx, root, p, user)
		if err != nil {
			return InstallResult{}, command.Errorf("failed", "%v", err)
		}
		return res, nil
	}
	if p.LicenceType == "" {
		lic = DetectLicence(p.Path)
	}
	m, warnings, err := c.l.Install(ctx, InstallOptions{Path: p.Path, Format: p.Format, ID: p.ID, Name: p.Name,
		Abbreviation: p.Abbreviation, Language: p.Language, Versification: p.Versification, Source: p.Source,
		Version: p.Version, Licence: lic, MappingFile: p.MappingFile, Replace: p.Replace})
	if warnings == nil {
		warnings = []string{}
	}
	if err != nil {
		return InstallResult{}, command.Errorf("failed", "%v", err)
	}
	return InstallResult{Module: m, Warnings: warnings}, nil
}

type LicenceParams struct {
	Type         string `json:"type" required:"true" enum:"public_domain,creative_commons,open_other,personal_purchase,own_work,unknown"`
	SPDX         string `json:"spdx,omitempty" desc:"SPDX or Creative Commons ID, e.g. CC-BY-SA-4.0"`
	RightsHolder string `json:"rights_holder,omitempty"`
	URL          string `json:"licence_url,omitempty"`
	Attribution  string `json:"attribution,omitempty" desc:"Credit line to show with copies"`
	Share        *bool  `json:"share_allowed,omitempty" desc:"Override whether copies may be given to others"`
}

type SetLicenceParams struct {
	Module string `json:"module" required:"true" desc:"Module ID from bible.modules"`
	LicenceParams
}

func userLicence(p LicenceParams) (licence.Licence, error) {
	l, err := licence.Defaults(licence.Type(p.Type), p.SPDX)
	if err != nil {
		return l, command.Errorf("bad_params", "%v; for other open licences use type open_other", err)
	}
	l.RightsHolder, l.URL, l.Attribution = p.RightsHolder, p.URL, p.Attribution
	if p.Share != nil {
		l.Share = *p.Share
	}
	l.Confirmed, l.Origin = true, licence.ByUser
	return l, nil
}

func (c cmds) setLicence(ctx context.Context, _ *command.Env, p SetLicenceParams) (Meta, error) {
	if _, err := c.module(p.Module); err != nil {
		return Meta{}, err
	}
	l, err := userLicence(p.LicenceParams)
	if err != nil {
		return Meta{}, err
	}
	m, err := c.l.SetLicence(ctx, p.Module, l)
	if err != nil {
		return Meta{}, command.Errorf("failed", "%v", err)
	}
	return m, nil
}

type ModuleParams struct {
	Module string `json:"module" required:"true" desc:"Module ID from bible.modules"`
}

func (c cmds) remove(_ context.Context, _ *command.Env, p ModuleParams) (struct{}, error) {
	if _, err := c.module(p.Module); err != nil {
		return struct{}{}, err
	}
	return struct{}{}, c.l.Remove(p.Module)
}

// parseRef reads a reference with verse.Parse, or a bare book name as the whole book.
func parseRef(ref string) (verse.Range, error) {
	r, err := verse.Parse(ref)
	if err == nil {
		return r, nil
	}
	if b, ok := verse.LookupBook(ref); ok {
		start := verse.ID(b.Number * 1_000_000)
		return verse.Range{Start: start, End: start + 999_999}, nil
	}
	return r, command.Errorf("bad_params", "%v", err)
}

type ReadParams struct {
	Module string `json:"module" required:"true" desc:"Module ID from bible.modules"`
	Ref    string `json:"ref" required:"true" desc:"A reference in the module's own numbering, e.g. \"John 3:16-18\", \"Ps 23\" or \"Jude\""`
	Markup bool   `json:"markup,omitempty" desc:"Include the marked-up text (headings, notes, words of Christ, Strong's)"`
}

// ReadVerse is a verse as returned to callers.
type ReadVerse struct {
	ID     verse.ID `json:"id"`
	End    verse.ID `json:"end,omitempty" desc:"Last verse when several are combined"`
	Ref    string   `json:"ref"`
	BaseID verse.ID `json:"base_id,omitempty" desc:"The verse in the requested numbering, for lining up parallel columns"`
	Text   string   `json:"text"`
	Markup string   `json:"markup,omitempty"`
}

type Passage struct {
	Module      string      `json:"module"`
	Ref         string      `json:"ref" desc:"The passage in this module's numbering"`
	Attribution string      `json:"attribution,omitempty"`
	Verses      []ReadVerse `json:"verses"`
}

func (c cmds) read(ctx context.Context, _ *command.Env, p ReadParams) (Passage, error) {
	m, err := c.module(p.Module)
	if err != nil {
		return Passage{}, err
	}
	r, err := parseRef(p.Ref)
	if err != nil {
		return Passage{}, err
	}
	return passage(ctx, m, r, "", p.Markup)
}

func passage(ctx context.Context, m *Module, r verse.Range, base verse.System, markup bool) (Passage, error) {
	vs, err := m.Read(ctx, r)
	if err != nil {
		return Passage{}, err
	}
	out := Passage{Module: m.ID, Ref: r.String(), Attribution: m.Licence.AttributionLine(), Verses: []ReadVerse{}}
	for _, v := range vs {
		rv := ReadVerse{ID: v.ID, Ref: refString(v.ID, v.End), Text: v.Text}
		if v.End != v.ID {
			rv.End = v.End
		}
		if markup {
			rv.Markup = v.Markup
		}
		if base != "" {
			rv.BaseID = m.FromModule(ctx, v.ID, base)
		}
		out.Verses = append(out.Verses, rv)
	}
	return out, nil
}

type ParallelParams struct {
	Modules       []string `json:"modules" required:"true" desc:"Module IDs, in column order"`
	Ref           string   `json:"ref" required:"true" desc:"A reference in the numbering given by versification"`
	Versification string   `json:"versification,omitempty" desc:"Numbering of ref: english (default) or hebrew"`
	Markup        bool     `json:"markup,omitempty"`
}

type ParallelResult struct {
	Ref     string    `json:"ref"`
	Columns []Passage `json:"columns"`
}

func (c cmds) parallel(ctx context.Context, _ *command.Env, p ParallelParams) (ParallelResult, error) {
	r, err := parseRef(p.Ref)
	if err != nil {
		return ParallelResult{}, err
	}
	base := system(p.Versification)
	out := ParallelResult{Ref: r.String(), Columns: []Passage{}}
	for _, id := range p.Modules {
		m, err := c.module(id)
		if err != nil {
			return ParallelResult{}, err
		}
		col, err := passage(ctx, m, m.ToModuleRange(ctx, r, base), base, p.Markup)
		if err != nil {
			return ParallelResult{}, err
		}
		out.Columns = append(out.Columns, col)
	}
	return out, nil
}

func system(s string) verse.System {
	if s == "" {
		return verse.English
	}
	return verse.System(strings.ToLower(s))
}

type SearchParams struct {
	Query         string   `json:"query,omitempty" desc:"Words to find"`
	Mode          string   `json:"mode,omitempty" enum:"all,any,phrase,fts" desc:"all words (default), any word, the exact phrase, or raw FTS5 syntax"`
	Strong        string   `json:"strong,omitempty" desc:"Find verses tagged with this Strong's number instead, e.g. G26"`
	Modules       []string `json:"modules,omitempty" desc:"Module IDs to search; all installed modules when omitted"`
	Ref           string   `json:"ref,omitempty" desc:"Only verses in this reference, e.g. \"Rom\" or \"John 3-5\""`
	Versification string   `json:"versification,omitempty" desc:"Numbering of ref: english (default) or hebrew"`
	Limit         int      `json:"limit,omitempty" desc:"Most results to return (default 50)"`
}

func (c cmds) search(ctx context.Context, _ *command.Env, p SearchParams) ([]Hit, error) {
	if strings.TrimSpace(p.Query) == "" && p.Strong == "" {
		return nil, command.Errorf("bad_params", "bible.search: query or strong is required")
	}
	if p.Limit <= 0 {
		p.Limit = 50
	}
	r := verse.Range{Start: 0, End: 999_999_999}
	if p.Ref != "" {
		var err error
		if r, err = parseRef(p.Ref); err != nil {
			return nil, err
		}
	}
	ids, err := c.searchModules(p.Modules)
	if err != nil {
		return nil, err
	}
	hits := []Hit{}
	for _, id := range ids {
		m, err := c.module(id)
		if err != nil {
			return nil, err
		}
		h, err := searchOne(ctx, m, p, c.scope(ctx, m, r, p))
		if err != nil {
			return nil, command.Errorf("bad_params", "bible.search: %v", err)
		}
		hits = append(hits, h...)
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > p.Limit {
		hits = hits[:p.Limit]
	}
	return hits, nil
}

func (c cmds) scope(ctx context.Context, m *Module, r verse.Range, p SearchParams) verse.Range {
	if p.Ref == "" {
		return r
	}
	return m.ToModuleRange(ctx, r, system(p.Versification))
}

func searchOne(ctx context.Context, m *Module, p SearchParams, r verse.Range) ([]Hit, error) {
	if p.Strong != "" {
		n := normStrong(p.Strong)
		if n == "" {
			return nil, errors.New("strong must look like H7225 or G26")
		}
		return m.SearchStrong(ctx, n, r, p.Limit)
	}
	return m.Search(ctx, ftsQuery(p.Query, p.Mode), r, p.Limit)
}

func (c cmds) searchModules(ids []string) ([]string, error) {
	if len(ids) > 0 {
		return ids, nil
	}
	all, err := c.l.List()
	if err != nil {
		return nil, err
	}
	for _, m := range all {
		if m.Kind == KindBible {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

// ftsQuery turns plain words into an FTS5 query, quoting each word so punctuation is safe.
func ftsQuery(q, mode string) string {
	if mode == "fts" {
		return q
	}
	words := strings.FieldsFunc(q, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '"' || r == ',' || r == ';' || r == ':'
	})
	for i, w := range words {
		words[i] = `"` + w + `"`
	}
	switch mode {
	case "any":
		return strings.Join(words, " OR ")
	case "phrase":
		return `"` + strings.Join(strings.Fields(strings.ReplaceAll(q, `"`, "")), " ") + `"`
	}
	return strings.Join(words, " ")
}
