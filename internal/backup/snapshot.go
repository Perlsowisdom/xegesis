package backup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/store"
	_ "modernc.org/sqlite"
)

// vacuumInto writes a consistent copy of the open library to dest. VACUUM INTO reads one
// transaction snapshot, so the copy is correct while the app keeps writing (SQLite's online
// backup, without needing a hook in internal/store).
func vacuumInto(ctx context.Context, s *store.Store, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	rows, err := s.Query(ctx, "VACUUM INTO ?", dest)
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	err = rows.Err()
	if cerr := rows.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dest)
	}
	return err
}

// openReadOnly opens a database file for reading only.
func openReadOnly(path string) (*sql.DB, error) {
	return sql.Open("sqlite", path+"?_pragma=query_only(1)&_pragma=busy_timeout(5000)")
}

// verifyCopy checks that a backup file opens, passes SQLite's integrity check and returns its
// schema version.
func verifyCopy(ctx context.Context, path string) (int, error) {
	db, err := openReadOnly(path)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var check string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&check); err != nil {
		return 0, err
	}
	if check != "ok" {
		return 0, fmt.Errorf("integrity check failed: %s", check)
	}
	var v int
	err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v)
	return v, err
}

// snapshot is a private consistent copy of the library, read by an export so that every table
// comes from the same moment.
type snapshot struct {
	db  *sql.DB
	dir string
}

func takeSnapshot(ctx context.Context, s *store.Store) (*snapshot, error) {
	dir, err := os.MkdirTemp("", "xegesis-export-")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "snapshot.db")
	if err := vacuumInto(ctx, s, path); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	db, err := openReadOnly(path)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return &snapshot{db: db, dir: dir}, nil
}

func (sn *snapshot) Close() {
	sn.db.Close()
	os.RemoveAll(sn.dir)
}

// rows reads every row of a table, ordered by the given columns.
func (sn *snapshot) rows(ctx context.Context, table string, order []string) ([]store.Row, error) {
	q := "SELECT * FROM " + table
	if len(order) > 0 {
		q += " ORDER BY " + strings.Join(order, ", ")
	}
	rs, err := sn.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	cols, err := rs.Columns()
	if err != nil {
		return nil, err
	}
	var out []store.Row
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := store.Row{}
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				vals[i] = string(b)
			}
			r[c] = vals[i]
		}
		out = append(out, r)
	}
	return out, rs.Err()
}
