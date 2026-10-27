package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// indexVersion is the index file format. An index of another version is rebuilt.
const indexVersion = 1

// One index per library folder (ADR 0006), at <data>/index/<folder-id>.db. It is a cache: it can
// always be rebuilt from the folder's files.
const indexSchema = `
CREATE TABLE state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
CREATE TABLE indexed (
    file_id    TEXT PRIMARY KEY,
    item_id    TEXT NOT NULL,
    rel_path   TEXT NOT NULL,
    sha256     TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    passages   INTEGER NOT NULL,
    error      TEXT,
    indexed_at INTEGER NOT NULL
);
CREATE TABLE passages (
    id      INTEGER PRIMARY KEY,
    file_id TEXT NOT NULL,
    item_id TEXT NOT NULL,
    seq     INTEGER NOT NULL,
    heading TEXT NOT NULL,
    locator TEXT NOT NULL,
    text    TEXT NOT NULL
);
CREATE INDEX passages_file ON passages(file_id, seq);
CREATE INDEX passages_item ON passages(item_id);
CREATE VIRTUAL TABLE passages_fts USING fts5(heading, text, content='passages', content_rowid='id',
    tokenize='unicode61 remove_diacritics 2');
CREATE TRIGGER passages_ai AFTER INSERT ON passages BEGIN
    INSERT INTO passages_fts(rowid, heading, text) VALUES (new.id, new.heading, new.text);
END;
CREATE TRIGGER passages_ad AFTER DELETE ON passages BEGIN
    INSERT INTO passages_fts(passages_fts, rowid, heading, text) VALUES ('delete', old.id, old.heading, old.text);
END;
CREATE TABLE refs (
    verse_start INTEGER NOT NULL,
    verse_end   INTEGER NOT NULL,
    passage_id  INTEGER NOT NULL,
    PRIMARY KEY (verse_start, verse_end, passage_id)
) WITHOUT ROWID;
CREATE INDEX refs_passage ON refs(passage_id);
`

// index is one folder's open index file. It keeps a single connection.
type index struct {
	folderID string
	path     string
	db       *sql.DB
}

func openIndex(path, folderID string) (*index, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
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
		return nil, fmt.Errorf("index %s: %w", filepath.Base(path), err)
	}
	return ix, nil
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
		return fmt.Errorf("format %d, want %d; rebuild it", v, indexVersion)
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

func (ix *index) close() error { return ix.db.Close() }

// indexedFile is an indexed row.
type indexedFile struct {
	sha  string
	size int64
	err  string
}

func (ix *index) indexed(ctx context.Context) (map[string]indexedFile, error) {
	rows, err := ix.db.QueryContext(ctx, `SELECT file_id, sha256, size_bytes, COALESCE(error, '') FROM indexed`)
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
	ins, err := tx.PrepareContext(ctx, `INSERT INTO passages (file_id, item_id, seq, heading, locator, text)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	ref, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO refs (verse_start, verse_end, passage_id) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ref.Close()
	for i, p := range ps {
		loc, _ := json.Marshal(p.loc)
		res, err := ins.ExecContext(ctx, f.id, f.itemID, i, p.heading, string(loc), p.text)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		for _, c := range findRefs(p.text) {
			if _, err := ref.ExecContext(ctx, int64(c.r.Start), int64(c.r.End), id); err != nil {
				return err
			}
		}
	}
	var msg any
	if readErr != nil {
		msg = readErr.Error()
	}
	_, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO indexed
		(file_id, item_id, rel_path, sha256, size_bytes, passages, error, indexed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		f.id, f.itemID, f.rel, f.sha, f.size, len(ps), msg, time.Now().UnixMilli())
	if err != nil {
		return err
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
	if _, err := tx.ExecContext(ctx, `DELETE FROM indexed WHERE file_id = ?`, fileID); err != nil {
		return err
	}
	return tx.Commit()
}

func dropFile(ctx context.Context, tx *sql.Tx, fileID string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM refs WHERE passage_id IN
		(SELECT id FROM passages WHERE file_id = ?)`, fileID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM passages WHERE file_id = ?`, fileID)
	return err
}

func (ix *index) getState(ctx context.Context, key string) string {
	var v string
	if err := ix.db.QueryRowContext(ctx, `SELECT value FROM state WHERE key = ?`, key).Scan(&v); err != nil &&
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
