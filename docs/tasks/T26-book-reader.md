# T26: Book reader

Brief for one agent (Opus). Read `AGENTS.md` first. This finishes T10, whose finish condition was
"opens a book and a Bible side by side".

## What to build

1. **Reading commands** (Go, `internal/reader`): `book.open` (item ID → table of contents, format,
   page or section count), `book.section` (one EPUB spine item, DOCX/HTML/MD/TXT section, or PDF page
   as sanitised HTML or page info), `book.locate` (a search locator from `search.query` → section and
   position). Reuse the text extraction in `internal/search`. Content is sanitised: no scripts and no
   remote resources, and images come only from inside the book file.
2. **PDF rendering** with pdf.js (Apache-2.0) in the interface, fed the file's bytes through a
   command; no GPL renderers.
3. **Reader pane**: opens any library book next to a Bible pane. It has a table of contents, a
   position you can return to, and font, size and theme settings for reflowable books. Verse
   references in the book text link to the Bible pane (`search.refs` data). Search results from
   `search.query` open at their locator.
4. **Notes and highlights on books**: select text → highlight or note, stored as anchors with
   `locator_json` (EPUB CFI with a text offset, or PDF page and rectangles). Add `highlight.create`,
   `highlight.list`, `highlight.delete`; extend `note.create` to take a book anchor.
5. **Copying follows the licence**: copying text from a book uses `licence.Check` with the export
   limit (per copy and summed per book per day) and appends the attribution line. Over the limit,
   the copy is refused with the reason shown.
6. **Library search in the interface**: the Search view gains Library and Bible tabs, plus "books
   citing this passage".

## Done when

A library EPUB, a PDF and a DOCX each open beside a Bible. A search hit opens at the right place.
Highlights and notes on a book survive a restart. Copying a purchased book is limited and carries
attribution. Tests cover the commands, locators and copy limits. `go vet`, `go test` and the
desktop build pass.
