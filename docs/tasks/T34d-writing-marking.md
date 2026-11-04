# T34d: Writing and marking (notes, sermons, highlights, look-up, pane menu)

Brief for one agent (Opus), after T34c. Read these first:
- `AGENTS.md`;
- `docs/ui-requirements.md`;
- `docs/tasks/followups.md`, the section "User testing after T34c".

The user approved the "Writing and marking" mock-up on 4 November 2026. It's a reference file you
can read: its inline styles and script show the look and the behaviour. Match it closely, but copy
none of its sample text.

The user called the current note and sermon editors "terrible", and highlights "super unintuitive".
Polish matters here.

## 1. Note editor: a document, not a form

Remove the labelled boxes ("Title", "Bible reference", "Text", "Tags") and the toolbar of big text
buttons.

**Layout, top to bottom:**
- A **breadcrumb** (My work › folder), a **Saved** indicator, and **⋯** (Move to folder, Print or
  export, Delete).
- A **large serif title** you type straight into. Its placeholder is "Untitled".
- A **row of chips** for folder, verse references and tags:
  - the verse chip uses ✦ in the accent colour;
  - tag chips have ×;
  - "+ tag" adds a tag inline;
  - clicking a chip edits it in a small popover.
- A **slim sticky toolbar** of icon buttons, each with a tooltip and a keyboard shortcut. In order:
  - Bold, Italic, Underline;
  - a separator;
  - Heading, Bulleted list, Numbered list, Quote;
  - a separator;
  - Link, Verse reference.

  At its right sits a faint hint, "Type / for more".
- **The text:** serif, comfortable line length, no box border.

Add a **slash menu** in the shared Lexical editor (`frontend/src/editor/`), shown when you type `/`:
Heading, Bulleted list, Numbered list, Quote, Verse quote, Divider. Quoted verses render as a styled
quote block with the credit line in small sans type, as in the mock-up. They still go through
`document.quote` and its licence rules.

**Where the editor appears:**
- My work;
- the note tab in the Study grid;
- the right sidebar's Notes tool, which uses a compact version of the same editor and opens the full
  one in a tab.

## 2. Sermon editor

It uses the same document page as notes, plus:
- **Properties** under the title, as a two-column list of label and chip:
  - **Passage** (✦ chip);
  - **Preached** (a chip that opens a small month calendar with "Not preached yet" and "Today");
  - **Theme** (a chip that is editable inline);
  - **Kind** (Sermon, Lesson or Study).
- **Outline** in a narrow left column, built from the sermon's headings:
  - numbered, with each point's verse references shown faintly;
  - clicking a point scrolls to it;
  - "+" adds a point;
  - dragging a point reorders its section in the text;
  - Ctrl+] and Ctrl+[ change a point's level.

  This replaces the separate outline list and "Add the outline to the text" button: the headings are
  the outline. Migrate existing `outline_json` points into headings when a sermon opens, and keep
  `outline_json` in sync for export.
- **"✦ Quote verses"** in the toolbar opens a popover: a reference field, a live preview, a Bible chip
  and Insert.

## 3. Highlights

**Selection toolbar:** selecting text in any text pane (Bible, book, commentary) shows a small
floating toolbar just above the selection with:
- 5 colour dots;
- Underline;
- Note;
- Look up;
- Copy (licence-checked, with the credit line).

**Clicking an existing highlight** opens the same toolbar with:
- the current colour ringed;
- underline toggling;
- **Remove**.

The right-click menu stays as the keyboard and right-click alternative. The toolbar can be reached
with the keyboard after a selection.

**Highlights sidebar tool:** a new tool in the right sidebar, on by default, after Notes.
- It lists the highlights in the selected pane's book, or the current Bible book.
- Each row shows a colour bar, the quoted text and its place.
- It filters by colour chips.
- Clicking a row jumps to the place, and × deletes it.
- The footer links to "All highlights in My work".
- Add **My work → Highlights**: a page of all highlights across books, with the same filters and a
  search.

Use the existing `highlight.*` commands. Add any missing command, such as listing by book or colour,
or changing the style, with Go tests.

## 4. Word look-up: calmer

- Bible text shows **no underlines or clickable styling on words**. Words look like plain text.
- Look-up happens only when you ask:
  - **Alt+click** a word;
  - "Look up" in the selection toolbar;
  - the right-click menu.
- It opens a **small card** next to the word with:
  - lemma, transliteration, Strong's number and a short gloss;
  - the source lexicon;
  - "More in the Word tool →".

  Escape or clicking elsewhere closes it.
- A setting, **Settings → Reading → "Show tagged words"** (off by default), brings back subtle
  marking for those who want it.

## 5. Pane header and ⋯ menu

The header row holds, in order:
- the tabs and +;
- the chapter arrows around the place box;
- **Link** (the only place to link or unlink);
- **☰ Contents**;
- **⋯**.

**☰ Contents** is a popover with:
- a "Go to a book or chapter…" field;
- a grid of chapter buttons, for a Bible, or the table of contents, for a book;
- a "Recent" line with history.

**⋯** is grouped, with faint headings and shortcut hints:
- **This pane:** Search in this book (Ctrl+F), Print or export… (Ctrl+P), Copy link to this place;
- **Layout:** Split right, Split down, Move to…, Close pane (Ctrl+W, in a warning colour).

Remove "Unlink from the other panes", Contents and History from ⋯.

## 6. Small fixes

- Remove the × from the right sidebar. The top-bar toggle and Ctrl+. are enough.
- **Bug:** the auto-hide "show rail" button covers page titles, such as "My work". Reserve its space
  in the header, or place it so it never overlaps content. Nothing may be covered by the rail button
  or the sidebar at any window width.

## Rules

- Use `frontend/src/controls.ts` and the colour tokens, and make both Light and Dark themes look
  right.
- Every data change goes through a command. Write vitest tests for pure logic: outline from headings,
  the highlight range and toolbar position maths, the slash-menu filter, and the menu model. Write Go
  tests for any new commands.
- Commit after each numbered section that builds and passes. Don't commit `frontend/dist`. Stop any
  preview server you start.

## Done when

1. Notes and sermons use the document layout above, in My work, Study tabs and the sidebar. No
   labelled-box form and no big text-button toolbar remain.
2. Sermon properties are chips with a calendar for Preached. The outline is built from the headings,
   and existing outlines are migrated.
3. Selecting text shows the floating toolbar, and clicking a highlight lets you recolour or remove
   it. The Highlights tool and My work → Highlights list, filter, jump to and delete highlights.
4. Words carry no clickable styling by default. Look-up opens only on Alt+click, the toolbar or
   right-click, as a small card.
5. The pane header and ⋯ menu match section 5.
6. The sidebar has no ×, and the rail button never covers content.
7. `go test ./...`, `npm run build` and `npm test` pass.
