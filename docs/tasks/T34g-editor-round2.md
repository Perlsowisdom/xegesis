# T34g: Editor round 2, tables, and reading EPUBs again

Brief for one agent (Opus), after T34e. Read these first:
- `AGENTS.md`;
- `docs/editor-behaviour.md`, the approved checklist;
- `docs/tasks/T34e-editor-behaviour.md`.

Use the real-app harness (`frontend/e2e/`, `npm run e2e`). Every item below needs an e2e test that
types in the real app, and a screenshot.

This is the user's feedback from 4 November 2026.

## 1. Remove the sermon's left outline column

The user never asked for it; it came from our mock-up. Remove it completely. The right-edge outline
strip stays, in both notes and sermons, and it is the only outline view. Update
`docs/editor-behaviour.md`, which still says "In sermons, the left outline column lists the same
headings".

## 2. Nothing hides behind the sticky toolbar

The slim toolbar sticks to the top and covers whatever scrolls under it: the outline-strip popover
and headings you jump to.
- Jumping to a heading scrolls it to just below the toolbar.
- Popovers open above the toolbar's layer, or beside it.
- Test this at a short window height.

## 3. `__text__` underlines

`__text__` makes **underline**, not bold. `**text**` stays bold, and `*text*` and `_text_` stay
italic. Underline must round-trip through Markdown. Pick one form, such as `<u>text</u>`, and
document it in `docs/editor-behaviour.md`. Update that file's table.

## 4. Every Markdown format is also a button

Everything Markdown can do must also be in the toolbar:
- bold, italic, underline, inline code;
- heading sizes;
- bulleted list, numbered list, checkbox list;
- quote, divider, link, table, Insert.

Group them with separators. If the row overflows a narrow pane, the rest goes into a "⋯ More"
overflow, never off-screen.

## 5. The `/` menu: all formats, alphabetical

The `/` menu lists every format and insert:
- Bulleted list, Checkbox list, Code, Divider;
- Heading large, Heading medium, Heading small;
- Highlight, Link, Numbered list, Quote, Table, Verse reference, Verse text.

Sort them **alphabetically**, with typing to filter. Keep the existing aliases (`/verse`, `/quote`,
`/highlight`, `/link`) working as filter words.

## 6. Tables (notes and sermons)

**Making a table:**
- **Toolbar:** a table button opens a size picker, a grid you hover over to choose rows by columns, up
  to 10 by 10, with an "R × C" label.
- **Slash command:** `/table` inserts a 3 × 3 table. `/table3x2` inserts **3 rows by 2 columns**.
  Both `x` and `×` work as the separator. Anything over 10 is capped.
- Use `@lexical/table`.

**Inside a cell:**
- Every inline format works: bold, italic, underline, code, links, verse chips.
- Markdown shortcuts work too.
- Tab moves to the next cell; Tab in the last cell adds a row.
- Shift+Tab moves back.

**Editing the table:**
- **Columns resize** by dragging their borders, and the widths are saved.
- Right-click, or a small handle, gives: add a row above or below, add a column left or right,
  delete a row, delete a column, delete the table, and toggle a header row.

**Saving and export:** tables save as GitHub-style Markdown tables (any's canonical Markdown).
Column widths are kept in a way that round-trips, such as a comment or attribute the importer reads.
They export to HTML, Word and PDF. Add round-trip tests.

## 7. EPUBs won't open

The user can't read an EPUB at the moment, so highlighting and quoting in books can't be tested.
This likely broke when T34b moved `views/book.ts` to `document/book.ts`.
- Find the cause, fix it, and add an e2e test. It adds an EPUB to the copied library, opens it in
  Read mode and in a Study pane, scrolls, and selects text.
- Use a small **generated** EPUB fixture, made by the test, not the user's books.
- Also check that PDFs still open.

## Done when

- Each section has a passing e2e test with a screenshot.
- `npm run e2e`, `npm test`, `npm run build` and `go test ./...` pass.
- The report lists every item as pass or fail, with screenshot names.
