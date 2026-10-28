package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// indexVersion is the index file format. An index file of another format is deleted and built
// again, since it is only a cache of the folder's files. Format 2 stores passage text compressed
// and refers to files by a small number.
const indexVersion = 2

// One index per library folder (ADR 0006), at <data>/index/<folder-id>.db. It is a cache: it can
// always be rebuilt from the folder's files.
//
// A file's passages have consecutive IDs, first_id to first_id+passages-1, so they are found and
// dropped by range. passages.text_z is the passage text compressed (compress.go); FTS5 reads it
// through the passages_text view, so snippets and highlighting work as with plain text.
const indexSchema = `
CREATE TABLE state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
CREATE TABLE indexed (
    num        INTEGER PRIMARY KEY,
    file_id    TEXT NOT NULL UNIQUE,
    item_id    TEXT NOT NULL,
    rel_path   TEXT NOT NULL,
    sha256     TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    passages   INTEGER NOT NULL,
    first_id   INTEGER NOT NULL,
    text_bytes INTEGER NOT NULL,
    error      TEXT,
    indexed_at INTEGER NOT NULL
);
CREATE INDEX indexed_item ON indexed(item_id);
CREATE TABLE passages (
    id      INTEGER PRIMARY KEY,
    file    INTEGER NOT NULL,
    heading TEXT NOT NULL,
    locator TEXT NOT NULL,
    text_z  BLOB NOT NULL
);
CREATE VIEW passages_text (id, heading, text) AS SELECT id, heading, xg_unpack(text_z) FROM passages;
CREATE VIRTUAL TABLE passages_fts USING fts5(heading, text, content='passages_text', content_rowid='id',
    tokenize='unicode61 remove_diacritics 2');
CREATE TABLE refs (
    verse_start INTEGER NOT NULL,
    verse_end   INTEGER NOT NULL,
    passage_id  INTEGER NOT NULL,
    PRIMARY KEY (verse_start, verse_end, passage_id)
) WITHOUT ROWID;
CREATE INDEX refs_passage ON refs(passage_id);
`

// index is one folder's open index file: one connection that writes, and a few that read, so
// searches go on while a book is being written.
type index struct {
	folderID string
	path     string
	db       *sql.DB // writes
	rd       *sql.DB // reads
}

func openIndex(path, folderID string) (*index, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	ix, err := tryOpen(path, folderID)
	if errors.Is(err, errOldFormat) {
		if err = removeIndexFiles(path); err == nil {
			ix, err = tryOpen(path, folderID)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("index %s: %w", filepath.Base(path), err)
	}
	return ix, nil
}

var errOldFormat = errors.New("index file of another format")

func tryOpen(path, folderID string) (*index, error) {
	dsn := "file:" + filepath.ToSlash(path) +
		"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ix := &index{folderID: folderID, path: path, db: db}
	if err := ix.init(); err != nil {
		db.Close()
		return nil, err
	}
	if ix.rd, err = sql.Open("sqlite", dsn+"&_pragma=query_only(1)"); err != nil {
		db.Close()
		return nil, err
	}
	ix.rd.SetMaxOpenConns(4)
	ix.rd.SetMaxIdleConns(2)
	return ix, nil
}

func removeIndexFiles(path string) error {
	for _, f := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (ix *index) init() error {
	var v int
	if err := ix.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v == indexVersion {
		return nil
	}
	if v != 0 {
		return errOldFormat
	}
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(indexSchema); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO state (key, value) VALUES ('folder_id', ?)`, ix.folderID); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", indexVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

func (ix *index) close() error { return errors.Join(ix.rd.Close(), ix.db.Close()) }

// indexedFile is an indexed row.
type indexedFile struct {
	sha  string
	size int64
	err  string
}

func (ix *index) indexed(ctx context.Context) (map[string]indexedFile, error) {
	rows, err := ix.rd.QueryContext(ctx, `SELECT file_id, sha256, size_bytes, COALESCE(error, '') FROM indexed`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]indexedFile{}
	for rows.Next() {
		var id string
		var f indexedFile
		if err := rows.Scan(&id, &f.sha, &f.size, &f.err); err != nil {
			return nil, err
		}
		out[id] = f
	}
	return out, rows.Err()
}

// put replaces a file's passages and references in one transaction, so a book becomes searchable
// whole or not at all.
func (ix *index) put(ctx context.Context, f libFile, ps []passage, readErr error) error {
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := dropFile(ctx, tx, f.id); err != nil {
		return err
	}
	var first int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) + 1 FROM passages`).Scan(&first); err != nil {
		return err
	}
	var msg any
	if readErr != nil {
		msg = readErr.Error()
	}
	textBytes := 0
	for _, p := range ps {
		textBytes += len(p.text)
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO indexed (file_id, item_id, rel_path, sha256, size_bytes, passages,
		first_id, text_bytes, error, indexed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.id, f.itemID, f.rel, f.sha, f.size, len(ps), first, textBytes, msg, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	num, err := res.LastInsertId()
	if err != nil {
		return err
	}
	ins, err := tx.PrepareContext(ctx, `INSERT INTO passages (id, file, heading, locator, text_z) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	fts, err := tx.PrepareContext(ctx, `INSERT INTO passages_fts (rowid, heading, text) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer fts.Close()
	ref, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO refs (verse_start, verse_end, passage_id) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ref.Close()
	for i, p := range ps {
		id := first + int64(i)
		loc, _ := json.Marshal(p.loc)
		if _, err := ins.ExecContext(ctx, id, num, p.heading, string(loc), packText(p.text)); err != nil {
			return err
		}
		if _, err := fts.ExecContext(ctx, id, p.heading, p.text); err != nil {
			return err
		}
		for _, c := range findRefs(p.text) {
			if _, err := ref.ExecContext(ctx, int64(c.r.Start), int64(c.r.End), id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// remove drops a file that is no longer in the library.
func (ix *index) remove(ctx context.Context, fileID string) error {
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := dropFile(ctx, tx, fileID); err != nil {
		return err
	}
	return tx.Commit()
}

// dropFile removes a file's passages from the full-text index (which needs their text to do so),
// its references and its indexed row.
func dropFile(ctx context.Context, tx *sql.Tx, fileID string) error {
	var first, n int64
	err := tx.QueryRowContext(ctx, `SELECT first_id, passages FROM indexed WHERE file_id = ?`, fileID).Scan(&first, &n)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	last := first + n - 1
	for _, q := range []string{
		`INSERT INTO passages_fts (passages_fts, rowid, heading, text)
			SELECT 'delete', id, heading, xg_unpack(text_z) FROM passages WHERE id BETWEEN ? AND ?`,
		`DELETE FROM refs WHERE passage_id BETWEEN ? AND ?`,
		`DELETE FROM passages WHERE id BETWEEN ? AND ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, first, last); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM indexed WHERE file_id = ?`, fileID)
	return err
}

func (ix *index) getState(ctx context.Context, key string) string {
	var v string
	if err := ix.rd.QueryRowContext(ctx, `SELECT value FROM state WHERE key = ?`, key).Scan(&v); err != nil &&
		!errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	return v
}

func (ix *index) setState(ctx context.Context, kv map[string]string) error {
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range kv {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO state (key, value) VALUES (?, ?)`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}
