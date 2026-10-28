# T33: Simple import

Brief for one agent (Opus). Read `AGENTS.md` first, especially the design philosophy: simple things
simple, complex things possible.

## Problem

Adding books today means a folder path, a scan, then a blocking list of licence questions with six
internal types, SPDX fields and "Not sure". Installing a Bible from a file needs a licence form. Both
should take one action and no questions.

## What to build

1. **Licences by default, not by question.**
   - When a book or Bible states its own licence, use it automatically:
     - EPUB rights and licence metadata (public domain, Creative Commons IDs);
     - OSIS `<rights>` in the header;
     - USFM `\rem` and copyright lines;
     - an eBible.org `copr.htm` inside a zip (T25 already reads it);
     - CrossWire `.conf` fields.
   - Everything else gets **Personal copy** (`personal_purchase`).
   - Record where each licence came from (from the book's own details, the default, or set by the user)
     so the interface can say so. A migration may add a column for this; update the schema-version
     test to match.
   - Never overwrite a licence the user set.
2. **Four plain choices in the interface:**
   - Public domain;
   - Free to share (Creative Commons and similar open licences; optional licence ID under More options);
   - Personal copy (bought or borrowed);
   - My own work.

   Existing `unknown` licences display and behave as Personal copy. SPDX ID, rights holder, licence URL,
   credit line and custom print and export limits live under **More options** in the book's details.
3. **Add books in one step:**
   - One **Add books** button offers "Choose files" and "Choose a folder".
   - **Folder:** registers the folder, scans it straight away and keeps watching it. The app rescans online
     folders at start-up and every 10 minutes, which is cheap because unchanged files are skipped. Use file
     notifications instead only if a permissive library makes that simple.
   - **Files:** copies them into a default library folder, `Documents/Xegesis Library` (created and
     registered on first use), then scans.
   - **Drag and drop** files or folders onto the window does the same, if Wails v2 supports file drop;
     check its documentation.
   - Afterwards, one summary line, for example "12 books added: 3 public domain (from their own details),
     9 personal copies. Review".
4. **Review and change:**
   - **Review** opens the book list filtered to the books just added, with the licence shown in plain
     words and its source.
   - Each book shows "Licence: Personal copy · change".
   - Selecting several books allows "Set licence for selected".
   - Remove the old licence questions panel.
5. **Bibles from a file:** "Install from file" asks for the file only. The licence comes from the file
   where it says one, otherwise Personal copy, with the same change link afterwards.
6. **Commands stay complete:** whatever the screens do is a command with plain parameters, such as
   `library.add_files` and licence fields with defaults. Keep existing commands working for the CLI and
   MCP. Error and status messages say what to do in plain words.

## Rules

- Work only in your worktree. You may change `internal/library`, `internal/bible` (licence detection
  only), `internal/licence` (labels or helpers), `app/` (timer, drag and drop, dialogs),
  `frontend/src`, and add one migration. Do not commit `frontend/dist`.
- Test with tiny fixtures. Use a scratch library, never `%APPDATA%\Xegesis`.

## Done when

- Adding a folder of mixed books (EPUB with public-domain rights, PDF with no licence, DOCX) takes one
  click and asks nothing, and the summary line is right.
- Adding single files copies them into the default library folder and scans them.
- A licence can be changed for one book and for a selection, in plain words, and the change survives a
  rescan.
- A Bible installs from an OSIS or USFM file with no licence form, picking up the file's own rights where
  present.
- Old `unknown` licences show as Personal copy.
- `gofmt`, `go vet`, `go test`, the frontend tests and build, and the desktop build pass.
