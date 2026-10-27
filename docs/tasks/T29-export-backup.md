# T29: Export and backup of your own data

Brief for one agent (Sonnet). Read `AGENTS.md` first.

## Format

Exports use the value encodings of **any** (anyproto/any, MIT), so the data can be imported into any
later and stays readable by people and tools. Sources, checked 27 October 2026; verify against the
current pages before building:

- Document bodies: any's canonical Markdown
  (https://docs.any.org/database/markdown-import-export.html). Blocks are separated by one blank
  line, every further blank line is an empty paragraph, and a single trailing newline is a
  terminator.
- Properties: any's kinds and `xFormat` slugs (https://docs.any.org/database/data-types.html).
  Datetimes are `{"$date": "<RFC 3339>"}`, a `date` is midnight UTC, `choice` values are arrays of
  option keys, and `relation` values are arrays of `any://<objectId>` URIs.

any does not document a whole-space archive format, so Xegesis defines a folder layout that uses
those encodings:

```
xegesis-export-<date>/
  manifest.json          format version, app version, device, export time, counts
  types.json             one entry per object type: xKey, name, and properties with kind and xFormat
  objects/<type>/<id>.json   the object's properties, in any's JSON encoding
  objects/<type>/<id>.md     the body (notes, sermons, documents) in any's canonical Markdown
  history.jsonl          optional: the change log, one change per line
```

Verse anchors become properties (`verse_start`, `verse_end` as numbers plus a `ref` text such as
"Romans 8:28"). Book anchors keep `locator_json` as an object property.

## What to build

1. `export.data` (everything, or chosen types, written to a folder or a .zip), `import.data`
   (merge an export back in: matching IDs update, new IDs are created, and every write is logged),
   and `backup.create` (a consistent copy of `library.db` made with SQLite's online backup, while
   the app runs).
2. A scheduled automatic backup setting (daily or weekly, keeping the last N copies), stored in
   `settings`.
3. **Interface**: Settings → Your data, with export, import, backup now and the backup schedule.

## Done when

Exporting and importing back into an empty library reproduces notes, highlights, lists, collections,
reading plans and documents exactly. The Markdown round-trips by any's rules, including empty
paragraphs. Tests cover both directions and the backup of a database that is open.
