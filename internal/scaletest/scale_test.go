//go:build scale

package scaletest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/backup"
	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/reader"
	"github.com/Perlsowisdom/xegesis/internal/search"
)

// The 10,000-book test. Run it with
//
//	go test -tags scale -run TestScale -timeout 8h -v ./internal/scaletest
//
// XEGESIS_SCALE_BOOKS (default 10000) and XEGESIS_SCALE_NOTES (default 8000) set the size,
// XEGESIS_SCALE_DIR the scratch folder (default: a new folder in the temp folder),
// XEGESIS_SCALE_OUT a JSON file for the results, and XEGESIS_SCALE_KEEP=1 keeps the scratch data.

func envInt(name string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return def
}

// meter samples Go heap and process memory while a phase runs.
type meter struct {
	mu               sync.Mutex
	heap, proc       uint64 // peaks in the current phase
	allHeap, allProc uint64
	stop             chan struct{}
	done             chan struct{}
}

func newMeter() *meter {
	m := &meter{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(m.done)
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		for {
			m.sample()
			select {
			case <-m.stop:
				return
			case <-t.C:
			}
		}
	}()
	return m
}

func (m *meter) sample() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	cur, _ := processMemory()
	m.mu.Lock()
	m.heap, m.proc = max(m.heap, ms.HeapInuse), max(m.proc, cur)
	m.allHeap, m.allProc = max(m.allHeap, ms.HeapInuse), max(m.allProc, cur)
	m.mu.Unlock()
}

// phase returns the peaks since the last call and starts a new phase.
func (m *meter) phase() (heap, proc uint64) {
	m.sample()
	m.mu.Lock()
	defer m.mu.Unlock()
	heap, proc = m.heap, m.proc
	m.heap, m.proc = 0, 0
	return heap, proc
}

func (m *meter) close() { close(m.stop); <-m.done }

// Row is one measurement.
type Row struct {
	Name    string  `json:"name"`
	Seconds float64 `json:"seconds,omitempty"`
	Value   string  `json:"value,omitempty"`
	HeapMB  float64 `json:"heap_mb,omitempty"`
	ProcMB  float64 `json:"process_mb,omitempty"`
}

type report struct {
	Machine string `json:"machine"`
	Books   int    `json:"books"`
	Notes   int    `json:"notes"`
	Rows    []Row  `json:"rows"`
}

func mb(b uint64) float64 { return float64(b) / (1 << 20) }

func dirSize(dir string) int64 {
	var n int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

func fileSizes(paths ...string) int64 {
	var n int64
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			n += fi.Size()
		}
	}
	return n
}

func median(ds []time.Duration) (time.Duration, time.Duration) {
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	return ds[len(ds)/2], ds[len(ds)-1]
}

func TestScale(t *testing.T) {
	books, notes := envInt("XEGESIS_SCALE_BOOKS", 10000), envInt("XEGESIS_SCALE_NOTES", 8000)
	root := os.Getenv("XEGESIS_SCALE_DIR")
	if root == "" {
		var err error
		if root, err = os.MkdirTemp("", "xegesis-scale-"); err != nil {
			t.Fatal(err)
		}
	}
	if os.Getenv("XEGESIS_SCALE_KEEP") == "" {
		// Index files stay open until the process ends, so removal is best effort.
		t.Cleanup(func() { os.RemoveAll(root) })
	}
	rep := &report{Machine: fmt.Sprintf("%s/%s, %d CPUs, %s", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(),
		runtime.Version()), Books: books, Notes: notes}
	m := newMeter()
	defer m.close()
	add := func(name string, d time.Duration, value string) {
		h, p := m.phase()
		r := Row{Name: name, Seconds: d.Seconds(), Value: value, HeapMB: mb(h), ProcMB: mb(p)}
		rep.Rows = append(rep.Rows, r)
		t.Logf("%-34s %9.3fs  %-40s heap %6.0f MB  process %6.0f MB", name, r.Seconds, value, r.HeapMB, r.ProcMB)
	}

	start := time.Now()
	gen, err := Generate(Config{Dir: filepath.Join(root, "books"), Books: books, Seed: 12})
	if err != nil {
		t.Fatal(err)
	}
	add("generate (not app work)", time.Since(start), fmt.Sprintf("%d books %v, %d pages, text %.0f MB, files %.0f MB",
		gen.Books, gen.Formats, gen.Pages, mb(uint64(gen.TextBytes)), mb(uint64(gen.FileBytes))))
	runtime.GC()
	m.phase()

	data := filepath.Join(root, "data")
	os.MkdirAll(data, 0o755)
	l := openLib(t, data)

	var added library.AddResult
	d := l.timed("library.add", map[string]any{"paths": []string{gen.Internal, gen.External}}, &added)
	add("add folders and scan", d, fmt.Sprintf("%d books added, %d errors", added.Added, len(added.Errors)))
	if added.Added != books {
		t.Fatalf("added %d of %d: %+v", added.Added, books, added.Errors[:min(3, len(added.Errors))])
	}

	// Index in the background, as the app does, timing searches while it runs.
	start = time.Now()
	l.must("search.index", map[string]any{}, nil)
	var first time.Duration
	var during []time.Duration
	for {
		var st []search.FolderStatus
		l.must("search.status", nil, &st)
		done, indexed := true, 0
		for _, s := range st {
			indexed += s.Indexed
			done = done && !s.Running
		}
		if first == 0 && indexed > 0 {
			first = time.Since(start)
		}
		if done {
			break
		}
		q := time.Now()
		l.must("search.query", map[string]any{"query": "grace"}, nil)
		during = append(during, time.Since(q))
		time.Sleep(2 * time.Second)
	}
	indexTime := time.Since(start)
	var st []search.FolderStatus
	l.must("search.status", nil, &st)
	passages, failed := 0, 0
	for _, s := range st {
		passages += s.Passages
		failed += s.Failed
		if s.State != "complete" {
			t.Fatalf("index state %+v", s)
		}
	}
	add("full index build", indexTime, fmt.Sprintf("%d passages, %d failed, %.1f MB text/s", passages, failed,
		mb(uint64(gen.TextBytes))/indexTime.Seconds()))
	add("first book searchable", first, "")
	if len(during) > 0 {
		med, worst := median(during)
		add("search while indexing (median)", med, fmt.Sprintf("worst %.3fs over %d", worst.Seconds(), len(during)))
	}

	var rescan library.RescanResult
	d = l.timed("library.rescan", nil, &rescan)
	add("rescan, nothing changed", d, fmt.Sprintf("%+v", rescan))

	type q struct{ name, cmd, query, mode string }
	queries := []q{
		{"search: very common word (the)", "search.query", "the", ""},
		{"search: common word (grace)", "search.query", "grace", ""},
		{"search: two words (faith hope)", "search.query", "faith hope", ""},
		{"search: phrase", "search.query", Phrase, "phrase"},
		{"search: rare word", "search.query", RareWord, ""},
		{"books citing Rom 8:28", "search.refs", "Rom 8:28", ""},
	}
	runSearches := func(suffix string) {
		for _, c := range queries {
			var ds []time.Duration
			var res search.QueryResult
			for range 5 {
				p := map[string]any{"query": c.query, "mode": c.mode}
				if c.cmd == "search.refs" {
					p = map[string]any{"ref": c.query}
				}
				ds = append(ds, l.timed(c.cmd, p, &res))
			}
			med, worst := median(ds)
			offline := 0
			for _, h := range res.Hits {
				if !h.Online {
					offline++
				}
			}
			add(c.name+suffix, med, fmt.Sprintf("%d hits (%d offline), worst %.3fs", len(res.Hits), offline, worst.Seconds()))
			if c.name == "search: rare word" && len(res.Hits) != gen.RareBooks {
				t.Errorf("rare word: %d hits, want %d", len(res.Hits), gen.RareBooks)
			}
		}
	}
	runSearches("")
	timeReports(t, l, root, add)

	var items library.ItemsResult
	l.must("library.items", map[string]any{"text": strings.TrimSuffix(filepath.Base(gen.LargestBook), ".epub")[6:]}, &items)
	if len(items.Items) > 0 {
		id := items.Items[0].ID
		var b reader.Book
		d = l.timed("book.open", map[string]any{"item_id": id}, &b)
		var sec reader.Section
		d += l.timed("book.section", map[string]any{"item_id": id, "section": max(1, b.Sections/2)}, &sec)
		add("open the largest book", d, fmt.Sprintf("%d pages, %d chapters, %d paragraphs shown", gen.LargestPages,
			b.Sections, sec.Paras))
	}

	start = time.Now()
	for i := range notes {
		p := map[string]any{"title": fmt.Sprintf("Note %d", i), "body": paragraph(randFor(i), 0.5)}
		if i%3 == 0 {
			p["ref"] = fmt.Sprintf("John %d:%d", 1+i%21, 1+i%20)
		}
		l.must("note.create", p, nil)
	}
	add(fmt.Sprintf("create %d notes", notes), time.Since(start), "")
	start = time.Now()
	highlights := 0
	for i, it := range firstItems(l, 500) {
		if l.run("highlight.create", map[string]any{"item_id": it, "locator": map[string]any{
			"section": 1, "para": 2, "offset": 0, "end_para": 2, "end_offset": 20, "text": "synthetic"},
			"style": []string{"yellow", "green", "blue"}[i%3]}, nil) == nil {
			highlights++
		}
	}
	add("create highlights", time.Since(start), fmt.Sprintf("%d highlights", highlights))
	var nl []json.RawMessage
	d = l.timed("note.list", nil, &nl)
	add("open the notes list", d, fmt.Sprintf("%d notes shown", len(nl)))
	d = l.timed("note.list", map[string]any{"ref": "John 3"}, &nl)
	add("notes on John 3", d, fmt.Sprintf("%d notes", len(nl)))

	var ex backup.ExportResult
	d = l.timed("export.data", map[string]any{"path": filepath.Join(root, "export.zip")}, &ex)
	add("export all user data", d, fmt.Sprintf("%v, %.1f MB", ex.Counts, mb(uint64(fileSizes(ex.Path)))))
	var bk backup.BackupResult
	d = l.timed("backup.create", map[string]any{"path": filepath.Join(root, "backup.db")}, &bk)
	add("back up the library", d, fmt.Sprintf("%.1f MB", mb(uint64(bk.Bytes))))

	// The external drive goes offline: its books must stay searchable.
	offline := filepath.Join(root, "books", "External drive offline")
	if err := os.Rename(filepath.Dir(gen.External), offline); err != nil {
		t.Fatal(err)
	}
	runSearches(" (drive offline)")
	if err := os.Rename(offline, filepath.Dir(gen.External)); err != nil {
		t.Fatal(err)
	}

	lib := fileSizes(filepath.Join(data, "library.db"), filepath.Join(data, "library.db-wal"))
	ix := dirSize(filepath.Join(data, "index"))
	add("size of library.db", 0, fmt.Sprintf("%.1f MB", mb(uint64(lib))))
	add("size of the search indexes", 0, fmt.Sprintf("%.0f MB, %.2f x the %.0f MB of text", mb(uint64(ix)),
		float64(ix)/float64(gen.TextBytes), mb(uint64(gen.TextBytes))))
	add("index size by table", 0, indexBreakdown(t, filepath.Join(data, "index")))
	if os.Getenv("XEGESIS_SCALE_OPTIMIZE") != "" {
		start = time.Now()
		files, _ := filepath.Glob(filepath.Join(data, "index", "*.db"))
		for _, f := range files {
			db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(f))
			if err != nil {
				t.Fatal(err)
			}
			for _, q := range []string{`INSERT INTO passages_fts(passages_fts) VALUES ('optimize')`, `VACUUM`} {
				if _, err := db.Exec(q); err != nil {
					t.Error(q, err)
				}
			}
			db.Close()
		}
		ix := dirSize(filepath.Join(data, "index"))
		add("optimize and vacuum the indexes", time.Since(start), fmt.Sprintf("%.0f MB, %.2f x text", mb(uint64(ix)),
			float64(ix)/float64(gen.TextBytes)))
		add("index size by table, optimized", 0, indexBreakdown(t, filepath.Join(data, "index")))
	}
	m.phase()
	_, peak := processMemory()
	rep.Rows = append(rep.Rows, Row{Name: "peak memory, whole run", HeapMB: mb(m.allHeap), ProcMB: mb(max(m.allProc, peak))})
	t.Logf("peak: heap %.0f MB, process %.0f MB", mb(m.allHeap), mb(max(m.allProc, peak)))

	if out := os.Getenv("XEGESIS_SCALE_OUT"); out != "" {
		b, _ := json.MarshalIndent(rep, "", "  ")
		if err := os.WriteFile(out, b, 0o644); err != nil {
			t.Error(err)
		}
	}
}

// usfmTagged is Romans 8:28 (World English Bible, public domain) with a Strong's number,
// so that word reports have occurrences to count.
const usfmTagged = `\id ROM Tagged test
\c 8
\p
\v 28 We know that all things work together for good for those who \w love|strong="G25"\w* God.
`

// timeReports measures each report with a Bible installed, the median of five, and checks that the
// library sections are there.
func timeReports(t *testing.T, l *lib, root string, add func(string, time.Duration, string)) {
	path := filepath.Join(root, "ROM-tagged.usfm")
	if err := os.WriteFile(path, []byte(usfmTagged), 0o644); err != nil {
		t.Fatal(err)
	}
	l.must("bible.install", map[string]any{"path": path, "id": "tagged", "name": "Tagged test", "licence_type": "public_domain"}, nil)
	type section struct {
		ID      string            `json:"id"`
		Entries []json.RawMessage `json:"entries"`
	}
	for _, c := range []struct{ name, cmd, key, value, want string }{
		{"passage report, Rom 8:28", "report.passage", "ref", "Rom 8:28", "citing"},
		{"word report, G25", "report.word", "word", "G25", "occurrences"},
		{"topic report, grace", "report.topic", "topic", "grace", "books"},
	} {
		var ds []time.Duration
		var rep struct {
			Sections []section `json:"sections"`
		}
		for range 5 {
			ds = append(ds, l.timed(c.cmd, map[string]any{c.key: c.value}, &rep))
		}
		med, worst := median(ds)
		ids := []string{}
		found := false
		for _, s := range rep.Sections {
			ids = append(ids, fmt.Sprintf("%s:%d", s.ID, len(s.Entries)))
			found = found || s.ID == c.want && len(s.Entries) > 0
		}
		if !found {
			t.Errorf("%s: no %s section in %v", c.name, c.want, ids)
		}
		add(c.name, med, fmt.Sprintf("worst %.3fs; %s", worst.Seconds(), strings.Join(ids, " ")))
	}
}

func firstItems(l *lib, n int) []string {
	var res library.ItemsResult
	l.must("library.items", map[string]any{"limit": n}, &res)
	var out []string
	for _, it := range res.Items {
		out = append(out, it.ID)
	}
	return out
}

func randFor(i int) *rand.Rand { return rand.New(rand.NewPCG(99, uint64(i))) }

// indexBreakdown sums the index files' space by table, largest first.
func indexBreakdown(t *testing.T, dir string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "*.db"))
	sizes := map[string]int64{}
	for _, f := range files {
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(f)+"?mode=ro")
		if err != nil {
			t.Log(err)
			continue
		}
		rows, err := db.Query(`SELECT name, SUM(pgsize) FROM dbstat GROUP BY name`)
		if err != nil {
			t.Log(err)
			db.Close()
			continue
		}
		for rows.Next() {
			var name string
			var n int64
			rows.Scan(&name, &n)
			sizes[name] += n
		}
		rows.Close()
		db.Close()
	}
	names := make([]string, 0, len(sizes))
	for n := range sizes {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return sizes[names[i]] > sizes[names[j]] })
	var parts []string
	for _, n := range names {
		if sizes[n] >= 1<<20 {
			parts = append(parts, fmt.Sprintf("%s %.0f MB", n, mb(uint64(sizes[n]))))
		}
	}
	return strings.Join(parts, ", ")
}
