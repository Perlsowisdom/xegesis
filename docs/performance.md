# Performance: the 10,000-book test (T12)

How Xegesis copes with a real-sized library: 10,000 books and 8,000 notes, added through the import
path people use (`library.add`), indexed in the background as the app does, and driven only through the
command registry.

## How to run it

```sh
go test -tags scale -run TestScale -timeout 8h -v ./internal/scaletest
```

| Variable | Default | Meaning |
| --- | --- | --- |
| `XEGESIS_SCALE_BOOKS` | 10000 | Books to generate |
| `XEGESIS_SCALE_NOTES` | 8000 | Notes to create |
| `XEGESIS_SCALE_DIR` | new folder in the temp folder | Scratch folder for books and data |
| `XEGESIS_SCALE_OUT` | none | JSON file for the results |
| `XEGESIS_SCALE_KEEP` | unset | `1` keeps the scratch data |
| `XEGESIS_SCALE_OPTIMIZE` | unset | `1` also times an FTS5 optimize and VACUUM |

The test needs the `scale` build tag, so `go test ./...` and CI skip it. It writes about 0.9 GB of books
and 2.7 GB of data for 10,000 books. Index files stay open until the process ends, so on Windows the
scratch folder may be left behind; delete `%TEMP%\xegesis-scale-*` afterwards. The milestone 1
integration test (`TestM1`, same package) and a small generator test run in the normal suite (about 7 s).

## The library

`internal/scaletest` generates synthetic books (seed 12): 8,480 EPUB, 1,048 PDF and 472 DOCX, 3 to
1,000 pages (log-normal, median about 60; PDF and DOCX at most 400), 962,829 pages in all, 1,675 MB of
text in 2.84 million passages and 865 MB of files. Books have chapters, section headings, Bible
references (more in commentaries and sermons) and metadata; one in five states a public-domain right.
70% are in one library folder, 30% in a second folder that plays an external drive and is unplugged
for the offline searches. The text is drawn from a Zipf-distributed vocabulary of common English and
theology words plus invented words, so it has the shape of prose without copying any book. It
compresses worse than real English (deflate 0.55 per passage, against roughly 0.40 to 0.45 for real
prose), so the index-size numbers are, if anything, pessimistic.

## Machine

Windows 11 Pro, Intel Core i7-1255U (10 cores, 12 threads, laptop), 16 GB RAM, Go 1.27.0,
`modernc.org/sqlite` 1.61.0. Each number is one run; searches are the median of five, with the worst
alongside. Times on this laptop vary by up to 1.7x between runs (see the two index builds below).

## Results

"Before" is the code at the start of T12 (index format 1); "after" is this branch (index format 2).
All numbers are measured; none are extrapolated. "After, 2,000" ran before the last two changes
(parallel slices, truncating the write-ahead log). "After, 10,000" gives the range of two runs: one
before the parallel slices and the final one; the search times are from the final run.

| Measurement | Target | Before, 2,000 | Before, 10,000 | After, 2,000 | After, 10,000 |
| --- | --- | --- | --- | --- | --- |
| Add both folders and scan (`library.add`) | responsive; no target | 7.3 s | 37.5 s | 6.7 s | 40 to 60 s |
| Full index build, in the background | may take hours | 186 s (1.8 MB/s) | 1,382 s (1.2 MB/s) | 124 s (2.7 MB/s) | 765 to 1,269 s (2.2 to 1.3 MB/s) |
| First book searchable after indexing starts | books searchable one by one | 0.8 s | 0.3 s | 2.1 s | 2.2 s |
| Search while indexing, median (worst) | app stays responsive | 0.14 s (1.6 s) | 0.35 s (2.9 s) | 0.06 s (0.15 s) | 0.14 to 0.16 s (0.4 to 2.3 s) |
| Rescan, nothing changed | responsive; no target | 0.49 s | 0.83 s | 0.11 s | 0.66 to 0.89 s |
| Search, very common word ("the") | under 1 s | 2.80 s | **15.8 s** (worst 31.8 s) | 0.17 s | **0.72 s** (worst 0.78 s) |
| Search, common word ("grace") | under 1 s | 0.076 s | 0.136 s | 0.026 s | 0.126 s |
| Search, two words ("faith hope") | under 1 s | 0.057 s | 0.133 s | 0.016 s | 0.043 s |
| Search, phrase ("living hope of") | under 1 s | 0.045 s | 0.058 s | 0.014 s | 0.037 s |
| Search, rare word (in 5 books) | under 1 s | 0.002 s | 0.002 s | 0.002 s | 0.008 s |
| Books citing a passage (Rom 8:28) | under 1 s | 0.014 s | 0.028 s | 0.007 s | 0.033 s |
| The same searches, external drive offline | under 1 s; offline books still found | all found, "the" 2.8 s | all found, "the" 13.1 s | all found, "the" 0.29 s | all found, "the" 0.77 s, others ≤ 0.12 s |
| Open the largest book (1,000 pages) and a chapter | under 1 s | 0.016 s | 0.009 s | 0.005 s | 0.010 s |
| Create 8,000 notes, one command each | no target | 8.0 s | 5.1 s | 5.7 s | 4.6 s |
| Open the notes list (100 newest of 8,000) | under 1 s | 0.084 s | 0.040 s | 0.045 s | 0.040 s |
| Export all user data (8,000 notes, 500 highlights) | no target | 1.2 s, 8.3 MB | 1.0 s, 8.3 MB | 0.6 s | 1.0 s, 8.3 MB |
| Back up the library (`backup.create`) | no target | 0.6 s, 25.5 MB | 0.6 s, 57 MB | 0.3 s | 0.65 s, 58 MB |
| `library.db` on disk | no target | 31 MB | 64 MB | 32 MB | 65 MB |
| Search indexes on disk | a few GB at most; no larger than the text | 873 MB, 2.60x text | **4,131 MB, 2.47x** | 568 MB, 1.69x | **2,645 MB, 1.58x** |
| Peak Go heap, whole run | — | 107 MB | 100 MB | 72 MB | 112 to 120 MB |
| Peak process memory (working set), whole run | under 500 MB | 122 MB | 183 MB | 156 MB | 147 to 153 MB |
| Peak process memory while indexing | under 500 MB | 68 MB | 135 MB | 155 MB | 123 to 146 MB |

The peak heap and process figures include generating the books, which the app never does; the app's
own work peaks at 146 MB (indexing) and 131 MB (searching).

## What changed

- **Search ranking cost** (the measured problem: a very common word took 16 to 32 s at 10,000 books).
  FTS5 scores every match with bm25 before it can sort. Each index is now searched in eight slices of
  passage IDs, each stopping after 2,500 matches, on four read connections at once. A search with fewer
  matches than that per slice is ranked exactly as before; a broader one is ranked among a sample spread
  over the whole library. Each FTS5 statement still counts the matches of every phrase for bm25 (about
  100 ms for "the" in the larger index), which is why the slices run in parallel.
- **Smaller index (format 2).** Passage text and locator are stored together, deflated with a preset
  dictionary, and FTS5 reads the text through a view (`passages_text`) that calls a Go SQL function, so
  snippets and highlighting work unchanged. Passages refer to their file by a small number and have
  consecutive IDs per file, so the per-passage file and item IDs and their two indexes are gone. The
  write-ahead log is truncated after each indexing run.
- **Old indexes rebuild themselves.** `PRAGMA user_version` holds the format; a file of another format
  is deleted and indexed again (before, it was an error). Indexing in the desktop app now starts by
  itself (below), so users' existing indexes are rebuilt without their doing anything.
- **Searches during indexing** use their own read connections (WAL), so they no longer wait for a
  1,000-page book to be written.
- **Desktop app** (`app/`): a 300 MiB soft Go heap limit unless `GOMEMLIMIT` is set (ADR 0008). Nothing
  in the app started indexing before, so books were never searchable from the interface; it now starts
  `search.index` after each rescan and after commands that add books, and cancels it at shutdown.
- **Leak checks:** `goleak` (MIT) checks every test in `internal/search` and `internal/library`.

## Index size: why it is not below the size of the text

At 10,000 books the index tables hold 1,327 MB of FTS5 inverted index, 1,202 MB of compressed
passages (text, locators and headings), 77 MB of Bible references and 31 MB of FTS5 document sizes,
2,645 MB in all for 1,675 MB of text (1.58x, down from 2.47x).

The FTS5 inverted index alone is 0.79x the text. It keeps word positions (`detail=full`), which phrase
search and `NEAR` need; `detail=column` would drop phrase search. An FTS5 `optimize` and `VACUUM` saved
1% (measured at 2,000 books, 22 s) and is not used. The text itself must stay in the index so that hits
on an unplugged drive still show snippets (ADR 0006). So the floor is about 0.8x plus the stored text.
Deflating each passage on its own leaves 0.55x of this synthetic text (0.58x without the dictionary);
compressing 16 KB blocks of passages would leave about 0.47x and bring the total to roughly 1.5x. For
real English text, which compresses better, we expect 1.3x to 1.4x. Getting below 1.0x would need a
contentless index with snippets read from the book files, which offline drives rule out.

## Collections (T15)

Counts for the built-in groupings and for rule collections, measured on a generated catalogue of
10,000 books (`TestCollectionsScale` in `internal/scaletest`, tag `scale`; same machine as above).
It makes catalogue rows only (3,333 authors, a type, a tradition, a year and a licence per book), not
book files, since counts never read files. Each number is the median of five runs, with the worst in
brackets. Times on this laptop vary by up to 2x between runs.

```sh
go test -tags scale -run TestCollectionsScale -v ./internal/scaletest
```

| Measurement | Target | 10,000 books |
| --- | --- | --- |
| `collection.list`, built-in groupings only | under 200 ms | 37 to 90 ms (worst 190 ms) |
| `collection.list`, groupings and 11 collections (1 hand-picked, 10 by rule) | under 200 ms | 150 to 180 ms (worst 280 ms) |
| `collection.list` also listing all 3,333 authors (on demand, when Author is opened) | none | 95 to 295 ms |
| `collection.preview`, one rule, with 5 examples | under 200 ms | 8 to 36 ms; 65 to 75 ms for an author text that matches many names |
| `collection.items`, rule collection, first 50 books | none | 55 ms |
| `library.items` in a built-in grouping, first 50 books | none | 30 ms |

What made it fast (from 1,050 ms for the first version): migration 5 adds covering indexes on
`item_facets(facet, value, item_id)`, `item_people(person_id, item_id, role)` and
`licences(type, item_id)`, so counts are read from an index; type, tradition, author and licence
conditions are `IN (subquery)` instead of one lookup per book; and the join to licences was dropped.
Each rule collection costs one count query (about 10 to 20 ms), so the tree grows with the number of
rules; ten is typical for a heavy user.

## Known gaps

- Ranking for very broad searches is approximate (a sample of 20,000 matches per index); exact for
  everything narrower.
- A very common word still costs about 0.7 to 0.8 s at 10,000 books on this laptop, close to the
  target; a library several times larger, or a slower machine, would pass a second.
- Index build time and search-while-indexing worst case varied a lot between runs on this laptop
  (765 s and 1,269 s; worst 0.4 s and 2.3 s).
- The notes list was measured as `note.list` with its default 100 notes; the interface may ask for more.
- Phone memory was not measured; the desktop peak suggests the budget is comfortable.
