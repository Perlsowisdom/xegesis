# T34c: Library and My work, redesigned

Brief for one agent (Opus), after T34a, T34b and their fixes (all merged). Read these first:
- `AGENTS.md`;
- `docs/ui-requirements.md`, especially "Library and My work" and the round 3 feedback;
- `docs/tasks/T34a-shell.md`;
- `frontend/src/controls.ts`: use its switches, pills, chips and tick boxes. Don't use plain
  checkboxes.

The user said the Library "looks good so far", so the Library work is polish. My work is the bigger
part.

## Library

- **One Filter button** opens a popover with every facet:
  - type (Bible, commentary, dictionary or lexicon, theology, and others in the data);
  - tradition;
  - author;
  - era or year;
  - licence group.

  Several choices can be made at once, within a facet and across facets: within a facet means OR,
  across facets means AND. For example, Bible and Commentary, with Reformed.
- Only the **active filters show**, as removable chips next to the button, with "Clear all".
- A count reads "12 of 140 books".
- Show book **covers** as a grid, or as a list, chosen by a pill switch. The choice persists.
- A **detail pane** for the selected book has Read, Open in Study, and licence details folded away.
- Get Bibles stays a tab of the Library.
- Use the existing facet and collection commands (`docs/facets.md`, `docs/collections.md`). No new
  facet storage.

## My work

The left column has three sections that **collapse** (▾ and ▸, remembered per device):
- **Folders:**
  - "All notes", then the user's folders;
  - a **+** creates a folder, named inline;
  - folders can be renamed, deleted (to the bin) and nested one level;
  - a folder holds **notes and sermons together**.
- **Tags:** every note tag, with counts.
- **Writing:** Sermons, Passage lists, Reading plans and Reports.

The **Bin** is pinned at the bottom of the column on every view. It shows soft-deleted notes,
sermons, lists and folders, with Restore and "Delete for good". "Delete for good" asks first.

**Main area:**
- Notes show as cards with title, first line, folder, tags and when edited.
- Sermons show as a table with **Title, Passage, Date preached and Theme**.
- Each view has a search box and **+ New**. + New offers a note or a sermon in a folder, and the
  matching thing elsewhere.
- Items can be moved to a folder by drag and drop, and by a "Move to folder…" menu as the keyboard
  alternative.
- Opening an item puts it in the Study grid as a tab (T34b's open API). Double-click or Enter opens
  it.

## Data changes (Go)

- **Folders:** a folders table (id, name, parent_id, sort, created, updated, deleted). Notes and
  documents (sermons) get a nullable folder_id.
  - Use `store.Write` and the changes log like other tables, so history, restore and sync work.
  - Commands: `folder.create`, `folder.rename`, `folder.move`, `folder.delete`, `folder.list`,
    `note.move` and `document.move`. Each has a JSON schema from struct tags as usual.
- **Sermons:** add `preached_on` (a date, optional) and `theme` (text, optional) to documents. Show
  and edit them in the sermon view (`views/sermons.ts`), the sermon tab (`document/sermondoc.ts`)
  and the table above.
- **Migration number:** the next migration is **0010**. 0008 was never used, and `user_version` is
  already 9, so a new 0008 would be skipped. Use `0010_folders_sermon_fields.sql`.
- Update `docs/documents.md` and the notes docs with the new fields and commands. Add export support
  in `internal/export` if notes and sermons are exported by folder today. If not, include the folder
  path in each exported note's properties.

## Rules

- Every data change goes through a command. Write Go tests for the new commands, including deleting
  a folder (its items go to the bin with it, and restoring brings them back) and moving an item.
- Write vitest tests for the pure logic: filter combination, the folder tree, and the sort.
- Commit after each step that builds and passes tests. Don't commit `frontend/dist`.

## Done when

1. The Library has one Filter popover with AND/OR combinations, removable chips, Clear all, a count,
   grid/list switching and the detail pane.
2. My work has collapsible Folders, Tags and Writing sections, and the Bin pinned at the bottom.
3. You can create, rename, nest one level, and delete folders. A folder holds notes and sermons.
   Items move by drag and by Move to folder….
4. Sermons have Date preached and Theme, shown in the table and editable.
5. The bin restores and deletes for good, including folders and their contents.
6. Migration 0010 applies cleanly on a copy of an existing library.
7. `go test ./...`, `npm run build` and `npm test` pass.
