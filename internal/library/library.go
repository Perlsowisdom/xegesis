// Package library manages library folders, imports the books in them and answers catalogue queries.
// Folders may live on external drives; each carries a marker file so it is found again after its
// drive letter or mount path changes (ADR 0006).
package library

import (
	"context"
	"strings"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// batchSize is how many files or items one write transaction handles.
const batchSize = 500

// Register adds the library commands to r.
func Register(r *command.Registry) {
	r.Add(
		command.Define("library.add_folder", "Add a folder of books to the library. The folder may be on an external drive.", true, addFolder),
		command.Define("library.folders", "List library folders, where each is and whether it is online.", false, listFolders),
		command.Define("library.rename_folder", "Change the name a library folder is shown under.", true, renameFolder),
		command.Define("library.remove_folder", "Remove a folder and its books from the library. Files on disk are left alone.", true, removeFolder),
		command.Define("library.relocate_folder", "Record that a library folder now lives at a new path.", true, relocateFolder),
		command.Define("library.locate", "Look for every library folder on mounted drives and record where each was found.", true, locate),
		command.Define("library.search_roots", "Show the extra paths searched for library folders that have moved.", false, searchRoots),
		command.Define("library.set_search_roots", "Set the extra paths searched for library folders that have moved.", true, setSearchRoots),
		command.Define("library.scan", "Find new, changed and removed book files in a folder and update the catalogue.", true, scan),
		command.Define("library.set_licence", "Record the licence the user chose for one or many items.", true, setLicence),
		command.Define("library.items", "List catalogue items, filtered and paged.", false, listItems),
		command.Define("library.item", "Show one item with its files, people, facets and licence.", false, getItem),
		command.Define("library.update_item", "Edit an item's catalogue details, authors and facets.", true, updateItem),
	)
}

func now() int64 { return time.Now().UnixMilli() }

func needStore(env *command.Env) error {
	if env.Store == nil {
		return command.Errorf("unavailable", "no library is open")
	}
	return nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(n int) any {
	if n == 0 {
		return nil
	}
	return int64(n)
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) int64 {
	n, _ := v.(int64)
	return n
}

// in returns "?, ?, ?" for n values and the values as query arguments.
func in[T any](vals []T) (string, []any) {
	args := make([]any, len(vals))
	for i, v := range vals {
		args[i] = v
	}
	return strings.TrimSuffix(strings.Repeat("?, ", len(vals)), ", "), args
}

func chunks[T any](s []T, n int) [][]T {
	var out [][]T
	for len(s) > n {
		out = append(out, s[:n])
		s = s[n:]
	}
	if len(s) > 0 {
		out = append(out, s)
	}
	return out
}

func likeArg(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}

// queryStrings runs a query returning one text column.
func queryStrings(ctx context.Context, s *store.Store, q string, args ...any) ([]string, error) {
	rows, err := s.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
