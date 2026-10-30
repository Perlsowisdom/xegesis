// Package bible reads and writes Bible modules: one read-only SQLite file per Bible,
// modules/<id>.db, described in docs/modules.md. Modules are made by importing OSIS, USFM or USX.
package bible

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// FormatVersion is the module schema version, stored in PRAGMA user_version.
const FormatVersion = 1

const schema = `
CREATE TABLE module (
    id            TEXT NOT NULL,
    kind          TEXT NOT NULL,
    name          TEXT NOT NULL,
    abbreviation  TEXT NOT NULL,
    language      TEXT NOT NULL,
    versification TEXT NOT NULL,
    licence       TEXT NOT NULL,
    source        TEXT NOT NULL,
    version       TEXT NOT NULL,
    books         TEXT NOT NULL,
    verse_count   INTEGER NOT NULL,
    created_at    INTEGER NOT NULL
);
CREATE TABLE verses (
    id     INTEGER PRIMARY KEY,
    end_id INTEGER NOT NULL,
    text   TEXT NOT NULL,
    markup TEXT NOT NULL
);
CREATE INDEX verses_end ON verses(end_id);
CREATE TABLE strongs (
    number   TEXT NOT NULL,
    verse_id INTEGER NOT NULL,
    PRIMARY KEY (number, verse_id)
) WITHOUT ROWID;
CREATE TABLE versification_map (
    system    TEXT NOT NULL,
    module_id INTEGER NOT NULL,
    system_id INTEGER NOT NULL,
    PRIMARY KEY (system, module_id)
) WITHOUT ROWID;
CREATE INDEX versification_map_system ON versification_map(system, system_id);
CREATE TABLE entries (
    key        TEXT NOT NULL,
    fold       TEXT NOT NULL,
    strong     TEXT NOT NULL,
    lemma      TEXT NOT NULL,
    lemma_fold TEXT NOT NULL,
    translit   TEXT NOT NULL,
    morph      TEXT NOT NULL,
    gloss      TEXT NOT NULL,
    text       TEXT NOT NULL,
    markup     TEXT NOT NULL
);
CREATE INDEX entries_fold ON entries(fold);
CREATE INDEX entries_strong ON entries(strong);
CREATE INDEX entries_lemma ON entries(lemma_fold);
CREATE VIRTUAL TABLE verses_fts USING fts5(text, content='verses', content_rowid='id',
    tokenize='unicode61 remove_diacritics 2');
`

// Meta describes a module.
type Meta struct {
	ID            string          `json:"id"`
	Kind          string          `json:"kind" desc:"bible, commentary, crossref, dictionary or lexicon"`
	Name          string          `json:"name"`
	Abbreviation  string          `json:"abbreviation"`
	Language      string          `json:"language" desc:"BCP 47 language tag"`
	Versification string          `json:"versification" desc:"Verse numbering, e.g. english or hebrew"`
	Licence       licence.Licence `json:"licence"`
	Source        string          `json:"source,omitempty" desc:"Where the text came from"`
	Version       string          `json:"version,omitempty"`
	Books         []string        `json:"books" desc:"OSIS IDs of the books present"`
	VerseCount    int             `json:"verse_count" desc:"Verses, or for a dictionary or lexicon its entries"`
	CreatedAt     int64           `json:"created_at"`
}

// Mapping is one row of a full versification table: this module's verse is the system's verse.
type Mapping struct {
	System   verse.System
	Module   verse.ID
	SystemID verse.ID
}

// write creates a module file at path from imported verses. It writes to a temporary file first.
func write(ctx context.Context, path string, m Meta, verses []Verse, maps []Mapping, heads ...Headword) error {
	tmp := path + ".tmp"
	os.Remove(tmp)
	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	err = fill(ctx, db, m, verses, maps, heads)
	if cerr := db.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func fill(ctx context.Context, db *sql.DB, m Meta, verses []Verse, maps []Mapping, heads []Headword) error {
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertMeta(ctx, tx, m); err != nil {
		return err
	}
	if err := insertVerses(ctx, tx, verses); err != nil {
		return err
	}
	if err := insertMaps(ctx, tx, maps); err != nil {
		return err
	}
	if err := insertHeadwords(ctx, tx, heads); err != nil {
		return err
	}
	stmts := []string{
		`INSERT INTO verses_fts(rowid, text) SELECT id, text FROM verses`,
		`INSERT INTO verses_fts(verses_fts) VALUES('optimize')`,
		fmt.Sprintf(`PRAGMA user_version = %d`, FormatVersion),
	}
	for _, q := range stmts {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertMeta(ctx context.Context, tx *sql.Tx, m Meta) error {
	lic, err := json.Marshal(m.Licence)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO module VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, m.ID, m.Kind, m.Name,
		m.Abbreviation, m.Language, m.Versification, string(lic), m.Source, m.Version,
		strings.Join(m.Books, " "), m.VerseCount, m.CreatedAt)
	return err
}

func insertVerses(ctx context.Context, tx *sql.Tx, verses []Verse) error {
	vs, err := tx.PrepareContext(ctx, `INSERT INTO verses VALUES (?,?,?,?)`)
	if err != nil {
		return err
	}
	defer vs.Close()
	ss, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO strongs VALUES (?,?)`)
	if err != nil {
		return err
	}
	defer ss.Close()
	for _, v := range verses {
		if _, err := vs.ExecContext(ctx, int64(v.ID), int64(v.End), v.Text, v.Markup); err != nil {
			return err
		}
		for _, n := range v.Strongs {
			if _, err := ss.ExecContext(ctx, n, int64(v.ID)); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertMaps(ctx context.Context, tx *sql.Tx, maps []Mapping) error {
	for _, m := range maps {
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO versification_map VALUES (?,?,?)`,
			string(m.System), int64(m.Module), int64(m.SystemID))
		if err != nil {
			return err
		}
	}
	return nil
}

// Module is an open module file.
type Module struct {
	Meta
	db      *sql.DB
	systems map[verse.System]bool // systems with rows in versification_map

	charsOnce sync.Once
	chars     int
	charsErr  error
}

func openModule(path string) (*Module, error) {
	dsn := "file:" + filepath.ToSlash(path) + "?mode=ro&_pragma=query_only(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	m := &Module{db: db, systems: map[verse.System]bool{}}
	if err := m.load(); err != nil {
		db.Close()
		return nil, fmt.Errorf("bible: %s: %w", filepath.Base(path), err)
	}
	return m, nil
}

func (m *Module) load() error {
	var v int
	if err := m.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v != FormatVersion {
		return fmt.Errorf("module format %d, want %d", v, FormatVersion)
	}
	var lic, books string
	err := m.db.QueryRow(`SELECT id, kind, name, abbreviation, language, versification, licence, source,
		version, books, verse_count, created_at FROM module`).Scan(&m.ID, &m.Kind, &m.Name, &m.Abbreviation,
		&m.Language, &m.Versification, &lic, &m.Source, &m.Version, &books, &m.VerseCount, &m.CreatedAt)
	if err != nil {
		return err
	}
	m.Books = strings.Fields(books)
	if err := json.Unmarshal([]byte(lic), &m.Licence); err != nil {
		return err
	}
	rows, err := m.db.Query(`SELECT DISTINCT system FROM versification_map`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return err
		}
		m.systems[verse.System(s)] = true
	}
	return rows.Err()
}

func (m *Module) Close() error { return m.db.Close() }

// Read returns the verses overlapping r, in this module's numbering.
func (m *Module) Read(ctx context.Context, r verse.Range) ([]Verse, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id, end_id, text, markup FROM verses
		WHERE id <= ? AND end_id >= ? ORDER BY id`, int64(r.End), int64(r.Start))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Verse{}
	for rows.Next() {
		var v Verse
		if err := rows.Scan(&v.ID, &v.End, &v.Text, &v.Markup); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ToModule converts a verse from another numbering into this module's numbering.
func (m *Module) ToModule(ctx context.Context, id verse.ID, from verse.System) verse.ID {
	if m.systems[from] {
		var out int64
		err := m.db.QueryRowContext(ctx, `SELECT module_id FROM versification_map WHERE system = ? AND system_id = ?
			ORDER BY module_id LIMIT 1`, string(from), int64(id)).Scan(&out)
		if err == nil {
			return verse.ID(out)
		}
	}
	if c, err := verse.Convert(id, from, verse.System(m.Versification)); err == nil {
		return c
	}
	return id
}

// FromModule converts one of this module's verses into another numbering.
func (m *Module) FromModule(ctx context.Context, id verse.ID, to verse.System) verse.ID {
	if m.systems[to] {
		var out int64
		err := m.db.QueryRowContext(ctx, `SELECT system_id FROM versification_map WHERE system = ? AND module_id = ?`,
			string(to), int64(id)).Scan(&out)
		if err == nil {
			return verse.ID(out)
		}
	}
	if c, err := verse.Convert(id, verse.System(m.Versification), to); err == nil {
		return c
	}
	return id
}

// ToModuleRange converts both ends of a range.
func (m *Module) ToModuleRange(ctx context.Context, r verse.Range, from verse.System) verse.Range {
	return verse.Range{Start: m.ToModule(ctx, r.Start, from), End: m.ToModule(ctx, r.End, from)}
}

// Hit is one search result.
type Hit struct {
	Module  string   `json:"module"`
	ID      verse.ID `json:"id"`
	End     verse.ID `json:"end,omitempty"`
	Ref     string   `json:"ref"`
	Text    string   `json:"text"`
	Snippet string   `json:"snippet,omitempty" desc:"Text with matches wrapped in <mark></mark>"`
	Score   float64  `json:"score" desc:"Higher is better; scores from different modules are roughly comparable"`
}

// Search runs an FTS5 query within r, best matches first.
func (m *Module) Search(ctx context.Context, match string, r verse.Range, limit int) ([]Hit, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT v.id, v.end_id, v.text,
		snippet(verses_fts, 0, '<mark>', '</mark>', '…', 16), bm25(verses_fts) AS rank
		FROM verses_fts JOIN verses v ON v.id = verses_fts.rowid
		WHERE verses_fts MATCH ? AND v.id <= ? AND v.end_id >= ?
		ORDER BY rank LIMIT ?`, match, int64(r.End), int64(r.Start), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return m.scanHits(rows, true)
}

// SearchStrong finds verses tagged with a Strong's number within r.
func (m *Module) SearchStrong(ctx context.Context, number string, r verse.Range, limit int) ([]Hit, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT v.id, v.end_id, v.text, '', 0 FROM strongs s
		JOIN verses v ON v.id = s.verse_id WHERE s.number = ? AND v.id <= ? AND v.end_id >= ?
		ORDER BY v.id LIMIT ?`, number, int64(r.End), int64(r.Start), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return m.scanHits(rows, false)
}

func (m *Module) scanHits(rows *sql.Rows, ranked bool) ([]Hit, error) {
	out := []Hit{}
	for rows.Next() {
		h := Hit{Module: m.ID}
		var rank float64
		if err := rows.Scan(&h.ID, &h.End, &h.Text, &h.Snippet, &rank); err != nil {
			return nil, err
		}
		if h.End == h.ID {
			h.End = 0
		}
		h.Ref = refString(h.ID, h.End)
		if ranked {
			h.Score = -rank
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func refString(id, end verse.ID) string {
	if end == 0 || end == id {
		return id.String()
	}
	return verse.Range{Start: id, End: end}.String()
}

func now() int64 { return time.Now().UnixMilli() }

// TextChars is the length of the module's text in characters, for licence limits measured against
// the whole work. It is counted once per open module.
func (m *Module) TextChars(ctx context.Context) (int, error) {
	m.charsOnce.Do(func() {
		m.charsErr = m.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(length(text)), 0) FROM verses`).Scan(&m.chars)
	})
	return m.chars, m.charsErr
}
