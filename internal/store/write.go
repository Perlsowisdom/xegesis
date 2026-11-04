package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrNotFound is returned when a keyed row does not exist.
var ErrNotFound = errors.New("not found")

// Tx is one logged write transaction, made by one command.
type Tx struct {
	s       *Store
	tx      *sql.Tx
	ctx     context.Context
	command string
}

// Write runs fn in a transaction. Every change fn makes is logged under the command ID.
func (s *Store) Write(ctx context.Context, command string, fn func(*Tx) error) error {
	if command == "" {
		return errors.New("store: write without a command ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&Tx{s: s, tx: tx, ctx: ctx, command: command}); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Create inserts a row and logs it. Columns left out take their defaults.
func (t *Tx) Create(name string, row Row) error {
	tb, err := t.table(name, row)
	if err != nil {
		return err
	}
	cols := sortedKeys(row)
	args := make([]any, len(cols))
	for i, c := range cols {
		args[i] = row[c]
	}
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", name, strings.Join(cols, ", "),
		strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", "))
	if _, err := t.tx.ExecContext(t.ctx, q, args...); err != nil {
		return err
	}
	key := Row{}
	for _, c := range tb.pk {
		key[c] = row[c]
	}
	full, err := t.get(name, tb, key)
	if err != nil {
		return err
	}
	return t.log(name, tb, key, "create", "", nil, full)
}

// Update sets fields on the row with key and logs one change per field whose value changed.
func (t *Tx) Update(name string, key, fields Row) error {
	tb, err := t.table(name, fields)
	if err != nil {
		return err
	}
	before, err := t.get(name, tb, key)
	if err != nil {
		return err
	}
	for _, c := range sortedKeys(fields) {
		if isKey(tb, c) {
			return fmt.Errorf("store: %s.%s is part of the key and cannot change", name, c)
		}
		if sameValue(before[c], fields[c]) {
			continue
		}
		where, args := keyClause(tb, key)
		q := fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s", name, c, where)
		if _, err := t.tx.ExecContext(t.ctx, q, append([]any{fields[c]}, args...)...); err != nil {
			return err
		}
		if err := t.log(name, tb, key, "update", c, before[c], fields[c]); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes the row with key and logs it. Tables with deleted_at are soft-deleted, so the row
// can be restored and the deletion can reach other devices.
func (t *Tx) Delete(name string, key Row) error {
	tb, err := t.table(name, nil)
	if err != nil {
		return err
	}
	before, err := t.get(name, tb, key)
	if err != nil {
		return err
	}
	where, args := keyClause(tb, key)
	if tb.soft {
		q := fmt.Sprintf("UPDATE %s SET deleted_at = ? WHERE %s", name, where)
		_, err = t.tx.ExecContext(t.ctx, q, append([]any{nowMS()}, args...)...)
	} else {
		_, err = t.tx.ExecContext(t.ctx, fmt.Sprintf("DELETE FROM %s WHERE %s", name, where), args...)
	}
	if err != nil {
		return err
	}
	return t.log(name, tb, key, "delete", "", before, nil)
}

// Purge removes the row with key for good, even from a table with deleted_at. Earlier changes to
// the row stay in the history, so other devices still learn of it, but lose their values: what is
// deleted for good cannot be read back from the history either. The purge is logged as a delete
// whose field is "purge".
func (t *Tx) Purge(name string, key Row) error {
	tb, err := t.table(name, nil)
	if err != nil {
		return err
	}
	if _, err := t.get(name, tb, key); err != nil {
		return err
	}
	where, args := keyClause(tb, key)
	if _, err := t.tx.ExecContext(t.ctx, fmt.Sprintf("DELETE FROM %s WHERE %s", name, where), args...); err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(t.ctx, `UPDATE changes SET before_json = NULL, after_json = NULL
		WHERE object_type = ? AND object_id = ?`, name, objectID(tb, key)); err != nil {
		return err
	}
	return t.log(name, tb, key, "delete", "purge", nil, nil)
}

// Get reads one row inside the transaction.
func (t *Tx) Get(name string, key Row) (Row, error) {
	tb, err := t.table(name, nil)
	if err != nil {
		return nil, err
	}
	return t.get(name, tb, key)
}

func (t *Tx) table(name string, row Row) (table, error) {
	tb, ok := t.s.tables[name]
	if !ok || name == "changes" || name == "devices" {
		return table{}, fmt.Errorf("store: unknown table %q", name)
	}
	for c := range row {
		if !tb.columns[c] {
			return table{}, fmt.Errorf("store: unknown column %s.%s", name, c)
		}
	}
	return tb, nil
}

func (t *Tx) get(name string, tb table, key Row) (Row, error) {
	where, args := keyClause(tb, key)
	rows, err := t.tx.QueryContext(t.ctx, fmt.Sprintf("SELECT * FROM %s WHERE %s", name, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%s %v: %w", name, key, ErrNotFound)
	}
	return scanRow(rows)
}

func (t *Tx) log(name string, tb table, key Row, op, field string, before, after any) error {
	b, err := encode(before)
	if err != nil {
		return err
	}
	a, err := encode(after)
	if err != nil {
		return err
	}
	var f any
	if field != "" {
		f = field
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO changes
		(id, device_id, hlc, command, object_type, object_id, op, field, before_json, after_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		NewID(), t.s.device, t.s.clock.Now(), t.command, name, objectID(tb, key), op, f, b, a, nowMS())
	return err
}

// objectID is the id column for ordinary tables, or the key values as a JSON array otherwise.
func objectID(tb table, key Row) string {
	if len(tb.pk) == 1 && tb.pk[0] == "id" {
		if s, ok := key["id"].(string); ok {
			return s
		}
	}
	vals := make([]any, len(tb.pk))
	for i, c := range tb.pk {
		vals[i] = key[c]
	}
	out, _ := json.Marshal(vals)
	return string(out)
}

func keyClause(tb table, key Row) (string, []any) {
	parts := make([]string, len(tb.pk))
	args := make([]any, len(tb.pk))
	for i, c := range tb.pk {
		parts[i] = c + " = ?"
		args[i] = key[c]
	}
	return strings.Join(parts, " AND "), args
}

func isKey(tb table, col string) bool {
	for _, c := range tb.pk {
		if c == col {
			return true
		}
	}
	return false
}

func scanRow(rows *sql.Rows) (Row, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	r := Row{}
	for i, c := range cols {
		r[c] = normalize(vals[i])
	}
	return r, nil
}

func normalize(v any) any {
	switch x := v.(type) {
	case int:
		return int64(x)
	case bool:
		if x {
			return int64(1)
		}
		return int64(0)
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		f, _ := x.Float64()
		return f
	}
	return v
}

func sameValue(a, b any) bool {
	ea, _ := json.Marshal(normalize(a))
	eb, _ := json.Marshal(normalize(b))
	return bytes.Equal(ea, eb)
}

func encode(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	if r, ok := v.(Row); ok && r == nil {
		return nil, nil
	}
	out, err := json.Marshal(v)
	return string(out), err
}

func decode(s sql.NullString) (any, error) {
	if !s.Valid {
		return nil, nil
	}
	d := json.NewDecoder(strings.NewReader(s.String))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	if m, ok := v.(map[string]any); ok {
		r := Row{}
		for k, x := range m {
			r[k] = normalize(x)
		}
		return r, nil
	}
	return normalize(v), nil
}

func sortedKeys(r Row) []string {
	out := make([]string, 0, len(r))
	for k := range r {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
