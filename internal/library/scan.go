package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// scanMu lets one scan run at a time, so the timed rescan and a scan the user asked for never
// both add the same new file.
var scanMu sync.Mutex

type ScanParams struct {
	FolderID string `json:"folder_id" required:"true"`
	Verify   bool   `json:"verify,omitempty" desc:"Re-hash every file, even those whose size and time are unchanged"`
}

type FileError struct {
	RelPath string `json:"rel_path"`
	Error   string `json:"error"`
}

type ScanResult struct {
	FolderID  string      `json:"folder_id"`
	Path      string      `json:"path"`
	Added     int         `json:"added"`
	Changed   int         `json:"changed"`
	Unchanged int         `json:"unchanged"`
	Moved     int         `json:"moved" desc:"Files renamed or moved within the folder"`
	Restored  int         `json:"restored" desc:"Files that had vanished and are back"`
	Removed   int         `json:"removed"`
	Errors    []FileError `json:"errors"`
	Typed     int         `json:"typed" desc:"New books given a type automatically"`
	Licensed  int         `json:"licensed" desc:"Books given a licence: the one their own details name, or Personal copy"`
}

// fileRow is a files row, as far as scanning needs it.
type fileRow struct {
	id, itemID, rel, sha string
	size                 int64
	deleted              bool
}

type diskFile struct {
	rel, abs, format string
	size, mtime      int64
}

// job is a file that must be read: new, or possibly changed.
type job struct {
	disk  diskFile
	known *fileRow
	sha   string
	meta  Meta
	err   error
}

type scanner struct {
	ctx    context.Context
	s      *store.Store
	folder folderRow
	res    ScanResult
	people map[string]string // name -> person ID
	gone   map[string][]*fileRow
	newIDs []string // items this scan made
}

func scan(ctx context.Context, env *command.Env, p ScanParams) (ScanResult, error) {
	scanMu.Lock()
	defer scanMu.Unlock()
	f, err := liveFolder(ctx, env.Store, p.FolderID)
	if err != nil {
		return ScanResult{}, err
	}
	l, err := newLocator(ctx, env.Store, true)
	if err != nil {
		return ScanResult{}, err
	}
	dir := l.find(f)
	if dir == "" {
		return ScanResult{}, command.Errorf("unavailable", "library folder %q is offline (last seen at %s); "+
			"connect its drive, or use library.relocate_folder if it has moved", f.label, f.path)
	}
	start := now()
	since, err := lastScan(ctx, env.Store, f.id)
	if err != nil {
		return ScanResult{}, err
	}
	sc := &scanner{ctx: ctx, s: env.Store, folder: f, people: map[string]string{},
		res: ScanResult{FolderID: f.id, Path: dir, Errors: []FileError{}}}
	if err := sc.run(dir, since, p.Verify); err != nil {
		return sc.res, err
	}
	err = env.Store.Write(ctx, "library.scan", func(tx *store.Tx) error {
		if err := putSetting(tx, scanKey(f.id), start); err != nil {
			return err
		}
		return tx.Update("library_folders", store.ByID(f.id), store.Row{"path_on_volume": dir, "last_seen_at": start})
	})
	if err != nil {
		return sc.res, err
	}
	if sc.res.Licensed, err = fillDefaults(ctx, env.Store, "library.scan", f.id); err != nil {
		return sc.res, err
	}
	applied, err := autoNew(ctx, env.Store, "library.scan", sc.newIDs)
	sc.res.Typed = applied.Typed
	return sc.res, err
}

func scanKey(folderID string) string { return "library.scanned_at." + folderID }

// lastScan is when the folder's last completed scan started; files modified after it are re-read.
func lastScan(ctx context.Context, s *store.Store, folderID string) (int64, error) {
	var t int64
	err := s.QueryRow(ctx, `SELECT CAST(value_json AS INTEGER) FROM settings WHERE key = ?`, scanKey(folderID)).Scan(&t)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return t, err
}

func (sc *scanner) run(dir string, since int64, verify bool) error {
	known, err := sc.knownFiles()
	if err != nil {
		return err
	}
	disk := sc.walk(dir)
	var jobs []*job
	seen := map[string]bool{}
	for _, d := range disk {
		seen[d.rel] = true
		k := known[d.rel]
		if k != nil && !k.deleted && !verify && k.size == d.size && d.mtime < since {
			sc.res.Unchanged++
			continue
		}
		jobs = append(jobs, &job{disk: d, known: k})
	}
	sc.gone = map[string][]*fileRow{}
	for rel, k := range known {
		if !seen[rel] && !k.deleted {
			sc.gone[k.sha] = append(sc.gone[k.sha], k)
		}
	}
	for _, batch := range chunks(jobs, batchSize) {
		if err := sc.ctx.Err(); err != nil {
			return err
		}
		if err := sc.apply(batch); err != nil {
			return err
		}
	}
	var vanished []fileRow
	for _, rows := range sc.gone {
		for _, k := range rows {
			vanished = append(vanished, *k)
		}
	}
	for _, batch := range chunks(vanished, batchSize) {
		if _, err := deleteFiles(sc.ctx, sc.s, "library.scan", batch); err != nil {
			return err
		}
		sc.res.Removed += len(batch)
	}
	return nil
}

func (sc *scanner) knownFiles() (map[string]*fileRow, error) {
	rows, err := sc.s.Query(sc.ctx, `SELECT id, item_id, rel_path, COALESCE(sha256, ''), COALESCE(size_bytes, -1),
		deleted_at IS NOT NULL FROM files WHERE folder_id = ?`, sc.folder.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*fileRow{}
	for rows.Next() {
		f := &fileRow{}
		if err := rows.Scan(&f.id, &f.itemID, &f.rel, &f.sha, &f.size, &f.deleted); err != nil {
			return nil, err
		}
		out[f.rel] = f
	}
	return out, rows.Err()
}

// walk lists the book files under dir, skipping hidden files and folders.
func (sc *scanner) walk(dir string) []diskFile {
	var out []diskFile
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if err != nil {
			sc.res.Errors = append(sc.res.Errors, FileError{RelPath: rel, Error: err.Error()})
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && p != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		format := formatOf(d.Name())
		if d.IsDir() || format == "" || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			sc.res.Errors = append(sc.res.Errors, FileError{RelPath: rel, Error: err.Error()})
			return nil
		}
		out = append(out, diskFile{rel: rel, abs: p, format: format, size: info.Size(), mtime: info.ModTime().UnixMilli()})
		return nil
	})
	return out
}

// read hashes the batch's files in parallel and reads metadata from new ones.
func read(batch []*job) {
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for _, j := range batch {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			if j.sha, j.err = hashFile(j.disk.abs); j.err == nil && j.known == nil {
				j.meta, _ = extract(j.disk.abs, j.disk.format) // unreadable metadata falls back to the file name
			}
		}()
	}
	wg.Wait()
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (sc *scanner) apply(batch []*job) error {
	read(batch)
	if err := sc.loadPeople(batch); err != nil {
		return err
	}
	added := 0
	err := sc.s.Write(sc.ctx, "library.scan", func(tx *store.Tx) error {
		added = 0
		for _, j := range batch {
			if j.err != nil {
				continue
			}
			isNew, err := sc.applyOne(tx, j)
			if err != nil {
				return err
			}
			if isNew {
				added++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, j := range batch {
		if j.err != nil {
			sc.res.Errors = append(sc.res.Errors, FileError{RelPath: j.disk.rel, Error: j.err.Error()})
		}
	}
	sc.res.Added += added
	return nil
}

// applyOne records one read file and reports whether it made a new item.
func (sc *scanner) applyOne(tx *store.Tx, j *job) (bool, error) {
	d := j.disk
	if j.known == nil {
		if rows := sc.gone[j.sha]; len(rows) > 0 && j.sha != "" {
			k := rows[0]
			sc.gone[j.sha] = rows[1:]
			sc.res.Moved++
			return false, tx.Update("files", store.ByID(k.id), store.Row{"rel_path": d.rel})
		}
		return true, sc.createItem(tx, j)
	}
	k := j.known
	fields := store.Row{"size_bytes": d.size}
	switch {
	case k.deleted:
		fields["deleted_at"] = nil
		sc.res.Restored++
		if err := restoreItem(tx, k.itemID); err != nil {
			return false, err
		}
	case j.sha != k.sha:
		sc.res.Changed++
	default:
		sc.res.Unchanged++
	}
	if j.sha != k.sha {
		fields["sha256"], fields["indexed_at"] = j.sha, nil
	}
	return false, tx.Update("files", store.ByID(k.id), fields)
}

func restoreItem(tx *store.Tx, id string) error {
	it, err := tx.Get("items", store.ByID(id))
	if err != nil || it["deleted_at"] == nil {
		return err
	}
	return tx.Update("items", store.ByID(id), store.Row{"deleted_at": nil, "updated_at": now()})
}

func (sc *scanner) createItem(tx *store.Tx, j *job) error {
	m, id, t := j.meta, store.NewID(), now()
	sc.newIDs = append(sc.newIDs, id)
	err := tx.Create("items", store.Row{"id": id, "kind": "book", "title": m.Title,
		"subtitle": nullable(m.Subtitle), "language": nullable(m.Language), "year": nullInt(m.Year),
		"publisher": nullable(m.Publisher), "series": nullable(m.Series), "series_no": nullable(m.SeriesNo),
		"isbn": nullable(m.ISBN), "source": "import", "created_at": t, "updated_at": t})
	if err != nil {
		return err
	}
	err = tx.Create("files", store.Row{"id": store.NewID(), "item_id": id, "folder_id": sc.folder.id,
		"rel_path": j.disk.rel, "format": j.disk.format, "size_bytes": j.disk.size, "sha256": j.sha})
	if err != nil {
		return err
	}
	if err := sc.addPeople(tx, id, m.People); err != nil {
		return err
	}
	facets := []Facet{}
	for _, s := range m.Subjects {
		facets = append(facets, Facet{Facet: "subject", Value: s})
	}
	if m.Rights != "" {
		facets = append(facets, Facet{Facet: "rights", Value: m.Rights})
	}
	for _, f := range facets {
		err := tx.Create("item_facets", store.Row{"item_id": id, "facet": f.Facet, "value": f.Value, "origin": "metadata"})
		if err != nil {
			return err
		}
	}
	return nil
}

func (sc *scanner) loadPeople(batch []*job) error {
	var names []string
	for _, j := range batch {
		for _, p := range j.meta.People {
			if _, ok := sc.people[p.Name]; !ok {
				names = append(names, p.Name)
			}
		}
	}
	found, err := peopleByName(sc.ctx, sc.s, names)
	for k, v := range found {
		sc.people[k] = v
	}
	return err
}

func (sc *scanner) addPeople(tx *store.Tx, itemID string, people []Person) error {
	for i, p := range people {
		pid, err := personID(tx, sc.people, p)
		if err != nil {
			return err
		}
		err = tx.Create("item_people", store.Row{"item_id": itemID, "person_id": pid, "role": p.Role, "position": int64(i)})
		if err != nil {
			return err
		}
	}
	return nil
}

// personID returns the ID of the person with p's name, creating them if needed. cache maps names
// to IDs and must already hold every existing person the caller may name.
func personID(tx *store.Tx, cache map[string]string, p Person) (string, error) {
	if id, ok := cache[p.Name]; ok {
		return id, nil
	}
	id := store.NewID()
	if p.SortName == "" {
		p.SortName = sortName(p.Name)
	}
	if err := tx.Create("people", store.Row{"id": id, "name": p.Name, "sort_name": p.SortName}); err != nil {
		return "", err
	}
	cache[p.Name] = id
	return id, nil
}

func peopleByName(ctx context.Context, s *store.Store, names []string) (map[string]string, error) {
	out := map[string]string{}
	for _, batch := range chunks(names, batchSize) {
		marks, args := in(batch)
		rows, err := s.Query(ctx, `SELECT name, MIN(id) FROM people WHERE name IN (`+marks+`) GROUP BY name`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var name, id string
			if err := rows.Scan(&name, &id); err != nil {
				rows.Close()
				return nil, err
			}
			out[name] = id
		}
		rows.Close()
	}
	return out, nil
}

func liveFiles(ctx context.Context, s *store.Store, folderID string) ([]fileRow, error) {
	rows, err := s.Query(ctx, `SELECT id, item_id FROM files WHERE folder_id = ? AND deleted_at IS NULL`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fileRow
	for rows.Next() {
		var f fileRow
		if err := rows.Scan(&f.id, &f.itemID); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// deleteFiles soft-deletes files, and their items when no other live file remains. It returns how
// many items it deleted.
func deleteFiles(ctx context.Context, s *store.Store, cmd string, files []fileRow) (int, error) {
	ids := make([]string, len(files))
	for i, f := range files {
		ids[i] = f.itemID
	}
	marks, args := in(ids)
	live := map[string]int{}
	rows, err := s.Query(ctx, `SELECT item_id, COUNT(*) FROM files WHERE deleted_at IS NULL AND item_id IN (`+marks+`)
		GROUP BY item_id`, args...)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			rows.Close()
			return 0, err
		}
		live[id] = n
	}
	rows.Close()
	items := 0
	err = s.Write(ctx, cmd, func(tx *store.Tx) error {
		items = 0
		for _, f := range files {
			if err := tx.Delete("files", store.ByID(f.id)); err != nil {
				return err
			}
			if live[f.itemID]--; live[f.itemID] == 0 {
				if err := tx.Delete("items", store.ByID(f.itemID)); err != nil {
					return err
				}
				items++
			}
		}
		return nil
	})
	return items, err
}
