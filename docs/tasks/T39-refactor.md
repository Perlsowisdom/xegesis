# T39: Refactor pass

Brief for one agent (Sonnet), approved by the user on 11 November 2026. It comes from a quick code
check on 6 November; the dead code it found was already deleted in commit 8160b30. **Behaviour
must not change.** The real-app suite (`npm run e2e`) is the safety net. Run it before you start,
note any failures, and finish with it green.

Read `AGENTS.md` first. **Commit after each numbered item**, so a stop for usage leaves finished
work.

1. **Split `frontend/src/editor/index.ts`** (about 1,400 lines) into cohesive modules:
   - toolbar and the "More" overflow;
   - the `/` menu;
   - Insert (verse reference, verse text, highlight, note link);
   - headings, folding and the outline strip;
   - tables;
   - ink wiring.

   Keep `index.ts` as the small entry point, with the same exports. No logic changes.
2. **Split `frontend/src/document/book.ts`** (about 1,000 lines) the same way:
   - layout and paging;
   - scroll mode;
   - highlights and marks;
   - the section and page field;
   - Bible-type book navigation.
3. **One ink engine.** The stroke outline is currently computed twice:
   - in TypeScript (`editor/inkengine.ts`);
   - in Go (`internal/export/inkoutline.go`).

   Prefer one source of truth. For example, export could take the outline polygons the frontend
   already computes and saves, or the Go port could stay with the shared golden test made stricter
   and a note in both files. Choose whichever is simpler and safer, and explain the choice in the
   commit message. Exports must look the same.
4. **Remove the global hook `core.BookVerses`** (`internal/command/core/bookverses.go`). Pass the
   book-to-verses lookup in through `command.Env`, or an interface registered at start-up, instead
   of a package-level variable set by the reader.
5. **Count quotes by module ID, not by credit line.**
   - Quote blocks get the Bible's module ID (or book ID) in their saved Markdown, in a form that
     round-trips.
   - The per-document personal-copy limit (`document.quote`) and export counting (`copy_log`) use
     it.
   - Old quotes that have only a credit line still count, as they do now.
6. **Close databases on shutdown.** The app's `shutdown` (`app/main.go`) should close:
   - the Bible module library (`bible.Library.Close`);
   - the search service's open indexes (`closeAll`).

   Do this after background work stops. Add a Go test where practical.
7. **Make the milestone 2 real-app steps independent.** In `frontend/e2e/m2.e2e.mjs`, each step
   should set up what it needs (by command where the step isn't testing that setup), so one failure
   no longer fails every later step. Keep the restart step's meaning: it still checks that what was
   made earlier survives a restart.
8. **Tidy `docs/tasks/followups.md`.**
   - Move fixed items (struck through, or found fixed in the code) to a short "Fixed" list at the
     end.
   - Group the open items by area: editor, books, Bibles, highlights, assistant, ink, tests, other.
   - Drop duplicates.
   - Don't lose any open item.

Rules:
- Run `npm ci` in `frontend/`; no junctions.
- Use the harness's temporary library copy only.
- Run long commands with timeouts. Close every process you start, and never touch
  `%LOCALAPPDATA%\Temp\xegesis-check\Xegesis*.exe`.
- Don't commit `frontend/dist`.
- Delete your run logs.

Done when every item is committed and `go vet`, `go test ./...`, `npm test`, `tsc --noEmit`,
`npm run build` and the full `npm run e2e` pass.
