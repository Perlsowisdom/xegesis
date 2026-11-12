# Follow-ups found while checking screens

Small issues to fold into the next task that touches each area, or a tidy-up pass. Open items are
grouped by area; what has been fixed is listed at the end. Tidied in T39 (11 November 2026): nothing
open was dropped, duplicates were merged, and the dates of the original notes are kept in brackets.

## Open

### Editor (notes and sermons)

- Sermons have no "date preached" or "theme" fields in the editor yet (the user named passage, date
  preached and theme as the three most important things). `document.update` takes `preached_on` and
  `theme` now; the page does not show them. [3 Nov]
- The note and sermon editors have been judged to "keep missing the mark" more than once. Before
  more editor work: study the user's examples hands-on, write behaviour-level acceptance tests, and
  test typing in the real app before calling it done. [4 Nov]
- The sermon toolbar was said to duplicate verse insertion (a Verse reference and a Quote verses button),
  and the sparkle icon reads as "AI" rather than "insert reference". Check that the Insert menu settled
  this. [4 Nov]
- Notes: after Restore in Recently deleted, the view stays on the empty deleted list; go back to the
  notes list instead. [29 Oct]
- Notes: four filter menus are always shown; fold them under More filters, as in the Library. [29 Oct]
- The "Notes on this passage" form sits below long commentary in the side column; the Save button
  can fall below the window (Ctrl+Enter still saves). Give notes their own pane. [29 Oct]
- "Insert into note" (assistant) inserts after the block with the caret of the note or sermon last
  typed in; it does not ask which when several are open side by side. [5 Nov]
- Dragging a Related card into a note was checked with the data a card carries, not by dragging a real
  card; the scratch library has no commentary. Recheck with a commentary installed. [4 Nov]

### Books and the reader

- EPUB sections loaded while scrolling are never unloaded. [4 Nov]
- Highlights tool for a book: notes are listed beside highlights but have no colour filter or search. [4 Nov]
- The cause of "EPUBs won't open" is not confirmed. A stuck selection toolbar covering the Library was
  fixed, which is the likely cause. Ask the user to retry. [4 Nov]
- Narrow panes (under 440px) are meant to wrap their tabs; not confirmed on screen. [3 Nov]
- The Reader shows only the verse in the passage box (Romans 8:28), not the chapter around it. Decide
  whether a passage should show the whole chapter scrolled to the verse. [3 Nov]
- Check in the real app: Study mode (the grid, drag to split, presets, resizing, Move to…, the
  right-click menu), the rail popovers, auto-hide and the right sidebar. They were only tested in a
  browser with stand-in data. [3 Nov]
- Study grid: at most 3 siblings per split and 9 panes; a drop that would break either is refused (use
  Move to… from the tab menu). "Equal sizes" evens every split. [after T38b]
- Tagged words cannot be reached with the keyboard. [29 Oct]
- A pane's ⋯ menu is one long list of items that feel out of place ("Unlink" duplicates the link
  button). [3 Nov]

### Bibles and Bible-type books

- Bible-type books (library books typed Bible), not done yet: PDF books (no map, so no verse links or
  quoting); DOCX, Markdown and plain-text Bibles are not mapped for verses (only EPUB and HTML);
  "Chapter Twelve"-style names and non-English book names (listed as unmapped by `book.bible_map`);
  a chapter that runs over more than one EPUB section has verses only in the section where it starts;
  a quotation across two chapters; reading such a book in side-by-side columns with the installed
  Bibles. [T38a, T38c]
- Verse numbers are read from `<sup>`, elements whose class or id says verse, a number alone in a
  span/b/strong/small, and digits leading a paragraph. Inline numbers mid-paragraph with no markup, and
  verses shown only as images, are not found. A chapter with no usable numbers opens at the chapter, and
  its notes and highlights link to the whole chapter. [T38c]
- A highlight or note is linked to verses when it is made. Changing a book's type to Bible later does not
  link the earlier ones, and changing it away leaves the links. [T38c]
- `highlight.list` and `note.list` `on: "verses"` mean made on Bible verses (no book anchor). A linked
  book highlight reports as "Title · Psalms 84:3". Quotations from a book give plain words (no bold
  verse numbers), a link to the passage and the book's credit. [T38c]
- Search: with every Bible chip checked, a listed but unsearchable module (after the e2e SWORD install,
  "sworde2e") makes `bible.search` fail with not_found and hides the book hits too. Seen only in the e2e
  library; worth a look. [T38c]
- Abbott-Smith's Strong's index should install its companion lexicon automatically. [29 Oct]
- A SWORD `.zip` installed with `bible.install` takes its licence from the module's `.conf`: public
  domain, Creative Commons and freely distributable stay as they say; anything unclear is a personal
  copy that keeps the credit line. (Worth a note in the user guide.) [T38a]
- Plans: the green "Started … finishing …" message keeps the original finish date after days are marked;
  refresh or clear it. [29 Oct]

### Highlights

- A whole-verse highlight is never split by new words over it: the newer words are drawn over it (the old
  one becomes words around them on screen, and stays whole in the library). Picking verse numbers deletes
  ranged highlights wholly inside those verses but leaves ones that run past them. [4 Nov]
- Side by side, a whole-verse highlight is drawn on each Bible's cell rather than the whole row, and an
  empty cell is not coloured. [4 Nov]
- Ranged highlights moved by `highlight.update` have no interface yet (the command takes a range; the
  toolbar only recolours, underlines and removes). [4 Nov]
- Offsets count code points of the text as drawn from the Bible's markup. If a Bible module is reinstalled
  with different text, ranges in it may drift; they are not re-found by their words yet. [4 Nov]
- Word look-up was judged "over-aggressive" (words marked as clickable throughout); words should look like
  plain text until one is asked for. Check it still holds. [3 Nov]

### Assistant

- Claude goes through Anthropic's OpenAI-compatible endpoint: no prompt caching, extended thinking or
  citations (ADR 0011). A native adapter would make long chats cheaper. [5 Nov]
- Chats are kept for the session only and one at a time in the panel; there is no list of earlier chats
  (only "Save as note"). [5 Nov]
- References become verse chips when a reply is finished (through `book.refs`); while it streams they show
  as plain text. [5 Nov]
- Bibles and reference works are shortened to their quoting limit for every provider; only personal
  copies of library books are withheld from online providers. A personal-copy Bible is therefore sent,
  shortened, to an online provider, as it may be quoted into a note. Confirm with the user that this is
  what "personal copies of books" meant. [5 Nov]
- The licence gate limits each tool result on its own; a model that reads the same limited passage many
  times in one chat can receive more than one quoting limit in total. [5 Nov]
- The command lines for the real `claude` and `codex` (flags such as `--include-partial-messages`,
  `--allowedTools`, `--disallowedTools`, and Codex's `-c mcp_servers.xegesis.*` and `--sandbox read-only`)
  were written from the tools' documented JSON output and never run against the real tools. Try each once
  by hand and adjust `CLIAgent.Args` and `streamParser` in `internal/assistant/cli.go` if a flag or event
  name differs. Check in particular that Codex runs MCP tool calls in `exec` mode without asking. [5 Nov]
- The chat is sent to the tool afresh on every question rather than resumed with the tool's own session;
  long chats cost more. [5 Nov]
- Tool lines from Claude Code and Codex use a label made from the call's arguments; the gate's own label
  and notice do not travel back through MCP. Putting them in the MCP result's structured content would let
  the chat show them. [5 Nov]
- No model chips for the two tools: they use the model set in the tool itself. A free-text model name could
  be passed with `--model` / `-m`. [5 Nov]
- Detection runs each tool with `--version` every time the Model page opens; cache it for a minute if a
  slow `claude` start-up shows. [5 Nov]
- The `mcp-assistant` mode starts the whole app executable without a window; check on macOS and Linux
  builds, where the Wails binary may need a display to start. [5 Nov]
- The Not found state is checked in e2e by hiding the fake `codex`; the install links open through
  `runtime.BrowserOpenURL`, which e2e does not click. [5 Nov]

### Ink

- Not done: lasso select and move. Pen tilt is read by the browser but not used. [T37]
- A finger on an ink block never scrolls the page (`touch-action: none`); on a phone, scroll by touching
  text above or below the block. A "scroll with finger" switch in the strip would fix it. [T37]
- Pen, colour and width are shared by every block on screen and are not remembered after a restart. [T37]
- "New handwritten note" is in My work's New menu only; the right-sidebar notes panel and the Sermons
  list's New menu do not offer it yet (a sermon takes ink through /handwriting). [T37]
- Ink blocks do not move by dragging; cut and paste of the surrounding text moves them like a divider. [T37]
- Word exports leave ink out (it stays in Markdown and HTML copies). [T37]
- Highlighter strokes in PDF are blended with white, not transparent, so they hide whatever is under them. [T37]
- The real pen feel, the delegated ink trail on a real digitiser, and the width curve (`pressureWidth`,
  `speedWidth`) are untested by machine; they need the user's pen. If the line feels too thick or thin at
  light pressure, change the curve in `inkengine.ts` and `inkoutline.go` and refresh
  `internal/export/testdata/ink-golden.json`. [T37]
- No smoothing of the newest point; a shaky hand shows. Add a small lag-free filter only if the pen feels
  jittery. The live stroke is redrawn whole on each frame (fine for ordinary strokes). The eraser removes
  whole strokes and uses the saved points, not the outline. [T37]
- Exported HTML grows with the outline (about 2 to 3 path points per page unit of a stroke's length);
  simplifying the polygon would shrink it. [T37]
- PDF ink (stage 2): one annotation per page (a highlight row of style "ink", strokes in `anchors.locator_json`),
  so a page of ink is saved whole on every stroke; a very full page (thousands of strokes) saves slowly
  and a limit of 5,000 strokes and 200,000 points applies. Per-stroke rows would need a migration. [T37]
- PDF ink: the pointer, live-stroke and eraser code in `document/pdfink.ts` is a second copy of the one in
  `editor/inknode.ts` (the pen, palm rejection, trail and shape are shared through `editor/inkpen.ts`).
  Merge the two into one surface class when either changes again. [T37]
- PDF ink: the Hide ink choice is kept per book on this device (localStorage), not in the library. Undo and
  redo cover what was written in this pane since the book opened; another pane's changes reload the page's
  ink and keep the history. Two panes writing on the same page at once: the last save wins. [T37]
- PDF ink is not shown over a printed or exported page of the book's own text; export adds each inked
  page's ink as a drawing under "Ink · page N" (HTML SVG, PDF outline, Markdown ink block; not Word).
  A page whose text is cut by a licence limit gets its ink with the last part. [T37]
- PDF ink: a finger or the mouse wheel cannot scroll while writing (the layer takes touch); press Done to
  scroll by touch. Ink on pages that are rotated by the PDF (/Rotate) is not tested. [T37]

### Tests

- Real-app tests stand in for the system print dialog and the save dialog (a test cannot answer them);
  printing and choosing where to save are still checked by hand. [5 Nov]
- Selecting a book's text by dragging in Read mode is unreliable as the first check of a full e2e run; the
  test retries. [4 Nov]
- e2e: "Highlights in verses: clicking a highlight selects all of it..." failed in two full runs (a highlight
  box was null after recolouring) but passes alone; likely a timing flake. Worth a wait-for-mark there. [after T38b]
- e2e: `bibles2.e2e.mjs` is named to run after `bibles.e2e`, whose `form.card .hint` selector finds the
  Search view's form once it has been opened. [T38c]
- e2e: the pen, touch and palm checks send synthetic pointer events (the harness has no pen); the mouse
  checks use real `page.mouse` input. [T37]
- e2e: "EPUB opens in Read mode" has a hardened wait after the wheel scroll that was never seen to fail. [T37]
- Real-app interface tests exist (`frontend/e2e/`, `npm run e2e`): they build the desktop app, start it on a
  copy of the scratch library with a WebView2 debugging port and type into it with real key events. New
  screens should add a `*.e2e.mjs` file there. Wails clears the `WEBVIEW2_*` variables, so the test build
  (only) uses a patched copy of its WebView2 loader through a temporary `go.work`; see `e2e/app.mjs`.
  Screenshots are git-ignored (`frontend/e2e/shots/`). [4 Nov]

### Other

- Confirm dialogs say "wails.localhost says"; replace them with the app's own dialogs. [29 Oct]
- Migrations are tracked by one version number, so an out-of-order merge could skip one; record each
  applied migration instead. [29 Oct]
- Some secondary forms (export options, plan weekdays, licence choices, Move to…) still use native selects
  or tick-box lists rather than the new controls. [3 Nov]
- Content must never be overlapped: "the sidebar cuts off part of the text UI" and the auto-hide "show
  rail" button covering page headers were reported; check both are gone everywhere. [3 Nov]
- The right sidebar's own × button should go (the top-bar toggle is enough). [3 Nov]
- Light, Dark and "Match the system" in Settings → Appearance: check the colour tokens cover every screen. [3 Nov]

## Fixed

- HTML and Word exports showed `---` dividers as text: fixed in T34e (rules in HTML, Word, PDF).
- Clicking on a verse quote's words put the caret where typing does nothing: the quote now takes the click
  and puts the caret on the line after it (followups-1).
- A personal-copy Bible's limit was checked per quotation only: `document.quote` counts a document's
  quotations from the Bible and export counts them in `copy_log` (followups-1). T39 counts them by module id
  (`([KJV](#bible/kjv))` in the credit line); older quotes with only the credit name still count.
- "Notes on this passage" showed a note from another passage: it lists the notes on the passage being read
  (followups-1).
- A new book highlight over an older one stacked the two colours: `highlight.create` for books splits the
  overlap (`internal/reader/bookrange.go`) (followups-1).
- The old local preference `xegesis.assistant.provider` and its helpers are gone (followups-1).
- e2e flakes in `structure.e2e.mjs` hover checks: the checks wait for a still page, and a pane no longer
  repaints on its own announcement (followups-1).
- Read mode book toolbar: "+" hidden outside Study; the section field is sized to its number (followups-2).
- e2e: M2 step 5 clicked an off-screen verse number and the scroll closed the toolbar; step 5 and the
  "EPUB opens in Read mode" check now wait for a settled page (followups-2).
- The five T34a/T34b problems (study grid not filling the window, installed Bibles missing in panes,
  unfinished controls, light mode, tall pane headers) were fixed on 3 November (branch m2/t34-fixes).
- Markdown shortcuts in the note editor, highlights on any part of a verse, inserting a verse or a highlight
  into a note, and heading sizes in the sermon editor all exist now (T34f, T19, the Insert and toolbar work;
  covered by the real-app tests).
- Ink exports draw the same smooth variable-width outline as the screen (the Go port of the stroke engine,
  T37 stage 1b); perfect-freehand is removed. T39 pins both engines to one golden file.
- Highlights and notes made in a Bible-type book are linked to the reference (T38c); T39 replaced the
  `core.BookVerses` hook with a `command.BookLinker` the reader registers.
- Databases are closed on shutdown (T39): the Bible module library and the search indexes.
- The M2 real-app steps no longer depend on each other (T39).
- e2e: build the keep-awake request (SetThreadExecutionState with ES_DISPLAY_REQUIRED) into run.mjs on Windows. On 12 November a laptop display going to sleep during a full run made screenshots hang and the app close, failing 116 of 130; the same run passed 130 of 130 with the display kept awake.
