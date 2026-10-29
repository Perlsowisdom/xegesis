# T14: Notes, highlights and saved lists

Brief for one agent. Read `AGENTS.md` first, especially the design philosophy (simple things simple,
complex things possible) and plain words.

## Already built

- Notes on verses and books (`note.*`) and highlights on books (`highlight.*`, T26).
- Version history with restore (`history.*`).
- The `anchors` table, which places a note on a verse range or a book location.
- The `lists` and `list_entries` tables exist, but nothing uses them yet.

## What to build

1. **Notes that are pleasant to use:**
   - A Notes section listing all notes, newest first, with search through note text.
   - Filters by book of the Bible, by library book and by tag.
   - Tags: free-text and suggested from existing tags, stored as notes facets or a small migration.
   - Markdown editing with a preview.
   - A note can be attached to a passage or a book place, moved to another place, or left free.
   - A "History" button on each note lists earlier versions; choosing one restores it, and the restore can
     itself be undone.
   - Deleted notes can be found and restored later, not only by Undo in the same session (a "Recently
     deleted" view).
2. **Highlights on Bible verses** as well as books:
   - Select verses in the Reader and highlight them in a colour.
   - A highlight shows in every Bible that covers those verses; convert verse numbering with
     `internal/verse`.
3. **Saved lists**, using the existing tables:
   - **Verse lists:** add the current passage or selected verses ("Add to list"). A list opens each
     verse in the Reader, and can be reordered and exported with T29.
   - **Word lists:** add a word from the word panel (T27) with its Strong's number and lemma.
   - **Excerpt lists:** add selected book text, which counts as a copy, so `book.copy` licence limits
     and attribution apply.
4. **Commands:**
   - `note.search`, `note.tags`, `note.move`, `note.deleted` and `note.restore`;
   - `highlight.create` for verse anchors;
   - `list.create`, `list.rename`, `list.delete`, `list.add`, `list.remove`, `list.reorder`, `list.items`
     and `list.list`.

   Every write goes through `store.Write`, so it has history.

## Rules

- Work only in your worktree.
- Another agent (T16, reading plans) works at the same time:
  - put your interface in new files (`views/notes.ts`, `views/lists.ts`) with minimal wiring in
    `main.ts`;
  - do not touch reading plan code;
  - use migration number **0006**; T16 uses 0007. Update the schema-version tests to match.
- Do not commit `frontend/dist`.
- Use a scratch library (`XEGESIS_LIBRARY`).

## Done when

- A note can be created on a verse, a book place or nowhere, then tagged, searched, moved, edited,
  restored to an earlier version, deleted and found again under Recently deleted.
- Verses highlighted in one Bible show highlighted in another that uses different numbering.
- Verse, word and excerpt lists can be made, filled, reordered and opened; excerpts respect licence
  limits.
- `gofmt`, `go vet`, `go test`, the frontend tests and build, and the desktop build pass.
