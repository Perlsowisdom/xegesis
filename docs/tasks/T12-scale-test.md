# T12: M1 integration and the 10,000-book test

Brief for one agent (Opus), after T33. Read `AGENTS.md` first.

## Goal

Show that Xegesis copes with a real-sized library: 10,000 books and 8,000 notes. Use the import path
people actually use (T33). Fix what the numbers show.

## What to build

1. **A generator** (`internal/scaletest`, behind a build tag or a `-short` skip so CI stays fast). It writes
   a library of synthetic but realistic books:
   - mostly EPUB, with some PDF and DOCX;
   - a spread of sizes (a few pages to about 1,000), with chapters and headings;
   - Bible references in the text;
   - metadata including some public-domain rights statements;
   - spread across two library folders, one of them treated as an external drive that goes offline.

   It also creates 8,000 notes and some highlights.
2. **Measurements**, written to `docs/performance.md` with the machine they ran on. Compare each with the
   performance targets in the planning doc:
   - add folders and scan;
   - full index build;
   - rescan with nothing changed;
   - search across both folders (common word, phrase, rare word);
   - books citing a passage;
   - opening a book;
   - opening the Notes list;
   - export and backup of the full library;
   - disk size of `library.db` and the indexes;
   - peak memory (Go heap and process).
3. **Smaller search indexes:** they are about 2.4 times the book text today. Store passage text
   compressed, for example with FTS5 external or contentless content plus compressed text in the
   passages table, keeping snippets and highlighting working. Aim for no more than the size of the text
   itself. Re-measure.
4. **Memory:** make sure scanning and indexing 10,000 books stay within a stated budget (aim for under
   500 MB process memory on desktop). Set `GOMEMLIMIT` defaults in the app if that helps. Add goleak checks
   to the indexer and scanner tests if missing.
5. **Integration check of M1:**
   - Add books (T33).
   - Install a Bible from a file.
   - Read a book beside it.
   - Search the library and open a hit.
   - Highlight.
   - Export.
   - Back up.

   Run it as an automated test against a scratch library through the command registry.

## Done when

- `docs/performance.md` lists every measurement against its target, before and after the fixes.
- The index is no larger than the book text it holds, or the report explains why not.
- Peak memory stays within the stated budget.
- The integration test passes.
- `go vet` and `go test` pass, and CI stays under its current run time.
