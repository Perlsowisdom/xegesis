package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

const maxFile = 256 << 20 // largest single file read from an export

type ImportParams struct {
	Path  string   `json:"path" required:"true" desc:"An export folder or .zip file"`
	Types []string `json:"types,omitempty" desc:"What to import: note, highlight, list, collection, reading_plan, document. Default: all that the export holds"`
}

type ImportResult struct {
	Created   map[string]int `json:"created"`
	Updated   map[string]int `json:"updated"`
	Unchanged map[string]int `json:"unchanged"`
}

func importData(ctx context.Context, env *command.Env, p ImportParams) (ImportResult, error) {
	if err := needStore(env); err != nil {
		return ImportResult{}, err
	}
	groups, err := chooseGroups(p.Types)
	if err != nil {
		return ImportResult{}, err
	}
	fsys, closeFS, err := openExport(p.Path)
	if err != nil {
		return ImportResult{}, err
	}
	defer closeFS()
	if err := checkManifest(fsys); err != nil {
		return ImportResult{}, err
	}
	imp := &importer{env: env, fsys: fsys, res: ImportResult{
		Created: map[string]int{}, Updated: map[string]int{}, Unchanged: map[string]int{}}}
	// Read everything before writing, so a bad file stops the import before any change.
	objs, err := imp.load(groups)
	if err != nil {
		return ImportResult{}, err
	}
	err = env.Store.Write(ctx, "import.data", func(tx *store.Tx) error {
		imp.tx = tx
		imp.ctx = ctx
		for _, o := range objs {
			if err := imp.apply(o); err != nil {
				return fmt.Errorf("%s: %w", o.file, err)
			}
		}
		return nil
	})
	if err != nil {
		return ImportResult{}, err
	}
	return imp.res, nil
}

// openExport opens an export folder or .zip. A zip may hold the export at its root or in one
// folder, as the exporter writes it.
func openExport(p string) (fs.FS, func(), error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, nil, command.Errorf("bad_params", "%v", err)
	}
	var fsys fs.FS
	closeFS := func() {}
	if st.IsDir() {
		fsys = os.DirFS(p)
	} else {
		zr, err := zip.OpenReader(p)
		if err != nil {
			return nil, nil, command.Errorf("bad_params", "%s is not a folder or zip file: %v", p, err)
		}
		fsys, closeFS = zr, func() { zr.Close() }
	}
	if _, err := fs.Stat(fsys, "manifest.json"); err != nil {
		entries, _ := fs.ReadDir(fsys, ".")
		for _, e := range entries {
			if e.IsDir() {
				if _, err := fs.Stat(fsys, e.Name()+"/manifest.json"); err == nil {
					sub, _ := fs.Sub(fsys, e.Name())
					return sub, closeFS, nil
				}
			}
		}
		closeFS()
		return nil, nil, command.Errorf("bad_params", "%s has no manifest.json: not a Xegesis export", p)
	}
	return fsys, closeFS, nil
}

func readFile(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err == nil && len(b) > maxFile {
		err = fmt.Errorf("%s is too large", name)
	}
	return b, err
}

func checkManifest(fsys fs.FS) error {
	b, err := readFile(fsys, "manifest.json")
	if err != nil {
		return err
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return command.Errorf("bad_params", "manifest.json: %v", err)
	}
	if m.Format != ManifestFormat {
		return command.Errorf("bad_params", "manifest.json: format is %q, not %q", m.Format, ManifestFormat)
	}
	if m.FormatVersion > FormatVersion {
		return command.Errorf("bad_params", "this export is format version %d; this Xegesis reads up to %d",
			m.FormatVersion, FormatVersion)
	}
	return nil
}

type object struct {
	t     objType
	file  string
	props map[string]json.RawMessage
	body  *string
}

type importer struct {
	env  *command.Env
	fsys fs.FS
	tx   *store.Tx
	ctx  context.Context
	res  ImportResult
}

// load reads the objects of the chosen groups, in an order that satisfies foreign keys.
func (im *importer) load(groups map[string]bool) ([]object, error) {
	var out []object
	types := []objType{itemType}
	for _, t := range objTypes {
		if groups[t.group] {
			types = append(types, t)
		}
	}
	for _, t := range types {
		objs, err := im.loadType(t)
		if err != nil {
			return nil, err
		}
		if t.name == "collection" || t.name == "folder" {
			sortCollections(objs)
		}
		out = append(out, objs...)
	}
	return out, nil
}

func (im *importer) loadType(t objType) ([]object, error) {
	dir := "objects/" + t.name
	entries, err := fs.ReadDir(im.fsys, dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var out []object
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		file := dir + "/" + e.Name()
		b, err := readFile(im.fsys, file)
		if err != nil {
			return nil, err
		}
		o := object{t: t, file: file}
		if err := json.Unmarshal(b, &o.props); err != nil {
			return nil, command.Errorf("bad_params", "%s: %v", file, err)
		}
		if md, err := readFile(im.fsys, strings.TrimSuffix(file, ".json")+".md"); err == nil {
			s := DecodeBody(string(md))
			o.body = &s
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

// sortCollections puts parents before children.
func sortCollections(objs []object) {
	parent := map[string]string{}
	for _, o := range objs {
		id, p := o.stringProp("id"), o.relationProp("parent")
		parent[id] = p
	}
	depth := func(id string) int {
		d := 0
		for p := parent[id]; p != "" && d < len(parent); p = parent[p] {
			d++
		}
		return d
	}
	sort.SliceStable(objs, func(i, j int) bool { return depth(objs[i].stringProp("id")) < depth(objs[j].stringProp("id")) })
}

func (o object) stringProp(name string) string {
	var s string
	json.Unmarshal(o.props[name], &s)
	return s
}

func (o object) relationProp(name string) string {
	var l []string
	json.Unmarshal(o.props[name], &l)
	if len(l) == 1 {
		return strings.TrimPrefix(l[0], anyURI)
	}
	return ""
}

// row turns an object's properties into a table row. A property that is absent leaves the column
// out; one that is null sets it to NULL.
func (o object) row() (store.Row, error) {
	row := store.Row{}
	for _, f := range o.t.fields {
		if f.kind == fBody {
			if o.body != nil {
				row[f.col] = *o.body
			} else {
				row[f.col] = ""
			}
			continue
		}
		raw, present := o.props[f.name]
		if !present {
			continue
		}
		v, err := decodeValue(f, raw)
		if err != nil {
			return nil, err
		}
		row[f.col] = v
	}
	for _, c := range o.t.key {
		if row[c] != nil {
			continue
		}
		if len(o.t.key) == 1 {
			if id := o.stringProp("id"); id != "" {
				row[c] = id
			}
		}
	}
	return row, nil
}

func (im *importer) apply(o object) error {
	row, err := o.row()
	if err != nil {
		return err
	}
	if o.t.anchor {
		if err := im.applyAnchor(o, row); err != nil {
			return err
		}
	}
	return im.upsert(o.t, row)
}

func (im *importer) applyAnchor(o object, row store.Row) error {
	raw, present := o.props["anchor_id"]
	if !present || string(raw) == "null" {
		if present {
			row["anchor_id"] = nil
		}
		return nil
	}
	var aid string
	if err := json.Unmarshal(raw, &aid); err != nil {
		return fmt.Errorf("anchor_id: %w", err)
	}
	a := store.Row{"id": aid}
	for _, f := range anchorFields {
		raw, ok := o.props[f.name]
		if !ok {
			continue
		}
		v, err := decodeValue(f, raw)
		if err != nil {
			return err
		}
		a[f.col] = v
	}
	if err := im.upsert(objType{name: "anchor", table: "anchors", key: []string{"id"}}, a); err != nil {
		return err
	}
	row["anchor_id"] = aid
	return nil
}

func keyOf(t objType, row store.Row) store.Row {
	k := store.Row{}
	for _, c := range t.key {
		k[c] = row[c]
	}
	return k
}

// upsert creates the row, or updates the existing row with the same key. Items are only created.
func (im *importer) upsert(t objType, row store.Row) error {
	key := keyOf(t, row)
	for c, v := range key {
		if v == nil {
			return fmt.Errorf("missing key %s", c)
		}
	}
	counter := t.name
	if t.table == "anchors" {
		counter = "anchor"
	}
	before, err := im.existing(t, key)
	if err != nil {
		return err
	}
	if before == nil {
		if err := im.tx.Create(t.table, row); err != nil {
			return err
		}
		im.res.Created[counter]++
		return nil
	}
	changed := store.Row{}
	for c, v := range row {
		if !isKeyCol(t, c) && !sameValue(before[c], v) {
			changed[c] = v
		}
	}
	if len(changed) == 0 || t.name == "item" {
		im.res.Unchanged[counter]++
		return nil
	}
	if err := im.tx.Update(t.table, key, changed); err != nil {
		return err
	}
	im.res.Updated[counter]++
	return nil
}

func isKeyCol(t objType, c string) bool {
	for _, k := range t.key {
		if k == c {
			return true
		}
	}
	return false
}

// existing returns the row with the key, or nil.
func (im *importer) existing(t objType, key store.Row) (store.Row, error) {
	r, err := im.tx.Get(t.table, key)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return r, err
}

func sameValue(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return bytes.Equal(ja, jb)
}
