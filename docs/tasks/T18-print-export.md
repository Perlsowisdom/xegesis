# T18: Printing and exporting by licence

Brief for one agent (Opus). Read `AGENTS.md` first: design philosophy (simple by default, plain words),
every feature a command, permissive dependencies only.

## Idea

The user found printing and exporting in their old software poor. In Xegesis it is simple, and it
follows each item's own licence instead of one rule for everything.
- Public domain and your own work have no limits.
- Free-to-share items carry their credit line.
- Personal copies print and export up to their limit, per item, and say so plainly.

## What to build

1. **One print and export flow** for every kind of content:
   - Bible passages (one or parallel);
   - book excerpts and pages;
   - notes and lists;
   - reading plans;
   - reports (T13, through `report.*`, when merged; build the flow so reports plug in);
   - sermon documents later (T17).
2. **Formats:**
   - Print uses the system print dialog, which on every desktop system also offers "save as PDF".
   - Export to PDF, Word (`.docx`), HTML and Markdown (Markdown by any's rules, as T29).
   - Use permissive libraries only, and confirm each licence. For example, a small writer of your own for
     `.docx` (it is a zip of XML), and a permissive Go PDF library or printing HTML to PDF in the web view
     if that works on all three systems. No GPL.
3. **Licence rules:**
   - Every piece of content in an export carries its item.
   - The export goes through `licence.Check`, with print or export limits measured the same way as
     `book.copy` (T26), counted per item per day in `copy_log`.
   - Over a limit, the preview shows exactly what is cut and why ("Personal copy: up to 10% of this book
     per day; 3 of 12 pages fit"), and offers to print the rest later. It never fails silently.
   - Credit lines appear at the end of each piece, or in a credits section.
4. **Preview first:** a preview pane shows the result with options folded under More options:
   - page size, font size, include verse numbers, include notes;
   - one or two columns for parallel Bibles;
   - header and footer, and credits at the end or under each piece.

   The common path is one click.
5. **Commands:** `export.preview` (what would be produced, and what licences cut) and `export.make` (write a
   file in the chosen format), plus a print path the interface calls. Plain messages throughout.

## Rules

- Work only in your worktree.
- Another agent (T13, reports) works at the same time:
  - do not build report data; call `report.*` only if present, and keep the report entry point ready;
  - use migration **0009** if you need one; T13 uses 0008.
- Do not commit `frontend/dist`.
- Use a scratch library (`XEGESIS_LIBRARY`).

## Done when

- A passage in two Bibles prints and exports to PDF, Word, HTML and Markdown, with credit lines.
- A purchased book's excerpt over its limit is cut, with the plain explanation in the preview, and the
  daily total is enforced across exports.
- Notes, lists and a reading plan export.
- Files open correctly in their usual apps (check the `.docx` with a zip and XML validity test, and the
  PDF with a parser).
- `gofmt`, `go vet`, `go test`, the frontend tests and build, and the desktop build pass.
