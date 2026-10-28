package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// LibraryFolderName is the folder in the user's Documents that added files are copied into.
const LibraryFolderName = "Xegesis Library"

// booksDir is where added files are copied when the caller names no folder. Tests replace it.
var booksDir = func() string { return filepath.Join(documentsDir(), LibraryFolderName) }

type AddParams struct {
	Paths []string `json:"paths" required:"true" desc:"Book files and folders. Folders are added, scanned and watched; files are copied into the library folder and scanned"`
	Into  string   `json:"into,omitempty" desc:"Folder to copy files into; Documents/Xegesis Library when omitted"`
}

type AddFilesParams struct {
	Files []string `json:"files" required:"true" desc:"Book files to copy into the library folder"`
	Into  string   `json:"into,omitempty" desc:"Folder to copy them into; Documents/Xegesis Library when omitted"`
}

// LicenceCount is how many of the new books have one licence.
type LicenceCount struct {
	Licence string `json:"licence" desc:"In plain words: Public domain, Free to share, Personal copy or My own work"`
	Origin  string `json:"origin" enum:"file,default,user" desc:"file: from the books' own details; default: none stated"`
	Count   int    `json:"count"`
}

type AddResult struct {
	Added    int            `json:"added" desc:"New books in the library"`
	Copied   int            `json:"copied" desc:"Files copied into the library folder"`
	Folders  []Folder       `json:"folders" desc:"Library folders that were added or scanned"`
	Licences []LicenceCount `json:"licences" desc:"The new books' licences"`
	Since    int64          `json:"since" desc:"Pass as added_since to library.items to list the books just added"`
	Errors   []FileError    `json:"errors" desc:"Paths that could not be added, and files that could not be read"`
	Summary  string         `json:"summary" desc:"One line saying what was added"`
}

func addFiles(ctx context.Context, env *command.Env, p AddFilesParams) (AddResult, error) {
	return add(ctx, env, AddParams{Paths: p.Files, Into: p.Into})
}

// add adds folders and files in one step. A folder, or a file, already inside a library folder is
// scanned where it is; other files are copied into the library folder.
func add(ctx context.Context, env *command.Env, p AddParams) (AddResult, error) {
	res := AddResult{Since: now(), Folders: []Folder{}, Licences: []LicenceCount{}, Errors: []FileError{}}
	if len(p.Paths) == 0 {
		return res, command.Errorf("bad_params", "give at least one file or folder in paths")
	}
	known, err := onlineFolders(ctx, env.Store)
	if err != nil {
		return res, err
	}
	var scanIDs, files []string
	want := func(id string) {
		for _, s := range scanIDs {
			if s == id {
				return
			}
		}
		scanIDs = append(scanIDs, id)
	}
	for _, raw := range p.Paths {
		if strings.TrimSpace(raw) == "" {
			res.Errors = append(res.Errors, FileError{RelPath: raw, Error: "empty path; give a file or folder"})
			continue
		}
		path, err := filepath.Abs(raw)
		st, serr := os.Stat(path)
		if err != nil || serr != nil {
			res.Errors = append(res.Errors, FileError{RelPath: raw, Error: "not found; check the path"})
			continue
		}
		if id := folderHolding(known, path); id != "" {
			want(id)
			continue
		}
		if !st.IsDir() {
			if formatOf(path) == "" {
				res.Errors = append(res.Errors, FileError{RelPath: raw,
					Error: "not a kind of book Xegesis can read (EPUB, PDF, Word, HTML, Markdown or plain text)"})
				continue
			}
			files = append(files, path)
			continue
		}
		id, err := addOrFind(ctx, env, path)
		if err != nil {
			return res, err
		}
		known[id] = path
		want(id)
	}
	if len(files) > 0 {
		id, dir, err := filesFolder(ctx, env, p.Into, known)
		if err != nil {
			return res, err
		}
		for _, src := range files {
			copied, err := copyInto(src, dir)
			if err != nil {
				res.Errors = append(res.Errors, FileError{RelPath: src, Error: "could not be copied: " + err.Error()})
				continue
			}
			if copied {
				res.Copied++
			}
		}
		want(id)
	}
	for _, id := range scanIDs {
		r, err := scan(ctx, env, ScanParams{FolderID: id})
		if err != nil {
			return res, err
		}
		f, err := liveFolder(ctx, env.Store, id)
		if err != nil {
			return res, err
		}
		res.Folders = append(res.Folders, f.folder(r.Path))
		for _, e := range r.Errors {
			res.Errors = append(res.Errors, FileError{RelPath: filepath.Join(r.Path, filepath.FromSlash(e.RelPath)), Error: e.Error})
		}
	}
	if res.Licences, err = licencesSince(ctx, env.Store, res.Since); err != nil {
		return res, err
	}
	for _, c := range res.Licences {
		res.Added += c.Count
	}
	res.Summary = summary(res)
	return res, nil
}

// onlineFolders maps live library folders that are online now to their paths.
func onlineFolders(ctx context.Context, s *store.Store) (map[string]string, error) {
	st, err := newStatus(ctx, s)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for id, dir := range st.paths {
		if dir != "" {
			out[id] = dir
		}
	}
	return out, nil
}

// folderHolding returns the library folder that path is in, or "".
func folderHolding(folders map[string]string, path string) string {
	best, bestLen := "", 0
	for id, dir := range folders {
		rel, err := filepath.Rel(dir, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if len(dir) > bestLen {
			best, bestLen = id, len(dir)
		}
	}
	return best
}

// filesFolder returns the folder files are copied into, creating and adding it on first use.
func filesFolder(ctx context.Context, env *command.Env, into string, known map[string]string) (string, string, error) {
	if strings.TrimSpace(into) == "" {
		into = booksDir()
	}
	dir, err := filepath.Abs(into)
	if err != nil {
		return "", "", command.Errorf("bad_params", "%v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", command.Errorf("failed", "cannot create the library folder %s: %v", dir, err)
	}
	if id := folderHolding(known, dir); id != "" {
		return id, dir, nil
	}
	id, err := addOrFind(ctx, env, dir)
	if err != nil {
		return "", "", err
	}
	known[id] = dir
	return id, dir, nil
}

// addOrFind adds a folder to the library. A folder that is already one, last seen elsewhere, is
// recorded at its new path instead.
func addOrFind(ctx context.Context, env *command.Env, dir string) (string, error) {
	if m, err := readMarker(dir); err == nil {
		rows, err := queryFolders(ctx, env.Store, "WHERE volume_id = ? AND deleted_at IS NULL", m.ID)
		if err != nil {
			return "", err
		}
		if len(rows) > 0 {
			return rows[0].id, recordSighting(ctx, env.Store, "library.add", rows[0], dir)
		}
	}
	f, err := addFolder(ctx, env, AddFolderParams{Path: dir})
	return f.ID, err
}

// copyInto copies a file into dir without overwriting anything: a name already taken gets a
// number, as in "Book (2).pdf". It reports false when an identical copy is already there.
func copyInto(src, dir string) (bool, error) {
	name := filepath.Base(src)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		target := filepath.Join(dir, name)
		if i > 1 {
			target = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		}
		if same(src, target) {
			return false, nil
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		if err := copyTo(out, src); err != nil {
			os.Remove(target)
			return false, err
		}
		return true, nil
	}
}

func copyTo(out *os.File, src string) error {
	in, err := os.Open(src)
	if err != nil {
		out.Close()
		return err
	}
	defer in.Close()
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// same reports whether two files have the same contents.
func same(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	if err1 != nil || err2 != nil || sa.Size() != sb.Size() {
		return false
	}
	ha, err1 := hashFile(a)
	hb, err2 := hashFile(b)
	return err1 == nil && err2 == nil && ha == hb
}

// licencesSince counts the licences of the books added at or after since.
func licencesSince(ctx context.Context, s *store.Store, since int64) ([]LicenceCount, error) {
	rows, err := queryRows(ctx, s, `SELECT l.type AS type, l.origin AS origin, l.share_allowed AS share,
		COUNT(*) AS n FROM items i LEFT JOIN licences l ON l.item_id = i.id
		WHERE i.deleted_at IS NULL AND i.created_at >= ? GROUP BY 1, 2, 3`, since)
	if err != nil {
		return nil, err
	}
	counts := map[[2]string]int{}
	for _, r := range rows {
		l := licence.Licence{Type: licence.Type(str(r["type"])), Share: num(r["share"]) == 1, Origin: str(r["origin"])}
		if l.Type == "" {
			l = licence.Default()
		}
		counts[[2]string{l.Plain(), l.Origin}] += int(num(r["n"]))
	}
	out := []LicenceCount{}
	for k, n := range counts {
		out = append(out, LicenceCount{Licence: k[0], Origin: k[1], Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Licence != out[j].Licence {
			return plainOrder[out[i].Licence] < plainOrder[out[j].Licence]
		}
		return originOrder[out[i].Origin] < originOrder[out[j].Origin]
	})
	return out, nil
}

var (
	plainOrder  = map[string]int{"Public domain": 0, "Free to share": 1, "Personal copy": 2, "My own work": 3}
	originOrder = map[string]int{licence.FromFile: 0, licence.ByDefault: 1, licence.ByUser: 2}
)

// summary is one line, e.g. "12 books added: 3 public domain (from their own details), 9 personal copies."
func summary(r AddResult) string {
	var b strings.Builder
	if r.Added == 0 {
		b.WriteString("No new books found.")
	} else {
		fmt.Fprintf(&b, "%s added: ", plural(r.Added, "book", "books"))
		for i, c := range r.Licences {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(countPhrase(c))
		}
		b.WriteString(".")
	}
	if n := len(r.Errors); n > 0 {
		fmt.Fprintf(&b, " %s could not be added; see the list for why.", plural(n, "file", "files"))
	}
	return b.String()
}

func countPhrase(c LicenceCount) string {
	var s string
	switch c.Licence {
	case "Personal copy":
		s = plural(c.Count, "personal copy", "personal copies")
	case "My own work":
		s = fmt.Sprintf("%d your own work", c.Count)
	default:
		s = fmt.Sprintf("%d %s", c.Count, strings.ToLower(c.Licence))
	}
	switch c.Origin {
	case licence.FromFile:
		if c.Count == 1 {
			return s + " (from its own details)"
		}
		return s + " (from their own details)"
	case licence.ByUser:
		return s + " (set by you)"
	}
	return s
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// Rescanning ------------------------------------------------------------------------------------

type RescanResult struct {
	Scanned int `json:"scanned" desc:"Online folders scanned"`
	Offline int `json:"offline" desc:"Folders skipped because their drive is not connected"`
	Added   int `json:"added"`
	Changed int `json:"changed"`
	Removed int `json:"removed"`
}

// rescan scans every online library folder. Unchanged files are skipped, so it is cheap to run
// often; the desktop app runs it at start-up and every ten minutes.
func rescan(ctx context.Context, env *command.Env, _ struct{}) (RescanResult, error) {
	var res RescanResult
	if err := needStore(env); err != nil {
		return res, err
	}
	rows, err := queryFolders(ctx, env.Store, "WHERE deleted_at IS NULL")
	if err != nil {
		return res, err
	}
	for _, f := range rows {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		r, err := scan(ctx, env, ScanParams{FolderID: f.id})
		var ce *command.Error
		if errors.As(err, &ce) && ce.Code == "unavailable" {
			res.Offline++
			continue
		}
		if err != nil {
			return res, err
		}
		res.Scanned++
		res.Added += r.Added
		res.Changed += r.Changed
		res.Removed += r.Removed
	}
	return res, nil
}
