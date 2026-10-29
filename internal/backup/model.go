package backup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Kinds and xFormat slugs follow docs.any.org/database/data-types.
type fieldKind int

const (
	fText     fieldKind = iota // kind string, xFormat text
	fNumber                    // kind number, xFormat number
	fDateTime                  // kind datetime, xFormat datetime; stored as UTC milliseconds
	fDate                      // kind datetime, xFormat date; stored as an ISO date
	fChoice                    // kind array, xFormat choice
	fRelation                  // kind array, xFormat relation
	fJSON                      // kind object, stored as JSON text
	fBody                      // the body, written to the .md file
)

type field struct {
	name string // property name
	col  string // column name
	kind fieldKind
	rel  string // relation target type
	opts []string
}

// objType is one kind of exported object, backed by one table.
type objType struct {
	name   string
	table  string
	group  string // user-facing group that selects it
	key    []string
	fields []field
	anchor bool // carries an anchor through anchor_id
}

const anyURI = "any://"

func ts(name string) field { return field{name: name, col: name, kind: fDateTime} }

var objTypes = []objType{
	{name: "note", table: "notes", group: "note", key: []string{"id"}, anchor: true, fields: []field{
		{name: "title", col: "title", kind: fText}, {name: "body", col: "body_md", kind: fBody},
		ts("created_at"), ts("updated_at"), ts("deleted_at")}},
	{name: "note_tag", table: "note_tags", group: "note", key: []string{"note_id", "tag"}, fields: []field{
		{name: "note", col: "note_id", kind: fRelation, rel: "note"}, {name: "tag", col: "tag", kind: fText}}},
	{name: "highlight", table: "highlights", group: "highlight", key: []string{"id"}, anchor: true, fields: []field{
		{name: "style", col: "style", kind: fText}, ts("created_at"), ts("deleted_at")}},
	{name: "list", table: "lists", group: "list", key: []string{"id"}, fields: []field{
		{name: "kind", col: "kind", kind: fChoice, opts: []string{"verses", "words", "excerpts"}},
		{name: "name", col: "name", kind: fText}, ts("created_at"), ts("updated_at"), ts("deleted_at")}},
	{name: "list_entry", table: "list_entries", group: "list", key: []string{"id"}, anchor: true, fields: []field{
		{name: "list", col: "list_id", kind: fRelation, rel: "list"},
		{name: "word", col: "word", kind: fText}, {name: "language", col: "language", kind: fText},
		{name: "note", col: "note", kind: fText}, {name: "position", col: "position", kind: fNumber},
		{name: "strong", col: "strong", kind: fText}, {name: "lemma", col: "lemma", kind: fText},
		{name: "attribution", col: "attribution", kind: fText}}},
	{name: "collection", table: "collections", group: "collection", key: []string{"id"}, fields: []field{
		{name: "parent", col: "parent_id", kind: fRelation, rel: "collection"},
		{name: "name", col: "name", kind: fText},
		{name: "kind", col: "kind", kind: fChoice, opts: []string{"manual", "rule"}},
		{name: "rule", col: "rule_json", kind: fJSON}, {name: "position", col: "position", kind: fNumber},
		ts("created_at"), ts("updated_at"), ts("deleted_at")}},
	{name: "collection_item", table: "collection_items", group: "collection", key: []string{"collection_id", "item_id"}, fields: []field{
		{name: "collection", col: "collection_id", kind: fRelation, rel: "collection"},
		{name: "item", col: "item_id", kind: fRelation, rel: "item"},
		{name: "position", col: "position", kind: fNumber}}},
	{name: "reading_plan", table: "reading_plans", group: "reading_plan", key: []string{"id"}, fields: []field{
		{name: "name", col: "name", kind: fText}, {name: "start_date", col: "start_date", kind: fDate},
		{name: "weekdays", col: "weekdays", kind: fNumber}, {name: "template", col: "template", kind: fText},
		ts("created_at"), ts("deleted_at")}},
	{name: "plan_schedule", table: "plan_schedule", group: "reading_plan", key: []string{"plan_id", "day_no"}, fields: []field{
		{name: "plan", col: "plan_id", kind: fRelation, rel: "reading_plan"},
		{name: "day_no", col: "day_no", kind: fNumber}, {name: "date", col: "day_date", kind: fDate}}},
	{name: "plan_day", table: "plan_days", group: "reading_plan", key: []string{"plan_id", "day_no", "verse_start"}, fields: []field{
		{name: "plan", col: "plan_id", kind: fRelation, rel: "reading_plan"},
		{name: "day_no", col: "day_no", kind: fNumber}, {name: "verse_start", col: "verse_start", kind: fNumber},
		{name: "verse_end", col: "verse_end", kind: fNumber},
		{name: "item", col: "item_id", kind: fRelation, rel: "item"},
		{name: "locator", col: "locator_json", kind: fJSON}}},
	{name: "plan_progress", table: "plan_progress", group: "reading_plan", key: []string{"plan_id", "day_no"}, fields: []field{
		{name: "plan", col: "plan_id", kind: fRelation, rel: "reading_plan"},
		{name: "day_no", col: "day_no", kind: fNumber}, ts("completed_at")}},
	{name: "document", table: "documents", group: "document", key: []string{"id"}, fields: []field{
		{name: "kind", col: "kind", kind: fText}, {name: "title", col: "title", kind: fText},
		{name: "body", col: "body_md", kind: fBody}, {name: "outline", col: "outline_json", kind: fJSON},
		ts("created_at"), ts("updated_at"), ts("deleted_at")}},
}

// itemType holds the catalogue rows that exported data refers to, so references still resolve in a
// library that lacks them. Import creates a missing item but never changes an existing one.
var itemType = objType{name: "item", table: "items", key: []string{"id"}, fields: []field{
	{name: "kind", col: "kind", kind: fText}, {name: "title", col: "title", kind: fText},
	{name: "subtitle", col: "subtitle", kind: fText}, {name: "language", col: "language", kind: fText},
	{name: "year", col: "year", kind: fNumber}, {name: "publisher", col: "publisher", kind: fText},
	{name: "series", col: "series", kind: fText}, {name: "series_no", col: "series_no", kind: fText},
	{name: "isbn", col: "isbn", kind: fText}, {name: "source", col: "source", kind: fText},
	{name: "source_ref", col: "source_ref", kind: fText}, {name: "module_id", col: "module_id", kind: fText},
	ts("created_at"), ts("updated_at"), ts("deleted_at")}}

var anchorFields = []field{
	{name: "verse_start", col: "verse_start", kind: fNumber}, {name: "verse_end", col: "verse_end", kind: fNumber},
	{name: "versification", col: "versification", kind: fText},
	{name: "item", col: "item_id", kind: fRelation, rel: "item"},
	{name: "locator", col: "locator_json", kind: fJSON},
}

// Groups are the names a caller can pass to choose what to export or import.
var Groups = []string{"note", "highlight", "list", "collection", "reading_plan", "document"}

func typeByName(name string) (objType, bool) {
	if name == "item" {
		return itemType, true
	}
	for _, t := range objTypes {
		if t.name == name {
			return t, true
		}
	}
	return objType{}, false
}

// objectID is the file name stem of a row: the id, or the key parts joined by "." (IDs are ULIDs, so
// they never contain a dot). A NULL key part is written "_".
func objectID(t objType, row store.Row) string {
	parts := make([]string, len(t.key))
	for i, c := range t.key {
		if row[c] == nil {
			parts[i] = "_"
		} else {
			parts[i] = fmt.Sprint(row[c])
		}
	}
	return strings.Join(parts, ".")
}

func msToDate(ms int64) map[string]string {
	return map[string]string{"$date": time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z")}
}

func encodeValue(f field, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch f.kind {
	case fText:
		return fmt.Sprint(v), nil
	case fNumber:
		return v, nil
	case fDateTime:
		ms, ok := v.(int64)
		if !ok {
			return nil, fmt.Errorf("%s: not a timestamp: %v", f.name, v)
		}
		return msToDate(ms), nil
	case fDate:
		s := fmt.Sprint(v)
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return map[string]string{"$date": t.Format("2006-01-02T15:04:05.000Z")}, nil
		}
		return s, nil
	case fChoice:
		return []string{fmt.Sprint(v)}, nil
	case fRelation:
		return []string{anyURI + fmt.Sprint(v)}, nil
	case fJSON:
		s := fmt.Sprint(v)
		var buf bytes.Buffer
		if (strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")) && json.Compact(&buf, []byte(s)) == nil && buf.String() == s {
			return json.RawMessage(s), nil
		}
		return s, nil
	}
	return nil, fmt.Errorf("%s: unsupported kind", f.name)
}

// decodeValue is the inverse of encodeValue. raw is the property as read from the JSON file.
func decodeValue(f field, raw json.RawMessage) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	switch f.kind {
	case fText:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
		return s, nil
	case fNumber:
		var n json.Number
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if err := d.Decode(&n); err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
		if i, err := n.Int64(); err == nil {
			return i, nil
		}
		return n.Float64()
	case fDateTime, fDate:
		return decodeDate(f, raw)
	case fChoice, fRelation:
		var list []string
		if err := json.Unmarshal(raw, &list); err != nil || len(list) != 1 {
			return nil, fmt.Errorf("%s: want an array with one value", f.name)
		}
		return strings.TrimPrefix(list[0], anyURI), nil
	case fJSON:
		if raw[0] == '"' {
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				return nil, err
			}
			return s, nil
		}
		var buf bytes.Buffer
		if err := json.Compact(&buf, raw); err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
		return buf.String(), nil
	}
	return nil, fmt.Errorf("%s: unsupported kind", f.name)
}

func decodeDate(f field, raw json.RawMessage) (any, error) {
	if f.kind == fDate && raw[0] == '"' { // a start_date that was not an ISO date
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		return s, nil
	}
	var d struct {
		Date json.RawMessage `json:"$date"`
	}
	if err := json.Unmarshal(raw, &d); err != nil || len(d.Date) == 0 {
		return nil, fmt.Errorf(`%s: want {"$date": ...}`, f.name)
	}
	var t time.Time
	if d.Date[0] == '"' {
		var s string
		if err := json.Unmarshal(d.Date, &s); err != nil {
			return nil, err
		}
		var err error
		if t, err = time.Parse(time.RFC3339Nano, s); err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
	} else {
		var ms int64
		if err := json.Unmarshal(d.Date, &ms); err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
		t = time.UnixMilli(ms)
	}
	if f.kind == fDate {
		return t.UTC().Format("2006-01-02"), nil
	}
	return t.UnixMilli(), nil
}

// refText is the readable reference for verse columns, such as "Romans 8:28".
func refText(start, end any) (string, bool) {
	s, ok1 := start.(int64)
	e, ok2 := end.(int64)
	if !ok1 {
		return "", false
	}
	if !ok2 {
		e = s
	}
	id := verse.ID(s)
	if !id.Valid() || !verse.ID(e).Valid() {
		return "", false
	}
	return verse.Range{Start: id, End: verse.ID(e)}.String(), true
}
