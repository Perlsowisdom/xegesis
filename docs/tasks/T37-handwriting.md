# T37: Handwritten notes

Brief for one agent (Opus), after the T34 redesign fixes. Read `AGENTS.md` first. Requested by the
user on 3 November 2026.

## Design sources

These are for ideas only. Copy no code from any of them:
- **obsidian_ink:** writing and drawing blocks inside a typed note. It is built on tldraw, whose SDK
  needs a licence key or shows a watermark, so neither it nor tldraw can be used.
- **Zotero:** ink annotations on PDF pages. It is AGPL.
- **Saber:** handwritten notebooks. It is GPL-3.0.

Describe the feature in Xegesis's own words. Do not reuse their screen names or layouts.

## Stage 1: ink blocks in notes and sermons

- Add a new block type in the shared Lexical editor (`frontend/src/editor/`): an **ink block** you
  write or draw in with a pen, mouse or finger.
- Pen input:
  - use pointer events for pressure, tilt and the pen's eraser end;
  - with a pen present, ignore touches from the palm;
  - draw stroke outlines with **perfect-freehand**, which is MIT-licensed.
- Tools:
  - pen, highlighter and eraser (by whole stroke);
  - colours from the highlight palette;
  - three widths;
  - undo and redo;
  - lasso select and move, if time allows.
- The block grows downwards as you write. Its width follows the note.
- Strokes are stored as vector data with the note:
  - a compact JSON of points with pressure, inside the note's Markdown;
  - store it as a fenced block, following any's canonical Markdown rules (see T14 and T29).
- Export draws ink as SVG in HTML and PDF exports, and keeps the stroke data in Markdown exports.
- A note made only of ink blocks is a handwritten page. "New handwritten note" starts one.
- It must work in every study-grid pane, and at phone and tablet widths.

## Stage 2: ink on PDF pages

- Add an ink layer over PDF pages in the shared renderer (`frontend/src/document/pdf.ts`). Strokes are
  pinned to page coordinates, so they line up at every zoom.
- Store strokes as annotations through commands, next to highlights. The book file never changes.
- Show and hide ink per book. Ink is included when exporting annotations.

## Not in scope

- **Freehand ink on Bibles and reflowing books (EPUB, HTML):** text moves when size or width change,
  so ink would drift. Highlights, underlines and notes cover this.
- **Handwriting recognition (search inside ink):** a later task, using a local, offline model only, per
  the privacy rules.
- **Importing Saber notebooks:** only if its format is documented well enough to read without its
  code.

## Done when

1. Notes and sermons can hold ink blocks, written with a pen using pressure, and with a mouse or touch.
2. Pen, highlighter, eraser, colours, widths, undo and redo work, and palm touches are ignored while
   a pen is used.
3. Ink is saved with the note, survives restarts and backups, and exports as SVG in HTML and PDF.
4. "New handwritten note" makes an ink-only note, and it opens as a study-grid tab.
5. Ink on PDF pages stays aligned at every zoom, and is stored as annotations through commands.
6. Unit tests cover the stroke data format and the Markdown round trip. `npm run build`, `npm test`
   and `go test ./...` pass.
