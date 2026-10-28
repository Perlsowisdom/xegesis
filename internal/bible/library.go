package bible

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// ErrNoModule is returned for a module that is not installed.
var ErrNoModule = errors.New("bible: no such module")

// Library is a folder of module files. Modules open on first use and stay open until removed.
type Library struct {
	dir  string
	mu   sync.Mutex
	open map[string]*Module
}

func NewLibrary(dir string) *Library { return &Library{dir: dir, open: map[string]*Module{}} }

var validID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func (l *Library) path(id string) string { return filepath.Join(l.dir, id+".db") }

// Get returns an open module.
func (l *Library) Get(id string) (*Module, error) {
	if !validID.MatchString(id) {
		return nil, fmt.Errorf("%w %q", ErrNoModule, id)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if m, ok := l.open[id]; ok {
		return m, nil
	}
	if _, err := os.Stat(l.path(id)); err != nil {
		return nil, fmt.Errorf("%w %q", ErrNoModule, id)
	}
	m, err := openModule(l.path(id))
	if err != nil {
		return nil, err
	}
	l.open[id] = m
	return m, nil
}

// List describes every installed module, sorted by ID. Files that cannot be read are skipped.
func (l *Library) List() ([]Meta, error) {
	entries, err := os.ReadDir(l.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Meta{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Meta{}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".db")
		if !ok || e.IsDir() {
			continue
		}
		if m, err := l.Get(id); err == nil {
			out = append(out, m.Meta)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Remove closes and deletes a module file.
func (l *Library) Remove(id string) error {
	if _, err := l.Get(id); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if m, ok := l.open[id]; ok {
		m.Close()
		delete(l.open, id)
	}
	return os.Remove(l.path(id))
}

// SetLicence records a new licence in an installed module.
func (l *Library) SetLicence(ctx context.Context, id string, lic licence.Licence) (Meta, error) {
	if _, err := l.Get(id); err != nil {
		return Meta{}, err
	}
	b, err := json.Marshal(lic)
	if err != nil {
		return Meta{}, err
	}
	l.mu.Lock()
	if m, ok := l.open[id]; ok {
		m.Close()
		delete(l.open, id)
	}
	err = updateLicence(ctx, l.path(id), string(b))
	l.mu.Unlock()
	if err != nil {
		return Meta{}, err
	}
	m, err := l.Get(id)
	if err != nil {
		return Meta{}, err
	}
	return m.Meta, nil
}

func updateLicence(ctx context.Context, path, lic string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `UPDATE module SET licence = ?`, lic)
	return err
}

// Close closes every open module.
func (l *Library) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, m := range l.open {
		m.Close()
		delete(l.open, id)
	}
}

// InstallOptions describe a module to create. The licence is the one the user stated.
type InstallOptions struct {
	Path          string // a file, or a folder of files
	Format        string // osis, usfm or usx; empty to detect from each file
	ID            string
	Name          string
	Abbreviation  string
	Language      string
	Versification string
	Source        string
	Version       string
	Licence       licence.Licence
	MappingFile   string // optional full versification table
	Replace       bool
}

// Install imports source files, a folder of them or a .zip archive of them into a new module and
// returns its description and any warnings.
func (l *Library) Install(ctx context.Context, o InstallOptions) (Meta, []string, error) {
	if strings.EqualFold(filepath.Ext(o.Path), ".zip") {
		dir, err := unzipTemp(o.Path)
		if err != nil {
			return Meta{}, nil, fmt.Errorf("bible: %s: %w", filepath.Base(o.Path), err)
		}
		defer os.RemoveAll(dir)
		o.Path = contentDir(dir)
	}
	b := newBuilder()
	hints, err := importPath(o.Path, o.Format, b)
	if err != nil {
		return Meta{}, nil, err
	}
	return l.finish(ctx, o, hints, b)
}

// finish turns what an importer collected into an installed module.
func (l *Library) finish(ctx context.Context, o InstallOptions, hints Doc, b *builder) (Meta, []string, error) {
	d := b.doc()
	if len(d.Verses) == 0 {
		return Meta{}, d.Warnings, fmt.Errorf("bible: no verses found in %s", o.Path)
	}
	m, err := o.meta(hints, d)
	if err != nil {
		return Meta{}, d.Warnings, err
	}
	var maps []Mapping
	if o.MappingFile != "" {
		if maps, err = readMapping(o.MappingFile); err != nil {
			return Meta{}, d.Warnings, err
		}
	}
	if err := l.place(ctx, m, d.Verses, maps, o.Replace); err != nil {
		return Meta{}, d.Warnings, err
	}
	return m, d.Warnings, nil
}

func (l *Library) place(ctx context.Context, m Meta, verses []Verse, maps []Mapping, replace bool) error {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := os.Stat(l.path(m.ID)); err == nil && !replace {
		return fmt.Errorf("bible: module %q is already installed", m.ID)
	}
	if old, ok := l.open[m.ID]; ok {
		old.Close()
		delete(l.open, m.ID)
	}
	return write(ctx, l.path(m.ID), m, verses, maps)
}

func (o InstallOptions) meta(hints Doc, d *Doc) (Meta, error) {
	m := Meta{Kind: "bible", Name: o.Name, Abbreviation: o.Abbreviation, Language: o.Language,
		Versification: strings.ToLower(o.Versification), Licence: o.Licence, Source: o.Source,
		Version: o.Version, VerseCount: len(d.Verses), CreatedAt: now()}
	if m.Name == "" {
		m.Name = hints.Name
	}
	if m.Name == "" {
		return m, errors.New("bible: the source has no title; give the module a name")
	}
	if m.Language == "" {
		m.Language = hints.Language
	}
	if m.Language == "" {
		m.Language = "und"
	}
	if !validLang.MatchString(m.Language) {
		return m, fmt.Errorf("bible: %q is not a BCP 47 language tag", m.Language)
	}
	if m.Versification == "" {
		m.Versification = string(verse.English)
	}
	m.ID = o.ID
	if m.ID == "" {
		m.ID = slug(firstNonEmpty(m.Abbreviation, m.Name))
	}
	if !validID.MatchString(m.ID) {
		return m, fmt.Errorf("bible: module ID %q must be lowercase letters, digits, - or _", m.ID)
	}
	if m.Abbreviation == "" {
		m.Abbreviation = strings.ToUpper(m.ID)
	}
	m.Books = booksOf(d.Verses)
	return m, nil
}

var validLang = regexp.MustCompile(`^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$`)

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func slug(s string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			if out.Len() > 0 {
				out.WriteByte('-')
			}
		}
	}
	return strings.Trim(out.String(), "-")
}

func booksOf(verses []Verse) []string {
	var out []string
	last := 0
	for _, v := range verses {
		if n := v.ID.Book(); n != last {
			b, _ := verse.BookByNumber(n)
			out = append(out, b.OSIS)
			last = n
		}
	}
	return out
}

// importPath reads a file or every source file in a folder.
func importPath(path, format string, b *builder) (Doc, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Doc{}, err
	}
	files := []string{path}
	if info.IsDir() {
		files = nil
		entries, err := os.ReadDir(path)
		if err != nil {
			return Doc{}, err
		}
		for _, e := range entries {
			if !e.IsDir() && detect(e.Name(), nil) != "" {
				files = append(files, filepath.Join(path, e.Name()))
			}
		}
		if len(files) == 0 {
			return Doc{}, fmt.Errorf("bible: no OSIS, USFM or USX files in %s", path)
		}
	}
	var hints Doc
	for _, f := range files {
		h, err := importFile(f, format, b)
		if err != nil {
			return Doc{}, err
		}
		hints.Name = firstNonEmpty(hints.Name, h.Name)
		hints.Language = firstNonEmpty(hints.Language, h.Language)
	}
	return hints, nil
}

func importFile(path, format string, b *builder) (Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Doc{}, err
	}
	data = bytes.TrimPrefix(data, []byte("ï»¿"))
	if format == "" {
		format = detect(path, data)
	}
	switch format {
	case "osis":
		return readOSIS(bytes.NewReader(data), b)
	case "usx":
		return Doc{}, readUSX(bytes.NewReader(data), b)
	case "usfm":
		readUSFM(string(data), b)
		return Doc{}, nil
	}
	return Doc{}, fmt.Errorf("bible: cannot tell the format of %s", filepath.Base(path))
}

// detect guesses a file's format from its extension and, for XML, its root element.
func detect(name string, data []byte) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".usfm", ".sfm", ".ptx":
		return "usfm"
	case ".usx":
		return "usx"
	case ".osis":
		return "osis"
	case ".xml":
		if data == nil {
			return "xml"
		}
		head := string(data[:min(len(data), 2048)])
		if strings.Contains(head, "<usx") {
			return "usx"
		}
		return "osis"
	}
	return ""
}

// readMapping reads a versification table: lines "system<TAB>module ref<TAB>system ref", with OSIS
// references. Blank lines and lines starting with # are ignored.
func readMapping(path string) ([]Mapping, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Mapping
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		p := strings.Split(line, "\t")
		if len(p) != 3 {
			return nil, fmt.Errorf("bible: %s line %d: want 3 tab-separated fields", filepath.Base(path), n)
		}
		mod, err1 := osisVerse(p[1])
		sys, err2 := osisVerse(p[2])
		if err := errors.Join(err1, err2); err != nil {
			return nil, fmt.Errorf("bible: %s line %d: %v", filepath.Base(path), n, err)
		}
		out = append(out, Mapping{System: verse.System(strings.ToLower(p[0])), Module: mod, SystemID: sys})
	}
	return out, sc.Err()
}
