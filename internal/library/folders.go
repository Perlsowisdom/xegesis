package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// MarkerName is the file written in the root of every library folder. It holds the folder's marker
// ID, which identifies the folder wherever its drive is mounted.
const MarkerName = ".xegesis-library.json"

const rootsKey = "library.search_roots"

// seenEvery is how stale last_seen_at may get before library.locate records a fresh sighting.
const seenEvery = 24 * 60 * 60 * 1000

type marker struct {
	Format int    `json:"xegesis_library"`
	ID     string `json:"id"`
	Label  string `json:"label,omitempty"`
}

func readMarker(dir string) (marker, error) {
	b, err := os.ReadFile(filepath.Join(dir, MarkerName))
	if err != nil {
		return marker{}, err
	}
	var m marker
	if err := json.Unmarshal(b, &m); err != nil || m.ID == "" {
		return marker{}, fmt.Errorf("%s is not a library marker", filepath.Join(dir, MarkerName))
	}
	return m, nil
}

func writeMarker(dir string, m marker) error {
	m.Format = 1
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(dir, MarkerName), append(b, '\n'), 0o644)
}

func hasMarker(dir, id string) bool {
	m, err := readMarker(dir)
	return err == nil && m.ID == id
}

// Folder is a library folder as shown to callers.
type Folder struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	MarkerID   string `json:"marker_id" desc:"ID in the folder's marker file"`
	Path       string `json:"path" desc:"Where the folder is now, or where it was last seen if offline"`
	Online     bool   `json:"online"`
	LastSeenAt int64  `json:"last_seen_at,omitempty"`
}

// folderRow is a library_folders row. volume_id holds the marker ID and path_on_volume the full
// path the folder was last found at.
type folderRow struct {
	id, label, markerID, path string
	lastSeen                  int64
	deleted                   bool
}

func (f folderRow) folder(path string) Folder {
	out := Folder{ID: f.id, Label: f.label, MarkerID: f.markerID, Path: f.path, LastSeenAt: f.lastSeen}
	if path != "" {
		out.Path, out.Online = path, true
	}
	return out
}

const folderSelect = `SELECT id, label, volume_id, path_on_volume, COALESCE(last_seen_at, 0),
	deleted_at IS NOT NULL FROM library_folders`

func queryFolders(ctx context.Context, s *store.Store, where string, args ...any) ([]folderRow, error) {
	rows, err := s.Query(ctx, folderSelect+" "+where+" ORDER BY label, id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []folderRow
	for rows.Next() {
		var f folderRow
		if err := rows.Scan(&f.id, &f.label, &f.markerID, &f.path, &f.lastSeen, &f.deleted); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func liveFolder(ctx context.Context, s *store.Store, id string) (folderRow, error) {
	rows, err := queryFolders(ctx, s, "WHERE id = ? AND deleted_at IS NULL", id)
	if err != nil {
		return folderRow{}, err
	}
	if len(rows) == 0 {
		return folderRow{}, command.Errorf("not_found", "no library folder %s", id)
	}
	return rows[0], nil
}

// locator finds folders by their markers: at the last known path, at the same path on every mounted
// volume (a changed drive letter or mount point), then under the user's search roots.
type locator struct {
	roots []string
	deep  bool // also search the user's roots; slower, so catalogue queries skip it
}

func newLocator(ctx context.Context, s *store.Store, deep bool) (*locator, error) {
	roots, err := loadRoots(ctx, s)
	return &locator{roots: roots, deep: deep}, err
}

// find returns the folder's current path, or "" if it is offline.
func (l *locator) find(f folderRow) string {
	if hasMarker(f.path, f.markerID) {
		return f.path
	}
	rel := strings.TrimLeft(strings.TrimPrefix(f.path, volumeOf(f.path)), `/\`)
	for _, root := range mountedRoots() {
		if p := filepath.Join(root, rel); hasMarker(p, f.markerID) {
			return p
		}
	}
	if !l.deep {
		return ""
	}
	for _, root := range l.roots {
		if p := searchUnder(root, f.markerID, 2); p != "" {
			return p
		}
	}
	return ""
}

// searchUnder looks for the marker in dir and its subfolders down to depth levels.
func searchUnder(dir, id string, depth int) string {
	if hasMarker(dir, id) {
		return dir
	}
	if depth == 0 {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			if p := searchUnder(filepath.Join(dir, e.Name()), id, depth-1); p != "" {
				return p
			}
		}
	}
	return ""
}

func absDir(path string) (string, error) {
	p, err := filepath.Abs(path)
	if err != nil {
		return "", command.Errorf("bad_params", "%v", err)
	}
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		return "", command.Errorf("bad_params", "%s is not a folder", p)
	}
	return p, nil
}

// Commands -------------------------------------------------------------------------------------

type AddFolderParams struct {
	Path  string `json:"path" required:"true" desc:"Path of the folder"`
	Label string `json:"label,omitempty" desc:"Name shown in the library; defaults to the folder's name"`
}

func addFolder(ctx context.Context, env *command.Env, p AddFolderParams) (Folder, error) {
	path, err := absDir(p.Path)
	if err != nil {
		return Folder{}, err
	}
	label := p.Label
	if label == "" {
		label = filepath.Base(path)
	}
	m, err := readMarker(path)
	if errors.Is(err, fs.ErrNotExist) {
		m, err = marker{ID: store.NewID()}, nil
	}
	if err != nil {
		return Folder{}, command.Errorf("bad_params", "%v", err)
	}
	known, err := queryFolders(ctx, env.Store, "WHERE volume_id = ?", m.ID)
	if err != nil {
		return Folder{}, err
	}
	if len(known) > 0 && !known[0].deleted {
		return Folder{}, command.Errorf("bad_params", "%s is already library folder %s", path, known[0].id)
	}
	m.Label = label
	if err := writeMarker(path, m); err != nil {
		return Folder{}, command.Errorf("failed", "cannot write the folder marker: %v", err)
	}
	f := folderRow{id: store.NewID(), label: label, markerID: m.ID, path: path, lastSeen: now()}
	err = env.Store.Write(ctx, "library.add_folder", func(tx *store.Tx) error {
		if len(known) > 0 { // re-adding a removed folder brings back its history
			f.id = known[0].id
			return tx.Update("library_folders", store.ByID(f.id), store.Row{"label": label,
				"path_on_volume": path, "last_seen_at": f.lastSeen, "deleted_at": nil})
		}
		return tx.Create("library_folders", store.Row{"id": f.id, "label": label, "volume_id": m.ID,
			"path_on_volume": path, "last_seen_at": f.lastSeen})
	})
	return f.folder(path), err
}

func listFolders(ctx context.Context, env *command.Env, _ struct{}) ([]Folder, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	rows, err := queryFolders(ctx, env.Store, "WHERE deleted_at IS NULL")
	if err != nil {
		return nil, err
	}
	l, err := newLocator(ctx, env.Store, true)
	if err != nil {
		return nil, err
	}
	out := []Folder{}
	for _, f := range rows {
		out = append(out, f.folder(l.find(f)))
	}
	return out, nil
}

type RenameFolderParams struct {
	ID    string `json:"id" required:"true"`
	Label string `json:"label" required:"true" desc:"New name"`
}

func renameFolder(ctx context.Context, env *command.Env, p RenameFolderParams) (Folder, error) {
	f, err := liveFolder(ctx, env.Store, p.ID)
	if err != nil {
		return Folder{}, err
	}
	err = env.Store.Write(ctx, "library.rename_folder", func(tx *store.Tx) error {
		return tx.Update("library_folders", store.ByID(f.id), store.Row{"label": p.Label})
	})
	if err != nil {
		return Folder{}, err
	}
	f.label = p.Label
	l, _ := newLocator(ctx, env.Store, false)
	path := l.find(f)
	if path != "" {
		writeMarker(path, marker{ID: f.markerID, Label: p.Label}) // best effort; the database is authoritative
	}
	return f.folder(path), nil
}

type IDParams struct {
	ID string `json:"id" required:"true"`
}

type RemoveResult struct {
	Files int `json:"files" desc:"File records removed"`
	Items int `json:"items" desc:"Items removed because they had no other files"`
}

func removeFolder(ctx context.Context, env *command.Env, p IDParams) (RemoveResult, error) {
	f, err := liveFolder(ctx, env.Store, p.ID)
	if err != nil {
		return RemoveResult{}, err
	}
	files, err := liveFiles(ctx, env.Store, f.id)
	if err != nil {
		return RemoveResult{}, err
	}
	var res RemoveResult
	for _, batch := range chunks(files, batchSize) {
		n, err := deleteFiles(ctx, env.Store, "library.remove_folder", batch)
		if err != nil {
			return res, err
		}
		res.Files += len(batch)
		res.Items += n
	}
	err = env.Store.Write(ctx, "library.remove_folder", func(tx *store.Tx) error {
		return tx.Delete("library_folders", store.ByID(f.id))
	})
	return res, err
}

type RelocateParams struct {
	ID    string `json:"id" required:"true"`
	Path  string `json:"path" required:"true" desc:"The folder's new path"`
	Force bool   `json:"force,omitempty" desc:"Adopt a folder that has no marker file, writing one"`
}

func relocateFolder(ctx context.Context, env *command.Env, p RelocateParams) (Folder, error) {
	f, err := liveFolder(ctx, env.Store, p.ID)
	if err != nil {
		return Folder{}, err
	}
	path, err := absDir(p.Path)
	if err != nil {
		return Folder{}, err
	}
	m, err := readMarker(path)
	switch {
	case err == nil && m.ID != f.markerID:
		return Folder{}, command.Errorf("bad_params", "%s holds a different library folder (%s)", path, m.ID)
	case err != nil && !p.Force:
		return Folder{}, command.Errorf("bad_params", "%s has no library marker; pass force to adopt it", path)
	case err != nil:
		if err := writeMarker(path, marker{ID: f.markerID, Label: f.label}); err != nil {
			return Folder{}, command.Errorf("failed", "cannot write the folder marker: %v", err)
		}
	}
	return f.folder(path), recordSighting(ctx, env.Store, "library.relocate_folder", f, path)
}

func recordSighting(ctx context.Context, s *store.Store, cmd string, f folderRow, path string) error {
	t := now()
	if path == f.path && t-f.lastSeen < seenEvery {
		return nil
	}
	return s.Write(ctx, cmd, func(tx *store.Tx) error {
		return tx.Update("library_folders", store.ByID(f.id), store.Row{"path_on_volume": path, "last_seen_at": t})
	})
}

func locate(ctx context.Context, env *command.Env, _ struct{}) ([]Folder, error) {
	rows, err := queryFolders(ctx, env.Store, "WHERE deleted_at IS NULL")
	if err != nil {
		return nil, err
	}
	l, err := newLocator(ctx, env.Store, true)
	if err != nil {
		return nil, err
	}
	out := []Folder{}
	for _, f := range rows {
		path := l.find(f)
		if path != "" {
			if err := recordSighting(ctx, env.Store, "library.locate", f, path); err != nil {
				return nil, err
			}
		}
		out = append(out, f.folder(path))
	}
	return out, nil
}

// Search roots ---------------------------------------------------------------------------------

func loadRoots(ctx context.Context, s *store.Store) ([]string, error) {
	var raw string
	err := s.QueryRow(ctx, `SELECT value_json FROM settings WHERE key = ?`, rootsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var roots []string
	return roots, json.Unmarshal([]byte(raw), &roots)
}

func putSetting(tx *store.Tx, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	k := store.Row{"key": key}
	if _, err := tx.Get("settings", k); errors.Is(err, store.ErrNotFound) {
		return tx.Create("settings", store.Row{"key": key, "value_json": string(b)})
	} else if err != nil {
		return err
	}
	return tx.Update("settings", k, store.Row{"value_json": string(b)})
}

func searchRoots(ctx context.Context, env *command.Env, _ struct{}) ([]string, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	return loadRoots(ctx, env.Store)
}

type SearchRootsParams struct {
	Roots []string `json:"roots" desc:"Paths to search, two levels deep, for folders that have moved"`
}

func setSearchRoots(ctx context.Context, env *command.Env, p SearchRootsParams) ([]string, error) {
	roots := []string{}
	for _, r := range p.Roots {
		abs, err := filepath.Abs(r)
		if err != nil {
			return nil, command.Errorf("bad_params", "%v", err)
		}
		roots = append(roots, abs)
	}
	return roots, env.Store.Write(ctx, "library.set_search_roots", func(tx *store.Tx) error {
		return putSetting(tx, rootsKey, roots)
	})
}
