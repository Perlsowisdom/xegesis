package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Change is one entry in the change history.
type Change struct {
	ID         string `json:"id"`
	Device     string `json:"device"`
	HLC        string `json:"hlc"`
	Command    string `json:"command"`
	ObjectType string `json:"object_type"`
	ObjectID   string `json:"object_id"`
	Op         string `json:"op"`
	Field      string `json:"field,omitempty"`
	Before     any    `json:"before,omitempty"`
	After      any    `json:"after,omitempty"`
	CreatedAt  int64  `json:"created_at"`
}

// History lists an object's changes, oldest first.
func (s *Store) History(ctx context.Context, objectType, objectID string) ([]Change, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, device_id, hlc, command, object_type, object_id, op,
		field, before_json, after_json, created_at FROM changes
		WHERE object_type = ? AND object_id = ? ORDER BY hlc`, objectType, objectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Change
	for rows.Next() {
		var c Change
		var field, before, after sql.NullString
		if err := rows.Scan(&c.ID, &c.Device, &c.HLC, &c.Command, &c.ObjectType, &c.ObjectID, &c.Op,
			&field, &before, &after, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.Field = field.String
		if c.Before, err = decode(before); err != nil {
			return nil, err
		}
		if c.After, err = decode(after); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Restore returns an object to its state just after the change changeID. The restore is itself a
// set of logged changes, so it can be undone the same way.
func (s *Store) Restore(ctx context.Context, command, objectType string, key Row, changeID string) error {
	tb, ok := s.tables[objectType]
	if !ok {
		return fmt.Errorf("store: unknown table %q", objectType)
	}
	history, err := s.History(ctx, objectType, objectID(tb, key))
	if err != nil {
		return err
	}
	target, found := replay(history, changeID)
	if !found {
		return fmt.Errorf("change %s of %s %v: %w", changeID, objectType, key, ErrNotFound)
	}
	return s.Write(ctx, command, func(t *Tx) error {
		current, err := t.Get(objectType, key)
		if errors.Is(err, ErrNotFound) {
			current = nil
		} else if err != nil {
			return err
		}
		switch {
		case target == nil && current == nil:
			return nil
		case target == nil:
			if tb.soft && current["deleted_at"] != nil {
				return nil
			}
			return t.Delete(objectType, key)
		case current == nil:
			return t.Create(objectType, target)
		}
		fields := Row{}
		for c, v := range target {
			if !isKey(tb, c) && tb.columns[c] {
				fields[c] = v
			}
		}
		return t.Update(objectType, key, fields)
	})
}

// replay applies history up to and including changeID and returns the resulting row, or nil if
// the object did not exist at that point.
func replay(history []Change, changeID string) (Row, bool) {
	var row Row
	for _, c := range history {
		switch c.Op {
		case "create":
			row, _ = c.After.(Row)
			row = copyRow(row)
		case "update":
			if row != nil {
				row[c.Field] = c.After
			}
		case "delete":
			row = nil
		}
		if c.ID == changeID {
			return row, true
		}
	}
	return nil, false
}

func copyRow(r Row) Row {
	if r == nil {
		return nil
	}
	out := make(Row, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}
