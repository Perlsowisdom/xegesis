# Sermons, lessons and Bible studies

Source: Phase 1 survey, row "Sermon or lesson builder with verses and outlines" (T17). Code:
`internal/document`, interface `frontend/src/views/sermons.ts`.

A document has a kind (`sermon`, `lesson` or `study`), a title, a main passage, an outline and its text
in Markdown. It is stored in the `documents` table from migration 0001. Migration 0010 adds `preached_on` (an
ISO date, YYYY-MM-DD, optional), `theme` (a few words, optional) and `folder_id` (the folder it is filed in;
see [folders.md](folders.md)).
`outline_json` is `{"passage": "Romans 8:28-39", "points": [{"h": "…", "ref": "Rom 8:28", "level": 1}]}`.
Rows that hold only an array of points (`[{"h": "One"}]`) are still read.

## Commands

| Command | What it does |
| --- | --- |
| `document.create` | Start a document; every field is optional (the title defaults to "Untitled sermon" and so on), including `preached_on`, `theme` and `folder_id` |
| `document.get` | One document with its outline, text and word count |
| `document.list` | Newest first; filter by `kind`, `query` (words in title, passage, outline, text or theme), `ref` (main passage or any point overlapping it) or `folder_id` (`none` for documents in no folder) |
| `document.update` | Change any of kind, title, passage, points (the whole outline), body, `preached_on` or `theme` (`""` clears either) |
| `document.move` | File it in a folder (`folder_id`), or take it out of its folder (`folder_id` empty); its last-changed time stays |
| `document.delete`, `document.restore`, `document.deleted` | Soft delete, as for notes |
| `document.quote` | Add a Bible passage to the end of the text as a block quote with a link, the Bible's short name and its credit line |

Every write goes through `store.Write` and is in the change history. Documents are exported with
`export.data` (group `document`, with `preached_on`, `theme` and a `folder` relation) and printed or saved with `export.preview` and `export.make` using the
source `{"kind": "document", "id": "…"}`: the outline as a numbered list, then the text.

## Quoting and licences

`document.quote` follows the Bible's licence:

- a Bible whose licence does not allow keeping a copy (`store_allowed` false) is never copied; the
  reason is returned instead;
- a Bible with an export limit may be quoted up to its page limit × 2,000 characters at a time (the page
  equivalent `book.copy` uses), or one page when the limit is a percentage only, because a Bible's total
  length is not measured here (an estimate, to revisit);
- every quotation carries the Bible's credit line when its licence asks for one.

## Interface

The Sermons section lists documents on the left (search, kind filter, New, Recently deleted). An open
document shows its title, kind and main passage, the outline (Enter adds a point, Tab makes a sub-point,
each point can hold verses and quote them), a Quote verses box, and the text editor. Changes save on
their own after a short pause, or with Ctrl+S. "Add the outline to the text" puts each point in the
text as a heading to write under. The address `#sermons/<id>` opens one document.
