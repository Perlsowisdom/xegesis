package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// FormatVersion is the version of the export folder layout.
const FormatVersion = 1

type ExportParams struct {
	Path    string   `json:"path,omitempty" desc:"Where to write: a folder (an xegesis-export-<date> folder is made inside it) or a file ending in .zip. Default: the exports folder in the data folder"`
	Types   []string `json:"types,omitempty" desc:"What to export: note, highlight, list, collection, reading_plan, document. Default: all"`
	History bool     `json:"history,omitempty" desc:"Also write the change history (history.jsonl)"`
}

type ExportResult struct {
	Path   string         `json:"path" desc:"The export folder or .zip file"`
	Counts map[string]int `json:"counts" desc:"Objects written, by type"`
	Bytes  int64          `json:"bytes,omitempty" desc:"Size of the .zip file"`
}

// sink receives the files of an export.
type sink interface {
	write(name string, data []byte) error
	close() error
}

type dirSink struct{ root string }

func (d dirSink) write(name string, data []byte) error {
	p := filepath.Join(d.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
func (d dirSink) close() error { return nil }

type zipSink struct {
	f      *os.File
	w      *zip.Writer
	prefix string
}

func (z *zipSink) write(name string, data []byte) error {
	w, err := z.w.CreateHeader(&zip.FileHeader{Name: z.prefix + "/" + name, Method: zip.Deflate, Modified: now()})
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func (z *zipSink) close() error {
	err := z.w.Close()
	if cerr := z.f.Close(); err == nil {
		err = cerr
	}
	return err
}

func exportData(ctx context.Context, env *command.Env, dataDir string, p ExportParams) (ExportResult, error) {
	if err := needStore(env); err != nil {
		return ExportResult{}, err
	}
	groups, err := chooseGroups(p.Types)
	if err != nil {
		return ExportResult{}, err
	}
	name := "xegesis-export-" + now().UTC().Format("2006-01-02")
	var out sink
	var result string
	if strings.HasSuffix(strings.ToLower(p.Path), ".zip") {
		if _, err := os.Stat(p.Path); err == nil {
			return ExportResult{}, command.Errorf("bad_params", "%s already exists", p.Path)
		}
		if err := os.MkdirAll(filepath.Dir(p.Path), 0o755); err != nil {
			return ExportResult{}, err
		}
		f, err := os.Create(p.Path)
		if err != nil {
			return ExportResult{}, err
		}
		out, result = &zipSink{f: f, w: zip.NewWriter(f), prefix: name}, p.Path
	} else {
		parent := p.Path
		if parent == "" {
			parent = filepath.Join(dataDir, "exports")
		}
		root, err := freshDir(parent, name)
		if err != nil {
			return ExportResult{}, err
		}
		out, result = dirSink{root: root}, root
	}
	counts, err := writeExport(ctx, env.Store, out, groups, p.History)
	cerr := out.close()
	if err == nil {
		err = cerr
	}
	if err != nil {
		if strings.HasSuffix(result, ".zip") {
			os.Remove(result)
		} else {
			os.RemoveAll(result)
		}
		return ExportResult{}, err
	}
	res := ExportResult{Path: result, Counts: counts}
	if st, err := os.Stat(result); err == nil && !st.IsDir() {
		res.Bytes = st.Size()
	}
	return res, nil
}

// freshDir makes parent/name, or parent/name-2 and so on if it exists.
func freshDir(parent, name string) (string, error) {
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	for i := 1; ; i++ {
		d := filepath.Join(parent, name)
		if i > 1 {
			d = fmt.Sprintf("%s-%d", d, i)
		}
		if err := os.Mkdir(d, 0o755); err == nil {
			return d, nil
		} else if !os.IsExist(err) {
			return "", err
		}
	}
}

func chooseGroups(names []string) (map[string]bool, error) {
	groups := map[string]bool{}
	if len(names) == 0 {
		names = Groups
	}
	for _, n := range names {
		ok := false
		for _, g := range Groups {
			ok = ok || g == n
		}
		if !ok {
			return nil, command.Errorf("bad_params", "unknown type %q (use %s)", n, strings.Join(Groups, ", "))
		}
		groups[n] = true
	}
	// Notes and documents refer to their folders, so the folders travel with them.
	if groups["note"] || groups["document"] {
		groups["folder"] = true
	}
	return groups, nil
}

func writeJSON(o sink, name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return o.write(name, append(b, '\n'))
}

func writeExport(ctx context.Context, s *store.Store, out sink, groups map[string]bool, history bool) (map[string]int, error) {
	sn, err := takeSnapshot(ctx, s)
	if err != nil {
		return nil, err
	}
	defer sn.Close()
	anchors, err := loadAnchors(ctx, sn)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	items := map[string]bool{}
	var used []objType
	for _, t := range objTypes {
		if !groups[t.group] {
			continue
		}
		rows, err := sn.rows(ctx, t.table, t.key)
		if err != nil {
			return nil, err
		}
		used = append(used, t)
		for _, row := range rows {
			if err := writeObject(out, t, row, anchors, items); err != nil {
				return nil, err
			}
			counts[t.name]++
		}
	}
	if len(items) > 0 {
		if err := writeItems(ctx, sn, out, items, counts); err != nil {
			return nil, err
		}
		used = append(used, itemType)
	}
	if err := writeJSON(out, "types.json", typesJSON(used)); err != nil {
		return nil, err
	}
	changes := 0
	if history {
		if changes, err = writeHistory(ctx, sn, out, used); err != nil {
			return nil, err
		}
	}
	return counts, writeManifest(ctx, s, out, sn, counts, history, changes)
}

func loadAnchors(ctx context.Context, sn *snapshot) (map[string]store.Row, error) {
	rows, err := sn.rows(ctx, "anchors", nil)
	if err != nil {
		return nil, err
	}
	m := map[string]store.Row{}
	for _, r := range rows {
		m[r["id"].(string)] = r
	}
	return m, nil
}

func writeObject(out sink, t objType, row store.Row, anchors map[string]store.Row, items map[string]bool) error {
	id := objectID(t, row)
	props := map[string]any{"id": id, "type": t.name}
	body := ""
	hasBody := false
	for _, f := range t.fields {
		if f.kind == fBody {
			body, hasBody = fmt.Sprint(row[f.col]), true
			continue
		}
		v, err := encodeValue(f, row[f.col])
		if err != nil {
			return fmt.Errorf("%s %s: %w", t.name, id, err)
		}
		props[f.name] = v
		if f.rel == "item" && row[f.col] != nil {
			items[fmt.Sprint(row[f.col])] = true
		}
	}
	if t.anchor {
		if err := addAnchor(props, row, anchors, items); err != nil {
			return fmt.Errorf("%s %s: %w", t.name, id, err)
		}
	}
	if t.name == "plan_day" {
		if ref, ok := refText(row["verse_start"], row["verse_end"]); ok {
			props["ref"] = ref
		}
	}
	if err := writeJSON(out, "objects/"+t.name+"/"+id+".json", props); err != nil {
		return err
	}
	if hasBody {
		return out.write("objects/"+t.name+"/"+id+".md", []byte(EncodeBody(body)))
	}
	return nil
}

// addAnchor writes the anchor's verse range and locator as properties of the object.
func addAnchor(props map[string]any, row store.Row, anchors map[string]store.Row, items map[string]bool) error {
	aid := row["anchor_id"]
	if aid == nil {
		props["anchor_id"] = nil
		return nil
	}
	a, ok := anchors[fmt.Sprint(aid)]
	if !ok {
		return fmt.Errorf("anchor %v is missing", aid)
	}
	props["anchor_id"] = aid
	for _, f := range anchorFields {
		v, err := encodeValue(f, a[f.col])
		if err != nil {
			return err
		}
		props[f.name] = v
	}
	if ref, ok := refText(a["verse_start"], a["verse_end"]); ok {
		props["ref"] = ref
	}
	if a["item_id"] != nil {
		items[fmt.Sprint(a["item_id"])] = true
	}
	return nil
}

func writeItems(ctx context.Context, sn *snapshot, out sink, want map[string]bool, counts map[string]int) error {
	rows, err := sn.rows(ctx, "items", []string{"id"})
	if err != nil {
		return err
	}
	for _, r := range rows {
		if !want[r["id"].(string)] {
			continue
		}
		if err := writeObject(out, itemType, r, nil, nil); err != nil {
			return err
		}
		counts["item"]++
	}
	return nil
}

type typeProp struct {
	Key     string         `json:"key"`
	Kind    string         `json:"kind"`
	XFormat string         `json:"xFormat,omitempty"`
	Config  map[string]any `json:"config,omitempty"`
	File    string         `json:"file,omitempty"`
}

type typeEntry struct {
	XKey       string     `json:"xKey"`
	Name       string     `json:"name"`
	Properties []typeProp `json:"properties"`
}

func propOf(f field) typeProp {
	p := typeProp{Key: f.name}
	switch f.kind {
	case fText:
		p.Kind, p.XFormat = "string", "text"
	case fNumber:
		p.Kind, p.XFormat = "number", "number"
	case fDateTime:
		p.Kind, p.XFormat = "datetime", "datetime"
	case fDate:
		p.Kind, p.XFormat = "datetime", "date"
	case fChoice:
		p.Kind, p.XFormat = "array", "choice"
		p.Config = map[string]any{"options": f.opts}
	case fRelation:
		p.Kind, p.XFormat = "array", "relation"
		p.Config = map[string]any{"target": f.rel}
	case fJSON:
		p.Kind = "object"
	case fBody:
		p.Kind, p.XFormat, p.File = "string", "markdown", ".md"
	}
	return p
}

func typesJSON(used []objType) []typeEntry {
	var out []typeEntry
	for _, t := range used {
		e := typeEntry{XKey: t.name, Name: strings.ReplaceAll(t.name, "_", " "),
			Properties: []typeProp{{Key: "id", Kind: "string", XFormat: "text"}}}
		for _, f := range t.fields {
			e.Properties = append(e.Properties, propOf(f))
		}
		if t.anchor {
			e.Properties = append(e.Properties, typeProp{Key: "anchor_id", Kind: "string", XFormat: "text"},
				typeProp{Key: "ref", Kind: "string", XFormat: "text"})
			for _, f := range anchorFields {
				e.Properties = append(e.Properties, propOf(f))
			}
		}
		out = append(out, e)
	}
	return out
}

// writeHistory writes the change log for the exported tables, one change per line.
func writeHistory(ctx context.Context, sn *snapshot, out sink, used []objType) (int, error) {
	tables := []string{"anchors"}
	for _, t := range used {
		tables = append(tables, t.table)
	}
	sort.Strings(tables)
	marks := strings.TrimSuffix(strings.Repeat("?,", len(tables)), ",")
	args := make([]any, len(tables))
	for i, t := range tables {
		args[i] = t
	}
	rs, err := sn.db.QueryContext(ctx, `SELECT id, device_id, hlc, command, object_type, object_id, op, field,
		before_json, after_json, created_at FROM changes WHERE object_type IN (`+marks+`) ORDER BY hlc, id`, args...)
	if err != nil {
		return 0, err
	}
	defer rs.Close()
	var sb strings.Builder
	n := 0
	for rs.Next() {
		var id, dev, hlc, cmd, typ, oid, op string
		var field, before, after sql.NullString
		var at int64
		if err := rs.Scan(&id, &dev, &hlc, &cmd, &typ, &oid, &op, &field, &before, &after, &at); err != nil {
			return 0, err
		}
		line := map[string]any{"id": id, "device": dev, "hlc": hlc, "command": cmd, "object_type": typ,
			"object_id": oid, "op": op, "created_at": msToDate(at)}
		if field.Valid {
			line["field"] = field.String
		}
		if before.Valid {
			line["before"] = json.RawMessage(before.String)
		}
		if after.Valid {
			line["after"] = json.RawMessage(after.String)
		}
		b, err := json.Marshal(line)
		if err != nil {
			return 0, err
		}
		sb.Write(b)
		sb.WriteByte('\n')
		n++
	}
	if err := rs.Err(); err != nil {
		return 0, err
	}
	return n, out.write("history.jsonl", []byte(sb.String()))
}

type manifest struct {
	Format        string         `json:"format"`
	FormatVersion int            `json:"format_version"`
	AppVersion    string         `json:"app_version"`
	SchemaVersion int            `json:"schema_version"`
	Device        string         `json:"device"`
	ExportedAt    any            `json:"exported_at"`
	Encoding      string         `json:"encoding"`
	Counts        map[string]int `json:"counts"`
	History       bool           `json:"history"`
	HistoryCount  int            `json:"history_changes,omitempty"`
}

func writeManifest(ctx context.Context, s *store.Store, out sink, sn *snapshot, counts map[string]int, history bool, changes int) error {
	var schema int
	if err := sn.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&schema); err != nil {
		return err
	}
	return writeJSON(out, "manifest.json", manifest{
		Format: ManifestFormat, FormatVersion: FormatVersion, AppVersion: core.Version, SchemaVersion: schema,
		Device: s.Device(), ExportedAt: msToDate(now().UnixMilli()),
		Encoding: "any value encodings: docs.any.org/database/data-types and /database/markdown-import-export",
		Counts:   counts, History: history, HistoryCount: changes,
	})
}

// ManifestFormat identifies an Xegesis export.
const ManifestFormat = "xegesis-export"

var now = time.Now
