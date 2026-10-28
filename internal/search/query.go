package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// maxPerFolder caps the hits taken from one folder's index before merging; paging stops there.
const maxPerFolder = 1000

// Filter narrows a search to some books.
type Filter struct {
	ItemIDs   []string `json:"item_ids,omitempty" desc:"Only these items"`
	Kind      string   `json:"kind,omitempty" desc:"Only items of this kind, e.g. book, commentary"`
	Author    string   `json:"author,omitempty" desc:"Only items crediting a person whose name contains this text"`
	FolderIDs []string `json:"folder_ids,omitempty" desc:"Only books in these library folders"`
}

// Hit is one matching passage.
type Hit struct {
	PassageID int64    `json:"passage_id"`
	ItemID    string   `json:"item_id"`
	Title     string   `json:"title"`
	Kind      string   `json:"kind"`
	Authors   []string `json:"authors"`
	FileID    string   `json:"file_id"`
	RelPath   string   `json:"rel_path" desc:"The file's path within its library folder"`
	Format    string   `json:"format"`
	FolderID  string   `json:"folder_id"`
	Online    bool     `json:"online" desc:"False when the book's folder is on a drive that is not mounted"`
	Heading   string   `json:"heading,omitempty" desc:"The nearest heading before the passage"`
	Locator   Locator  `json:"locator" desc:"Where the passage starts in the file"`
	Snippet   string   `json:"snippet" desc:"Text around the match, with matches wrapped in <mark></mark>"`
	Score     float64  `json:"score,omitempty" desc:"search.query: higher is better"`
	Cites     string   `json:"cites,omitempty" desc:"search.refs: the reference the passage cites"`
}

type QueryParams struct {
	Query string `json:"query" required:"true" desc:"Words to find"`
	Mode  string `json:"mode,omitempty" enum:"all,any,phrase,fts" desc:"all words (default), any word, the exact phrase, or raw FTS5 syntax"`
	Filter
	Limit  int `json:"limit,omitempty" desc:"Most hits to return (default 20, at most 200)"`
	Offset int `json:"offset,omitempty" desc:"Hits to skip, for paging"`
}

type QueryResult struct {
	Hits       []Hit    `json:"hits"`
	More       bool     `json:"more" desc:"More hits follow this page"`
	Incomplete []string `json:"incomplete" desc:"Folders not indexed yet or being indexed, so their books may be missing"`
}

// rawHit is a hit as read from one index.
type rawHit struct {
	Hit
	locator string
	width   int // search.refs: verses in the cited range
	text    string
}

func (s *service) queryCmd(ctx context.Context, env *command.Env, p QueryParams) (QueryResult, error) {
	match := ftsQuery(p.Query, p.Mode)
	if strings.Trim(match, `" `) == "" {
		return QueryResult{}, command.Errorf("bad_params", "search.query: query has no words")
	}
	return s.merged(ctx, env, p.Filter, p.Limit, p.Offset, func(ix *index, items []string, n int) ([]rawHit, error) {
		return ix.search(ctx, match, items, n)
	}, func(a, b rawHit) bool { return a.Score > b.Score })
}

type RefsParams struct {
	Ref string `json:"ref" required:"true" desc:"A Bible reference, e.g. \"Rom 8:28\" or \"John 3\""`
	Filter
	Limit  int `json:"limit,omitempty" desc:"Most hits to return (default 20, at most 200)"`
	Offset int `json:"offset,omitempty" desc:"Hits to skip, for paging"`
}

// refsCmd returns passages whose citations overlap the reference, closest citations first.
func (s *service) refsCmd(ctx context.Context, env *command.Env, p RefsParams) (QueryResult, error) {
	r, err := verse.Parse(p.Ref)
	if err != nil {
		return QueryResult{}, command.Errorf("bad_params", "search.refs: %v", err)
	}
	res, err := s.merged(ctx, env, p.Filter, p.Limit, p.Offset, func(ix *index, items []string, n int) ([]rawHit, error) {
		return ix.refs(ctx, r, items, n)
	}, func(a, b rawHit) bool {
		if a.width != b.width {
			return a.width < b.width
		}
		if a.PassageID != b.PassageID {
			return a.PassageID < b.PassageID
		}
		return a.FolderID < b.FolderID
	})
	return res, err
}

type searchFn func(ix *index, items []string, limit int) ([]rawHit, error)

// merged runs fn on every folder's index at once, merges the hits in order and returns one page.
func (s *service) merged(ctx context.Context, env *command.Env, f Filter, limit, offset int, fn searchFn,
	less func(a, b rawHit) bool) (QueryResult, error) {
	if limit <= 0 {
		limit = 20
	}
	limit, offset = min(limit, 200), max(offset, 0)
	if offset+limit > maxPerFolder {
		return QueryResult{}, command.Errorf("bad_params", "cannot page past %d hits; narrow the search", maxPerFolder)
	}
	res := QueryResult{Hits: []Hit{}, Incomplete: []string{}}
	all, err := s.folders(ctx, env)
	if err != nil {
		return res, err
	}
	folders, err := pick(all, f.FolderIDs...)
	if err != nil {
		return res, err
	}
	items, err := filterItems(ctx, env.Store, f)
	if err != nil || (items != nil && len(items) == 0) {
		return res, err
	}
	hits, incomplete, err := s.fanOut(folders, items, offset+limit+1, fn)
	if err != nil {
		return res, err
	}
	res.Incomplete = incomplete
	sort.SliceStable(hits, func(i, j int) bool { return less(hits[i], hits[j]) })
	if res.More = len(hits) > offset+limit; res.More {
		hits = hits[:offset+limit]
	}
	if offset >= len(hits) {
		return res, nil
	}
	res.Hits, err = enrich(ctx, env.Store, hits[offset:])
	return res, err
}

// fanOut queries each folder's index concurrently.
func (s *service) fanOut(folders []library.Folder, items []string, n int, fn searchFn) ([]rawHit, []string, error) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	var hits []rawHit
	incomplete := []string{}
	var firstErr error
	for _, f := range folders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ix, err := s.index(f.ID, false)
			var found []rawHit
			if err == nil && ix != nil {
				found, err = fn(ix, items, n)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			if ix == nil || s.running(f.ID) {
				incomplete = append(incomplete, f.ID)
			}
			for _, h := range found {
				h.FolderID, h.Online = f.ID, f.Online
				hits = append(hits, h)
			}
		}()
	}
	wg.Wait()
	sort.Strings(incomplete)
	if firstErr != nil && (strings.Contains(firstErr.Error(), "fts5") || strings.Contains(firstErr.Error(), "no such column")) {
		return nil, nil, command.Errorf("bad_params", "bad search syntax: %v", firstErr)
	}
	return hits, incomplete, firstErr
}

func (s *service) running(folderID string) bool {
	s.mu.Lock()
	j := s.jobs[folderID]
	s.mu.Unlock()
	return j != nil && j.progress().Running
}

// Ranking cost grows with the number of matching passages, since FTS5 scores every match before
// it can sort. So each index is searched in slices of its passage IDs, and each slice stops after
// sliceRows matches. A search with fewer matches than that in every slice is ranked exactly; a
// broader one (a very common word) is ranked among the first sliceRows matches of every slice,
// which spreads the hits across the whole library and keeps the search under a second.
const slices = 8

// Variables so tests can change them.
var (
	sliceRows  = 2500
	itemRanges = 256 // the most items whose passages are searched range by range under a filter
)

// idRange is a span of passage IDs.
type idRange struct{ lo, hi int64 }

const itemFilter = ` AND p.file IN (SELECT num FROM indexed WHERE item_id IN (SELECT value FROM json_each(?)))`

// search ranks the passages matching an FTS5 query and returns the best n with snippets. Ranking
// happens first, on the full-text index alone, so snippets are built only for the passages kept.
func (ix *index) search(ctx context.Context, match string, items []string, n int) ([]rawHit, error) {
	ranges, filtered, err := ix.ranges(ctx, items)
	if err != nil || len(ranges) == 0 {
		return nil, err
	}
	q := `SELECT rowid, bm25(passages_fts, 2.0, 1.0) FROM passages_fts
		WHERE passages_fts MATCH ? AND rowid BETWEEN ? AND ? LIMIT ?`
	if filtered {
		q = `SELECT passages_fts.rowid, bm25(passages_fts, 2.0, 1.0) FROM passages_fts
			JOIN passages p ON p.id = passages_fts.rowid
			WHERE passages_fts MATCH ? AND passages_fts.rowid BETWEEN ? AND ?` + itemFilter + ` LIMIT ?`
	}
	per := max(n, slices*sliceRows/len(ranges))
	type ranked struct {
		id   int64
		rank float64
	}
	var all []ranked
	for _, r := range ranges {
		args := []any{match, r.lo, r.hi}
		if filtered {
			args = append(args, jsonList(items))
		}
		rows, err := ix.rd.QueryContext(ctx, q, append(args, per)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var h ranked
			if err := rows.Scan(&h.id, &h.rank); err != nil {
				rows.Close()
				return nil, err
			}
			all = append(all, h)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].rank != all[j].rank {
			return all[i].rank < all[j].rank
		}
		return all[i].id < all[j].id
	})
	all = all[:min(n, len(all))]
	snip, err := ix.rd.PrepareContext(ctx, `SELECT i.item_id, i.file_id, p.heading, p.locator,
		snippet(passages_fts, 1, '<mark>', '</mark>', '…', 24)
		FROM passages_fts JOIN passages p ON p.id = passages_fts.rowid JOIN indexed i ON i.num = p.file
		WHERE passages_fts MATCH ? AND passages_fts.rowid = ?`)
	if err != nil {
		return nil, err
	}
	defer snip.Close()
	out := make([]rawHit, 0, len(all))
	for _, a := range all {
		h := rawHit{}
		h.PassageID, h.Score = a.id, -a.rank
		err := snip.QueryRowContext(ctx, match, a.id).Scan(&h.ItemID, &h.FileID, &h.Heading, &h.locator, &h.Snippet)
		if errors.Is(err, sql.ErrNoRows) {
			continue // the book was re-indexed meanwhile
		}
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}

// ranges returns the passage ID ranges to search: slices of the whole index, or, under an item
// filter, the items' own ranges when there are few of them. filtered says the ranges still need
// the item filter.
func (ix *index) ranges(ctx context.Context, items []string) (rs []idRange, filtered bool, err error) {
	if items != nil {
		rows, err := ix.rd.QueryContext(ctx, `SELECT first_id, first_id + passages - 1 FROM indexed
			WHERE passages > 0 AND item_id IN (SELECT value FROM json_each(?)) ORDER BY first_id LIMIT ?`,
			jsonList(items), itemRanges+1)
		if err != nil {
			return nil, false, err
		}
		defer rows.Close()
		for rows.Next() {
			var r idRange
			if err := rows.Scan(&r.lo, &r.hi); err != nil {
				return nil, false, err
			}
			rs = append(rs, r)
		}
		if err := rows.Err(); err != nil || len(rs) <= itemRanges {
			return rs, false, err
		}
	}
	var top int64
	if err := ix.rd.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM passages`).Scan(&top); err != nil {
		return nil, false, err
	}
	if top == 0 {
		return nil, false, nil
	}
	rs = rs[:0]
	step := (top + slices - 1) / slices
	for lo := int64(1); lo <= top; lo += step {
		rs = append(rs, idRange{lo, min(lo+step-1, top)})
	}
	return rs, items != nil, nil
}

// refs finds passages citing a range that overlaps r. Citations never cross books, so the scan
// starts at r's book.
func (ix *index) refs(ctx context.Context, r verse.Range, items []string, n int) ([]rawHit, error) {
	top := `SELECT x.passage_id, MIN(x.verse_end - x.verse_start) AS width, x.verse_start, x.verse_end
		FROM refs x WHERE x.verse_start BETWEEN ? AND ? AND x.verse_end >= ?`
	args := []any{int64(r.Start.Book()) * 1_000_000, int64(r.End), int64(r.Start)}
	if items != nil {
		top += ` AND x.passage_id IN (SELECT id FROM passages p WHERE 1` + itemFilter + `)`
		args = append(args, jsonList(items))
	}
	top += ` GROUP BY x.passage_id ORDER BY width, x.passage_id LIMIT ?`
	rows, err := ix.rd.QueryContext(ctx, `SELECT p.id, i.item_id, i.file_id, p.heading, p.locator, p.text_z,
		t.width, t.verse_start, t.verse_end FROM (`+top+`) t JOIN passages p ON p.id = t.passage_id
		JOIN indexed i ON i.num = p.file ORDER BY t.width, p.id`, append(args, n)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rawHit
	for rows.Next() {
		var h rawHit
		var start, end int64
		var z []byte
		if err := rows.Scan(&h.PassageID, &h.ItemID, &h.FileID, &h.Heading, &h.locator, &z, &h.width,
			&start, &end); err != nil {
			return nil, err
		}
		if h.text, err = unpackText(z); err != nil {
			return nil, err
		}
		cited := verse.Range{Start: verse.ID(start), End: verse.ID(end)}
		h.Cites = cited.String()
		h.Snippet = citeSnippet(h.text, r)
		out = append(out, h)
	}
	return out, rows.Err()
}

// citeSnippet returns the text around the first citation overlapping r, with it marked.
func citeSnippet(text string, r verse.Range) string {
	for _, c := range findRefs(text) {
		if !c.r.Overlaps(r) {
			continue
		}
		from, to := max(c.start-120, 0), min(c.end+120, len(text))
		for from > 0 && !isRuneStart(text[from]) {
			from--
		}
		for to < len(text) && !isRuneStart(text[to]) {
			to++
		}
		s := text[from:c.start] + "<mark>" + text[c.start:c.end] + "</mark>" + text[c.end:to]
		if from > 0 {
			s = "…" + s
		}
		if to < len(text) {
			s += "…"
		}
		return s
	}
	return text
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

func jsonList(ids []string) string {
	b, _ := json.Marshal(ids)
	return string(b)
}

// filterItems returns the IDs of the items a filter allows, or nil when it allows every item.
func filterItems(ctx context.Context, st *store.Store, f Filter) ([]string, error) {
	if len(f.ItemIDs) == 0 && f.Kind == "" && f.Author == "" {
		return nil, nil
	}
	q := `SELECT i.id FROM items i WHERE i.deleted_at IS NULL`
	var args []any
	if len(f.ItemIDs) > 0 {
		q += ` AND i.id IN (SELECT value FROM json_each(?))`
		args = append(args, jsonList(f.ItemIDs))
	}
	if f.Kind != "" {
		q += ` AND i.kind = ?`
		args = append(args, f.Kind)
	}
	if f.Author != "" {
		q += ` AND EXISTS (SELECT 1 FROM item_people ip JOIN people pe ON pe.id = ip.person_id
			WHERE ip.item_id = i.id AND pe.name LIKE ? ESCAPE '\')`
		args = append(args, likeArg(f.Author))
	}
	out := []string{}
	rows, err := st.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func likeArg(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}

// enrich adds catalogue details from library.db. Hits on items that have left the library are
// dropped; the index forgets them on its next run.
func enrich(ctx context.Context, st *store.Store, raw []rawHit) ([]Hit, error) {
	itemIDs, fileIDs := []string{}, []string{}
	for _, h := range raw {
		itemIDs, fileIDs = append(itemIDs, h.ItemID), append(fileIDs, h.FileID)
	}
	items := map[string][2]string{}
	err := each(ctx, st, `SELECT id, title, kind FROM items WHERE deleted_at IS NULL
		AND id IN (SELECT value FROM json_each(?))`, itemIDs, func(r *sql.Rows) error {
		var id, title, kind string
		err := r.Scan(&id, &title, &kind)
		items[id] = [2]string{title, kind}
		return err
	})
	if err != nil {
		return nil, err
	}
	authors := map[string][]string{}
	err = each(ctx, st, `SELECT ip.item_id, pe.name FROM item_people ip JOIN people pe ON pe.id = ip.person_id
		WHERE ip.role = 'author' AND ip.item_id IN (SELECT value FROM json_each(?)) ORDER BY ip.position`,
		itemIDs, func(r *sql.Rows) error {
			var id, name string
			err := r.Scan(&id, &name)
			authors[id] = append(authors[id], name)
			return err
		})
	if err != nil {
		return nil, err
	}
	files := map[string][2]string{}
	err = each(ctx, st, `SELECT id, rel_path, format FROM files WHERE id IN (SELECT value FROM json_each(?))`,
		fileIDs, func(r *sql.Rows) error {
			var id, rel, format string
			err := r.Scan(&id, &rel, &format)
			files[id] = [2]string{rel, format}
			return err
		})
	if err != nil {
		return nil, err
	}
	out := []Hit{}
	for _, h := range raw {
		it, ok := items[h.ItemID]
		if !ok {
			continue
		}
		hit := h.Hit
		hit.Title, hit.Kind, hit.Authors = it[0], it[1], authors[h.ItemID]
		if hit.Authors == nil {
			hit.Authors = []string{}
		}
		hit.RelPath, hit.Format = files[h.FileID][0], files[h.FileID][1]
		json.Unmarshal([]byte(h.locator), &hit.Locator)
		out = append(out, hit)
	}
	return out, nil
}

func each(ctx context.Context, st *store.Store, q string, ids []string, fn func(*sql.Rows) error) error {
	rows, err := st.Query(ctx, q, jsonList(ids))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ftsQuery turns plain words into an FTS5 query, quoting each word so punctuation is safe. It
// matches bible.search's modes.
func ftsQuery(q, mode string) string {
	if mode == "fts" {
		return q
	}
	words := strings.FieldsFunc(q, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '"' || r == ',' || r == ';' || r == ':'
	})
	for i, w := range words {
		words[i] = `"` + w + `"`
	}
	switch mode {
	case "any":
		return strings.Join(words, " OR ")
	case "phrase":
		return `"` + strings.Join(strings.Fields(strings.ReplaceAll(q, `"`, "")), " ") + `"`
	}
	return strings.Join(words, " ")
}
