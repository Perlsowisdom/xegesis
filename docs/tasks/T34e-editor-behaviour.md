# T34e: Editors that behave like the user expects, tested in the real app

Brief for one agent (Opus). Read these first:
- `AGENTS.md`;
- **`docs/editor-behaviour.md`**: the acceptance checklist the user approved on 4 November 2026;
- `docs/tasks/followups.md`, the section "User testing of T34d";
- `docs/tasks/T34d-writing-marking.md`, the visual design. Keep it.

**Background.** The user has said three times that the note and sermon editors miss the mark. The
cause: work was judged by how it looked in a browser preview, not by typing in the real app. This
task fixes the *behaviour*, and proves it in the real app.

## Scope

This task covers everything in `docs/editor-behaviour.md` **except "Highlights inside verses"**. That
is T34f.

### 1. A real-app test harness (do this first)

The desktop app runs in WebView2, which can expose the Chrome DevTools Protocol:
- Start the app with the environment variable
  `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=<port>`.
- Point `XEGESIS_LIBRARY` at a temporary **copy** of the scratch library (`%USERPROFILE%\Documents\Xegesis-scratch3`, or the folder `XEGESIS_E2E_SOURCE` names).
  Never use the original or the user's default library.
- Connect with `playwright-core`'s `chromium.connectOverCDP`. It's Apache-2.0; add it as a dev
  dependency only.

Build `frontend/e2e/`:
- a launcher script that builds the app, copies the scratch library, starts the app with the
  debugging port, connects, and **closes the app at the end**;
- one test file per checklist section. Each test **types with real key events** (`page.keyboard`),
  checks the resulting DOM, and saves a screenshot to `frontend/e2e/shots/`, which is git-ignored;
- `npm run e2e` runs it all on Windows.

This harness also starts T30 (interface tests). Note that in `docs/tasks/followups.md`.

### 2. Markdown as you type

Make every row of the "Markdown as you type" table pass, using Lexical's Markdown shortcut
transformers (`@lexical/markdown`):
- Backspace at the start of a heading or list item turns it back to plain text.
- Pasting Markdown text arrives formatted.

### 3. Headings and structure

- The heading control offers Large, Medium and Small (H1, H2 and H3), with Ctrl+Alt+1, 2 and 3.
- **Folding headings** (an arrow beside each heading) in notes and sermons. Folding is display only,
  remembered per document per device.
- A slim **outline strip** at the right edge: a mark per heading, the names on hover, and a click
  to jump.
- The sermon's left outline column shows heading levels by indent.

### 4. Inserting

There is **one Insert control**:
- its icon is a book with a plus (not a sparkle);
- it sits in the toolbar of both notes and sermons.

Remove the duplicate "Quote verses" button from the sermon toolbar.

The Insert control offers:
- **Verse reference:** a linked chip that opens the passage in the Study grid when clicked.
- **Verse text:** the quote block with its credit line, through `document.quote`'s licence rules.
- **Highlight:**
  - a searchable picker of the user's highlights, showing their colours;
  - it inserts the highlighted words as a quote, with the place and a link back;
  - T34f will make highlights partial, so read the highlight's text through a function that can take
    character ranges later.
- **Note link:** links to another note or sermon.

These also work:
- the `/` menu: `/verse`, `/quote`, `/highlight`, `/link`;
- typing a reference such as `Rom 8:28` and pressing **Tab**, which turns it into a reference chip;
- dragging a verse from a Bible pane, a highlight from the Highlights tool, or a Related card into the
  editor. Each inserts the matching block.

Store references, verse quotes, highlight quotes and note links in the note's Markdown in a form that
round-trips: save, reload and export (`internal/export`). Add vitest tests for the round trip.

## Rules

- Keep the approved look: document page, chips and slim toolbar.
- Use the colour tokens and both themes.
- Every data change goes through a command.
- Commit after each numbered section once it builds and its tests pass.
- Don't commit `frontend/dist`.
- **Close every app instance and server you start.**

## Done when

1. `npm run e2e` launches the real app on a copied library and passes a typed test for **every
   checklist row in scope**, with screenshots saved.
2. `npm run build`, `npm test` and `go test ./...` pass.
3. The final report lists each checklist row with pass or fail and its screenshot file name.
