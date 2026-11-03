# T34b: Study grid with tabs, and the shared document renderer

Brief for one agent (Opus), after T34a (the new shell) is merged. Read these first:
- `AGENTS.md`;
- `docs/ui-requirements.md`;
- `docs/tasks/T34-read-study.md`, the original T34 brief, whose renderer and mode rules still apply;
- `docs/tasks/T34a-shell.md`.

## Scope

### 1. Shared document renderer

Build the shared renderer that T34 describes, in `frontend/src/document/`:
- one component draws a Bible passage, a reflowable book (EPUB, HTML, text) or a PDF;
- it can be **mounted many times** on one page, once per pane.

Move the drawing code out of `views/reader.ts`, `views/book.ts` and `pdfview.ts` into it. The old
views become thin wrappers or are removed.

Read mode shows one document full width, with Aa reading settings (size, spacing, Dark, Sepia or
Light), and the settings persist per device. Read mode shows the selected pane's document, and
switching modes keeps your place.

### 2. Study grid

Study mode is a grid of **panes**, up to 3 columns with up to 3 panes in each.

**Each pane:**
- has its own **tab strip**, with a **+** for a new tab and an × to close each tab;
- has, beside the tabs:
  - a **place box**: the passage for Bibles, commentaries and lexicons, or the chapter for books;
  - a **link button**: linked panes follow one shared passage, and an unlinked pane keeps its own;
  - a **⋯ menu**: Contents, History, Search in this, Print or export, Close pane.
- When its last tab closes, the pane closes, and an empty column closes with it.

**A tab can hold any of these:**
- a Bible, commentary, lexicon or other library book;
- a note, opened in the shared editor from `frontend/src/editor/`;
- a sermon, showing its passage, date preached and theme, with its outline in the editor;
- a passage list;
- a compact **Library browser**, whose rows can be dragged into the grid;
- a compact **My work browser**.

Only Read mode is outside the grid.

**Drag and drop:**
- Drag a tab onto another pane's middle to move it there.
- Drag it onto a pane's left, right, top or bottom edge to split that pane. A new column or a new pane
  is created, up to 3 × 3.
- A dashed preview shows where it will land.
- Rows from the Library browser, the My work browser and the right sidebar's Related cards can also
  be dragged in.
- Use HTML5 drag and drop with a keyboard alternative: the ⋯ menu's "Move to…" with
  left, right, above, below and into another pane.

**Grid presets:**
- 1×1 up to 3×3, picked from a 3 × 3 chooser. It opens from the rail button (T34a's hook) or the
  Reader top bar, depending on the "Grid presets on the rail" setting.
- Choosing one spreads the open tabs across the new layout: it splits panes that have several tabs
  before opening defaults, and merges extra panes into the last one.
- "Make panes equal" resets sizes.

**Resizing:** drag the gaps between columns and between panes. There is a minimum size, and the sizes
persist.

**Layout persistence:** the whole grid (columns, panes, tabs, the current tab, links and places) is
saved per device, and restored at start.

**Selected pane:** the highlighted pane is where + and the sidebar act. The right sidebar's Related
and Word tools follow the selected pane's passage.

### 3. Context menu

The right-click menu and highlights from T34 work in every pane: Look up, Highlight, Add a note and
Copy within the licence.

## Rules

- Keep the layout model (columns → panes → tabs) as pure TypeScript in `frontend/src/document/grid.ts`
  or similar. Unit-test the drop rules, presets, closing and persistence with vitest.
- Memory: panes that aren't visible must not keep big PDFs rendered. Unmount or virtualise, and keep
  within GOMEMLIMIT thinking on the Go side (AGENTS.md).
- Commands for everything that touches data. Don't commit `frontend/dist`.

## Done when

1. Bibles, EPUB, PDF and books draw through the shared renderer in both modes, and the old separate
   reader code is gone.
2. The grid holds up to 3 × 3 panes, and each has tabs, +, × and a place box. Linking works across
   panes.
3. Dragging a tab to another pane's middle or edge moves it or splits the pane as described, with a
   preview. There is a keyboard "Move to…" alternative.
4. Notes, sermons, lists, the Library browser and the My work browser open as tabs. Library rows drag
   into the grid.
5. The presets and resizing work, and the layout survives a restart.
6. The right-click menu and highlights work in every pane type that has text.
7. Unit tests cover the grid model. `npm run build`, `npm test` and `go test ./...` pass.
