# T19: Milestone 2 integration check

Brief for one agent (Opus), after T13–T18 and the T34 redesign. Read these first:
- `AGENTS.md`;
- `frontend/e2e/`, the real-app harness (`npm run e2e`);
- the briefs `T13-reports.md`, `T14-notes-lists.md`, `T15-collections.md`, `T16-reading-plans.md`,
  `T18-print-export.md`, and `docs/documents.md` for T17, the sermon builder (it has no brief);
- `docs/tasks/followups.md`.

**The tracker row says:** "All M2 tests pass together." Milestone 2 is reports, notes, highlights and
saved lists, collections, reading plans, the sermon and lesson builder, and licence-aware printing and
exporting. Each was built and tested on its own, and most were reshaped by the T34 redesign. This
task proves that they work **together, in the real app**.

## 1. One study session, end to end

Write `frontend/e2e/m2.e2e.mjs`. It does what a user would do in one sitting, on the harness's copy of
the scratch library, typing and clicking with real events and saving a screenshot at each step:

1. Open a passage in Read mode, then in a Study pane.
2. Run a **passage report** and a **word report** on it (T13).
3. Highlight part of a verse (T34f) and write a **note** on the passage, inserting that highlight and a
   verse quote (T14, T34e).
4. Edit the note, then **restore an earlier version** of it (T14).
5. Add the passage to a **saved list** (T14).
6. Make a **collection** with a rule, and check that the books it should hold are in it (T15).
7. Start a **reading plan** and mark today's reading done; check that progress shows (T16).
8. Build a **sermon** in a folder: a title, an outline with headings, verses from the passage, the
   highlight inserted, and a table (T17, T34g).
9. **Print and export** the sermon to HTML, Word and PDF, and the note to Markdown. Check that the
   Bible's credit line appears, and that a quote over the licence limit is shortened or refused as
   the licence says (T18).
10. Restart the app on the same library copy, and check that everything above is still there.

## 2. Every suite together

`go vet ./...`, `go test ./...`, `npm test`, `npm run build` and `npm run e2e` must pass in one run,
on a clean checkout.

## 3. Check T17 against its row

The T17 row ("Outline with verses exported") is still "In progress". Check that step 8 and step 9
meet it. Report pass or fail.

## Rules

- Fix small bugs you find on the way, each with a test. Log bigger ones in `docs/tasks/followups.md`
  under "After T19" rather than redesigning anything. The user approved the current look.
- Never use the user's real library. Generate any book fixtures in the test.
- Run long commands with timeouts. Close every process you start.
- Don't junction or symlink `node_modules`; run `npm ci` in the worktree.

## Done when

- `m2.e2e.mjs` passes with a screenshot for each step.
- All suites pass together.
- The report lists each step as pass or fail with its screenshot, says whether T17 meets its row, and
  lists the bugs found, fixed or logged.
