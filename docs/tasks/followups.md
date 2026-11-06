# Follow-ups found while checking screens

Small issues to fold into the next task that touches each area, or a tidy-up pass.

## 29 October 2026 (T14, T16 on screen)

- Reader: the "Notes on this passage" form sits below long commentary in the side column; the Save
  button can fall below the window (Ctrl+Enter still saves). T34's shared layout should give notes
  their own pane.
- Notes: after Restore in Recently deleted, the view stays on the empty deleted list; go back to the
  notes list instead.
- Notes: four filter menus are always shown; fold them under More filters, as in the Library.
- Plans: the green "Started … finishing …" message keeps the original finish date after days are
  marked; refresh or clear it.
- Confirm dialogs say "wails.localhost says"; replace them with the app's own dialogs.
- Abbott-Smith's Strong's index should install its companion lexicon automatically.
- Tagged words cannot be reached with the keyboard.
- Migrations are tracked by one version number, so an out-of-order merge could skip one; record each
  applied migration instead.

## From the T34a/T34b redesign (3 November 2026)

- Check Study mode in the real app: the grid, drag to split, presets, resizing, Move to…, and the
  right-click menu. It was only tested in a browser with stub data, and the screenshot option can't
  reach it.
- Check the rail popovers, auto-hide and the right sidebar in the real app.
- Sermons have no "date preached" or "theme" fields yet. They need a Go schema change (T17 or T34c).
  The user named passage, date preached and theme as the three most important things.
- The Reader shows only the verse in the passage box (Romans 8:28), not the chapter around it.
  Decide whether a passage should show the whole chapter scrolled to the verse.
- Narrow panes (under 440px) are meant to wrap their tabs; this hasn't been confirmed on screen.
- Next stages: T34c (Library and My work: Filter popover, folders holding notes and sermons,
  collapsible sections, bin) and T36 (the AI assistant).

## User testing of T34a/T34b (3 November 2026): fixed 3 November (branch m2/t34-fixes)

All five fixed. Left: some secondary forms (export options, plan weekdays, licence choices, Move to…)
still use native selects or tick-box lists rather than the new controls.

1. **The study grid doesn't fill the window.** Three panes are squeezed into about 300px on the left,
   with the header controls wrapping, text one word per line, and horizontal scrollbars in each pane.
   The grid container (or its columns) isn't growing to the main area's width and height.
2. **Installed Bibles don't open in the Reader or Study panes.** Panes say "No Bibles are installed
   yet. Get one in Library → Get Bibles", but the installed Bible does show in the sidebar's Compare
   tool. The pane Bible list must use the same source Compare uses.
3. **Controls look unfinished.** The user wants the toggles, switches, segmented controls and chips
   from the approved mock-up instead of plain checkboxes. Their words: it makes the app "feel like a
   linux opensource weekend project". Apply them across Settings, Rail settings, the sidebar tool
   list and the filters.
4. **Light mode.** Only the dark theme shows. Add Light and Dark plus "Match the system" in Settings →
   Appearance, using the colour tokens.
5. The pane header is too tall in narrow panes: the tabs, place box, Bibles picker, link and ⋯ are
   stacked on four rows. Keep it to one row, with overflow in ⋯.

The user liked the Library ("looks good so far").

## User testing after T34c (3 November 2026)

Design first: mock-up for approval, then build.

- **Right sidebar:** remove its × button. The top-bar sidebar toggle is enough.
- **Pane ⋯ menu:** its items feel out of place, because everything is in one long list. "Unlink"
  duplicates the link button.
- **Sermon editor and note editor:** both look "terrible": a form of labelled boxes and a toolbar of
  big text buttons. They need a document-style design.
- **Highlights:** unintuitive, with no way to see or delete them. Needed: a clear selection toolbar,
  clicking a highlight to change or remove it, and a list of highlights.
- **Word look-up:** "over-aggressive", because words are marked as clickable throughout the text.
  Words should look like plain text until you ask to look one up.
- **Bug: the auto-hide "show rail" button sits on top of page headers.** It covers "My work". Content
  must never be overlapped: the user reported "the sidebar cuts off part of the text UI".

## After T34d (4 November 2026)

- ~~HTML and Word exports still show `---` dividers as text instead of a rule.~~ Fixed in T34e (rules in HTML, Word, PDF).
- T34d was checked in a browser preview with stand-in data. The real app was only seen on its start screen.

## User testing of T34d (4 November 2026): editors still miss the mark

- Markdown shortcuts do not work in the note editor (typing ## , **, - , > should format, as in Any or Obsidian).
- Highlights cover whole verses only. They must cover any part of a verse, down to words and characters.
- There is no way to insert a verse or a highlight into a note.
- The sermon editor has no way to choose the heading size (H1, H2, H3).
- The sermon toolbar duplicates verse insertion (the Verse reference button and the Quote verses button), and the sparkle icon reads as "AI" rather than "insert reference".
- The user feels the note and sermon editors keep missing the mark despite examples and detailed instructions.
  Next: study Any's editor and the user's examples hands-on first, write behaviour-level acceptance tests, and test typing in the real app before calling it done.

## From T34e (4 November 2026)

- **Real-app interface tests exist** (`frontend/e2e/`, `npm run e2e`): they build the desktop app,
  start it on a copy of the scratch library with a WebView2 debugging port, and type into it with
  real key events through `playwright-core`. This is the start of T30 (interface tests): new screens
  should add a `*.e2e.mjs` file there. Wails clears the `WEBVIEW2_*` variables, so the test build
  (only) uses a patched copy of its WebView2 loader through a temporary `go.work`; see `e2e/app.mjs`.
- The scratch library has no commentary, so dragging a Related card was checked with the data a card
  carries, not by dragging a real card. Recheck with a commentary installed.
- Highlights are quoted as whole verses until T34f; `highlightText` (src/insertwords.ts) already takes
  a character range for it.

## After T34g (4 November 2026)

- The cause of "EPUBs won't open" is not confirmed. A stuck selection toolbar covering the Library was fixed, which is the likely cause. Ask the user to retry.
- Selecting a book's text by dragging in Read mode is unreliable as the first check of a full e2e run; the test retries.

## After scroll mode (4 November 2026)

- EPUB chapters loaded while scrolling are never unloaded.
- Highlights in the sidebar for a book: notes are listed beside highlights but have no colour filter or search.

## After T19 (5 November 2026)

- ~~Clicking on the words of a verse quote in a note or sermon puts the caret in them, where typing does nothing and says nothing. Ctrl+End now leaves the quote; a click should put the caret on the line after it instead.~~ Fixed in followups-1: the quote now takes the click and puts the caret on the line after it.
- ~~A personal-copy Bible's limit is checked per quotation only (about 2,000 characters each), so several quotations in one sermon can add up past it, and exporting the sermon does not count them in `copy_log` (documents.md calls the per-quote limit an estimate).~~ Fixed in followups-1: `document.quote` counts a document's quotations from the Bible (the editor sends its text) and export counts them in `copy_log`, cutting by the day's limit.
- ~~⋯ → "Notes on this passage" opens the Notes tool as it was left: if a note on another passage is still open there (Romans 8 while reading Romans 12), that note shows instead of the notes on this passage.~~ Fixed in followups-1: it lists the notes on the passage being read.
- The interface tests stand in for the system print dialog and the save dialog (a test cannot answer them); printing and choosing where to save are still checked by hand.

## After T34f (4 November 2026)

- ~~Books (EPUB) already keep exact ranges (paragraph and offset); a new book highlight over an older one still stacks the two colours. Give `highlight.create` for books the same overlap splitting as Bible ranges.~~ Fixed in followups-1: `highlight.create` for books takes the overlapped part out of older highlights (`internal/reader/bookrange.go`); pieces keep exact words only inside one paragraph.
- A whole-verse highlight is never split by new words over it: the newer words are drawn over it (the old one becomes words around them on screen, and stays whole in the library). Picking verse numbers deletes ranged highlights wholly inside those verses but leaves ones that run past them.
- Side by side, a whole-verse highlight is drawn on each Bible's cell rather than the whole row, and an empty cell is not coloured.
- Ranged highlights moved by `highlight.update` have no interface yet (the command takes a range; the toolbar only recolours, underlines and removes).
- Offsets count code points of the text as drawn from the Bible's markup. If a Bible module is reinstalled with different text, ranges in it may drift; they are not re-found by their words yet.

## After T36a (5 November 2026)

- Claude Code and Codex show as "coming in T36b" in Settings → Assistant → Model; the MCP assistant mode (read-only tools only) comes with them and should reuse `assistant.Tools`.
- Claude goes through Anthropic's OpenAI-compatible endpoint: no prompt caching, extended thinking or citations (ADR 0011). A native adapter would make long chats cheaper.
- Chats are kept for the session only and one at a time in the panel; there is no list of earlier chats to go back to (only "Save as note").
- References become verse chips when a reply is finished (through `book.refs`); while it streams they show as plain text.
- "Insert into note" inserts after the block with the caret of the note or sermon last typed in; it does not ask which when several are open side by side.
- Bibles and reference works are shortened to their quoting limit for every provider; only personal copies of library books are withheld from online providers. A personal-copy Bible is therefore sent, shortened, to an online provider, as it may be quoted into a note. Confirm with the user that this is what "personal copies of books" meant.
- ~~The old local preference `xegesis.assistant.provider` (T34a's placeholder) is no longer read by Settings; `prefs.provider` in `state.ts` and `providerText` in `shellwords.ts` can go once nothing else uses them.~~ Fixed in followups-1: `prefs.provider`, `providerText`, `normalizeProvider` and the provider list are gone.
- The licence gate limits each tool result on its own; a model that reads the same limited passage many times in one chat can receive more than one quoting limit in total, as several quotations in one sermon can (see After T19).

## After T36b (5 November 2026)

- The command lines for the real `claude` and `codex` (flags such as `--include-partial-messages`, `--allowedTools`, `--disallowedTools`, and Codex's `-c mcp_servers.xegesis.*` and `--sandbox read-only`) were written from the tools' documented JSON output and were never run against the real tools, as the task forbids. Try each once by hand and adjust `CLIAgent.Args` and `streamParser` in `internal/assistant/cli.go` if a flag or event name differs. Check in particular that Codex runs MCP tool calls in `exec` mode without asking.
- The chat is sent to the tool afresh on every question (the chat so far is in the prompt) rather than resumed with the tool's own session; long chats cost more.
- Tool lines from Claude Code and Codex use a label made from the call's arguments ("Read Romans 8 (KJV)"); the gate's own label and notice (for example "shortened to the licence limit") do not travel back through MCP. Putting them in the MCP result's structured content would let the chat show them.
- No model chips for the two tools: they use the model set in the tool itself. A free-text model name could be passed with `--model` / `-m`.
- Detection runs each tool with `--version` every time the Model page opens; cache it for a minute if a slow `claude` start-up shows.
- The `mcp-assistant` mode starts the whole app executable without a window; check on macOS and Linux builds, where the Wails binary may need a display to start.
- The Not found state is checked in e2e by hiding the fake `codex`; the install links open through `runtime.BrowserOpenURL`, which e2e does not click.
- ~~e2e: `structure.e2e.mjs` hover checks ("Headings fold…", "Outline strip…") fail 1–3 times per full run after T36b but pass alone. Make them wait for hover state, and check whether the base commit flakes too.~~ Fixed in followups-1: the cause was the page not settled when the pointer arrived (and a repaint of the marks that followed every highlight change); the checks wait for a still page, and a pane no longer repaints on its own announcement.

## After T38a (5 November 2026)

- **Bible-type books, first stage only.** Done: they are listed with the Bibles in a pane's Bibles
  chooser and in the sidebar Compare panel, marked as a book, and a reference opens the book at that
  chapter (`book.at_reference`, from the contents and headings, cached per file version). Not done:
  - Verse level. The map goes to the chapter only; verse numbers inside the text are not read.
  - Highlights and notes made in such a book are not yet linked to the reference, so they do not show
    in passage reports or "Notes on this passage".
  - PDF books: the map is built from the contents the core can read, and a PDF's outline is read by
    the interface, so PDFs are not mapped yet.
  - Contents entries that give only a number or a word ("Chapter Twelve") under no book heading, or
    other languages' book names, are not placed; `book.bible_map` lists them as unmapped.
  - Other lists still show only installed Bibles: Insert verse text, a sermon's Bible choice, Search
    and the Highlights panel.
  - Reading a book typed Bible in side-by-side columns with the installed Bibles.
  - Quoting from such a book follows its own licence through `book.copy`; nothing new was added.
- Fixed on the way: jumping to a place inside a book (contents, links, search hits) briefly showed the
  target's block hidden, because the book's highlight class clashed with the pop-up message class.
- A SWORD `.zip` installed with `bible.install` takes its licence from the module's `.conf`: public
  domain, Creative Commons and freely distributable stay as they say; copyrighted modules, and any
  that state nothing clear, are a personal copy that keeps the credit line.

## After T38b
- Study grid: at most 3 siblings per split and 9 panes; a drop that would break either is refused
  (use Move to... from the tab menu). Splits sit as a tree; "Equal sizes" evens every split.
- e2e: the `book.e2e.mjs` grid reader now understands the tree format as well as old column saves;
  `openNoteInStudy` goes back to the My work list first when an earlier check left a note open.
- Screenshots are git-ignored (`frontend/e2e/shots/`), so they are local evidence only.
- e2e: "Highlights in verses: clicking a highlight selects all of it..." failed in two full runs (a
  highlight box was null after recolouring) but passes alone and after every file that runs before it;
  looks like a timing flake in the highlights check, not grid-related. Worth a wait-for-mark there.
- Read mode book toolbar: a stray "+" shows before the section arrows, and the section field is very wide (seen in fu-03-book-overlap.png). Hide "+" outside Study and size the field to its number.

## After T37 stage 1
- Ink blocks (`frontend/src/editor/inknode.ts`, `ink.ts`) are stored as a fenced ```` ```ink ```` block of one line of
  JSON (`{"v":1,"h":height,"s":[{t,c,w,p:[[x,y,pressure]]}]}`), always 1000 units wide so strokes follow the
  note's width. New dependency: `perfect-freehand` 1.2.2 (MIT).
- Not done: lasso select and move (the brief said "if time allows").
- Pen tilt is read by the browser but not used; perfect-freehand only uses pressure and speed.
- HTML and PDF exports draw each stroke as a round-capped polyline with one mean width (HTML as SVG
  `<polyline>`), not the smoothed variable-width outline the editor shows. The Go side has no
  perfect-freehand. Word exports leave ink out (it stays in Markdown and HTML copies).
- A finger on a block never scrolls the page (`touch-action: none`); on a phone, scroll by touching text
  above or below the block. A "scroll with finger" switch in the strip would fix it.
- Pen, colour and width are shared by every block on screen and are not remembered after a restart.
- "New handwritten note" is in My work's New menu only; the right-sidebar notes panel and the
  Sermons list's New menu do not offer it yet (a sermon takes ink through /handwriting).
- Ink blocks do not move by dragging; cut and paste of the surrounding text moves them like a divider.
- Highlighter strokes in PDF are blended with white, not transparent, so they hide whatever is under them.
- e2e: the pen, touch and palm checks send synthetic pointer events (the harness has no pen); the
  mouse checks use real `page.mouse` input.
