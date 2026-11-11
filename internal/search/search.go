// Package search indexes the text of library books and searches it. Each library folder has its
// own index file, index/<folder-id>.db next to library.db, kept on the internal drive so books on an
// unplugged drive stay searchable (ADR 0006). Indexing runs in the background, a few files at a
// time, and resumes where it stopped.
package search

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// Register adds the search commands to r. dataDir is the folder holding library.db.
func Register(r *command.Registry, dataDir string) { register(r, newService(r, dataDir)) }

func register(r *command.Registry, s *service) {
	r.OnClose(s.closeAll)
	r.Add(
		command.Define("search.index", "Index the text of books in one library folder or all of them, resuming where indexing stopped.", false, s.indexCmd),
		command.Define("search.status", "Show how far each library folder's search index has got.", false, s.statusCmd),
		command.Define("search.cancel", "Stop indexing one library folder or all of them. Indexing resumes with search.index.", false, s.cancelCmd),
		command.Define("search.rebuild", "Delete one library folder's search index and index the folder again.", false, s.rebuildCmd),
		command.Define("search.query", "Search the text of every indexed book, best matches first.", false, s.queryCmd),
		command.Define("search.refs", "Find passages in books that cite a Bible reference.", false, s.refsCmd),
	)
}

// service holds the open index files and the running indexing jobs.
type service struct {
	reg     *command.Registry
	dir     string
	workers int
	onFile  func(folderID string) // called after each file is indexed; tests use it

	mu      sync.Mutex
	indexes map[string]*index
	jobs    map[string]*job // by folder ID
}

func newService(r *command.Registry, dataDir string) *service {
	return &service{reg: r, dir: filepath.Join(dataDir, "index"), workers: min(4, runtime.NumCPU()),
		indexes: map[string]*index{}, jobs: map[string]*job{}}
}

var validFolderID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func (s *service) path(folderID string) string { return filepath.Join(s.dir, folderID+".db") }

// index returns the folder's open index, creating the file if create is set. It returns nil, nil
// when the folder has no index and create is not set.
func (s *service) index(folderID string, create bool) (*index, error) {
	if !validFolderID.MatchString(folderID) {
		return nil, fmt.Errorf("bad folder ID %q", folderID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ix := s.indexes[folderID]; ix != nil {
		return ix, nil
	}
	p := s.path(folderID)
	if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) && !create {
		return nil, nil
	}
	ix, err := openIndex(p, folderID)
	if err != nil {
		return nil, err
	}
	s.indexes[folderID] = ix
	return ix, nil
}

// closeAll stops every run and closes every index.
func (s *service) closeAll() {
	s.mu.Lock()
	jobs := make([]*job, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	s.mu.Unlock()
	for _, j := range jobs {
		j.cancel()
		<-j.done
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, ix := range s.indexes {
		ix.close()
		delete(s.indexes, id)
	}
}

// drop closes and deletes a folder's index file.
func (s *service) drop(folderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ix := s.indexes[folderID]; ix != nil {
		ix.close()
		delete(s.indexes, folderID)
	}
	return removeIndexFiles(s.path(folderID))
}

// folders lists the live library folders and whether each is online.
func (s *service) folders(ctx context.Context, env *command.Env) ([]library.Folder, error) {
	if env.Store == nil {
		return nil, command.Errorf("unavailable", "no library is open")
	}
	res, err := s.reg.Run(ctx, env, "library.folders", nil)
	if err != nil {
		return nil, err
	}
	list, ok := res.([]library.Folder)
	if !ok {
		return nil, fmt.Errorf("library.folders returned %T", res)
	}
	return list, nil
}

// pick returns the folders with the given IDs, or all of them when ids is empty.
func pick(all []library.Folder, ids ...string) ([]library.Folder, error) {
	if len(ids) == 0 {
		return all, nil
	}
	var out []library.Folder
	for _, id := range ids {
		found := false
		for _, f := range all {
			if f.ID == id {
				out, found = append(out, f), true
			}
		}
		if !found {
			return nil, command.Errorf("not_found", "no library folder %s", id)
		}
	}
	return out, nil
}

// libFile is a live files row in library.db.
type libFile struct {
	id, itemID, rel, format, sha string
	size                         int64
}

func libFiles(ctx context.Context, st *store.Store, folderID string) ([]libFile, error) {
	rows, err := st.Query(ctx, `SELECT id, item_id, rel_path, format, COALESCE(sha256, ''), COALESCE(size_bytes, 0)
		FROM files WHERE folder_id = ? AND deleted_at IS NULL ORDER BY rel_path`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []libFile
	for rows.Next() {
		var f libFile
		if err := rows.Scan(&f.id, &f.itemID, &f.rel, &f.format, &f.sha, &f.size); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// current reports whether the index holds this version of the file.
func current(known map[string]indexedFile, f libFile) bool {
	k, ok := known[f.id]
	return ok && k.sha == f.sha && k.size == f.size
}
