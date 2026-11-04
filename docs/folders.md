# Folders and the bin

Source: Phase 1 survey, rows "Notes organised in notebooks or folders" and "Sermon or lesson builder";
`docs/ui-requirements.md`, "Library and My work" (T34c). Code: `internal/folder`, interface
`frontend/src/views/work.ts`.

## Folders

A folder holds the user's notes and documents (sermons, lessons and studies) together. Folders sit at the
top or inside one other folder: they nest one level, so a folder that holds folders cannot itself go inside
another. Lists, reading plans and reports are not filed in folders.

Migration 0010 adds the `folders` table (`id`, `name`, `parent_id`, `sort`, `created_at`, `updated_at`,
`deleted_at`) and a nullable `folder_id` on `notes` and `documents`. Every change goes through
`store.Write`, so folders have a history and can be restored like notes.

| Command | What it does |
| --- | --- |
| `folder.create` | Make a folder: `name`, and `parent_id` to put it inside a top-level folder |
| `folder.rename` | Rename a folder |
| `folder.move` | Move a folder to the top (`parent_id` empty) or into a top-level folder, optionally to a `position` among its neighbours |
| `folder.delete` | Put a folder in the bin with its subfolders and everything filed in them |
| `folder.list` | Every folder in order, each top-level folder followed by its own, with counts of `notes` and `documents` |
| `note.move` with `folder_id` | File a note in a folder; `""` takes it out. The note's passage and last-changed time stay |
| `document.move` | The same for a document |

`note.create` and `document.create` take a `folder_id` too, and `note.search` and `document.list` filter by
it (`none` for things in no folder).

## The bin

Deleted notes, documents, lists and folders wait in the bin until they are restored or deleted for good.

| Command | What it does |
| --- | --- |
| `bin.list` | What is in the bin, most recently deleted first. A deleted folder is one entry, with counts of the subfolders, notes and documents deleted with it |
| `bin.restore` | Bring back a `kind` (`folder`, `note`, `document` or `list`) and `id`. A folder comes back with what was deleted with it |
| `bin.purge` | Delete one entry for good, with everything it holds. This cannot be undone |

What went into the bin with a folder is what `folder.delete` deleted: the folder is deleted first, so
everything deleted with it has a `deleted_at` no earlier than the folder's. A note deleted on its own
before its folder keeps its own place in the bin. A note or document restored on its own while its folder
is still in the bin comes back at the top, so it is never out of sight; `note.restore` and
`document.restore` do the same.

Deleting for good removes the rows, a note's tags and the places it was attached (anchors nothing else
uses), and a list's entries. Their earlier changes stay in the change history so other devices still learn
of them, but lose their values (`store.Tx.Purge`): what is deleted for good cannot be read back from the
history either. The purge is logged as a `delete` whose field is `purge`. See ADR 0010.

## Interface

My work (`views/work.ts`) has a column beside each of its views: Folders ("All notes", then the folders, with a "+"
button that makes one named inline, and a ⋯ menu to rename, nest, move or delete), Tags with their counts, and Writing
(Sermons, Lists, Reading plans, Reports). Each section folds away and remembers it on the device; the Bin is
pinned at the bottom. Notes and sermons show as cards (`views/notes.ts`); sermons also as a table of Title,
Passage, Date preached and Theme (`views/sermons.ts`). Dragging a card or row onto a folder moves it there;
"Move to folder…" in its ⋯ menu does the same from the keyboard. Double-click or Enter opens an item in the
Study grid. The Bin (`views/bin.ts`) restores, and deletes for good after asking.

## Export

`export.data` writes folders as objects of type `folder` (with a `parent` relation and a `position`), and a
`folder` relation on each note and document. Exporting notes or documents always includes the folders.
