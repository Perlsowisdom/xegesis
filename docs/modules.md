# Bible module format

Each Bible is one read-only SQLite file, `modules/<id>.db` (ADR 0002). The `bible.install` command
creates it by importing OSIS 2.1.1, USFM 3 or USX 3 source files; nothing else writes it. A module can
always be rebuilt from its sources, so the format may change between versions: `PRAGMA user_version`
holds the format version (currently 1) and Xegesis refuses files of another version.

The module ID is lowercase letters, digits, `-` and `_` (at most 64 characters) and is also the file
name.

## Tables

### `module`

One row describing the module.

| Column | Contents |
| --- | --- |
| `id` | Module ID, same as the file name |
| `kind` | `bible` |
| `name`, `abbreviation` | Full and short names |
| `language` | BCP 47 language tag; `und` when unknown |
| `versification` | Verse numbering the verse IDs use: `english`, `hebrew`, or another name with a mapping table |
| `licence` | JSON of `internal/licence.Licence`, as the user stated it at import (`confirmed_by_user` is true) |
| `source`, `version` | Where the text came from and which edition |
| `books` | Space-separated OSIS book IDs present, in canonical order |
| `verse_count` | Rows in `verses` |
| `created_at` | Unix time in milliseconds |

### `verses`

| Column | Contents |
| --- | --- |
| `id` | Verse ID `BBCCCVVV` (`internal/verse`) in the module's own numbering; verse 0 is a superscription |
| `end_id` | Last verse covered. Equal to `id` except for combined verses such as "1-2", which are one row |
| `text` | Plain text: no headings, notes or markup, white space collapsed |
| `markup` | The same text with light markup (below) |

A passage `start..end` is the rows with `id <= end AND end_id >= start`, so asking for verse 2 finds a
combined verse 1-2.

### `strongs`

`(number, verse_id)`: every Strong's number tagged in a verse, normalised to `H7225` / `G26` (no leading
zeros). Used by `bible.search` with `strong`.

### `versification_map`

`(system, module_id, system_id)`: an optional full mapping from this module's verse IDs to the verse IDs
of a named numbering system. When a module has rows for a system, conversions to and from that system use
them; otherwise Xegesis falls back to `verse.Convert`, and then to the same number. Load a table at import
with `mapping_file`, a UTF-8 text file of tab-separated lines:

```
# system <TAB> verse in this module <TAB> same verse in the system (OSIS references)
english	Mal.3.19	Mal.4.1
```

### `verses_fts`

An FTS5 table over `verses.text` (external content, `rowid` = verse ID), tokenised with
`unicode61 remove_diacritics 2`. Search ranks by `bm25`.

## Markup

The `markup` column is XML-like text with a small fixed set of elements. Text is escaped (`&amp;`,
`&lt;`, `&gt;`, `&quot;`). Elements never span verses: an element that continues into the next verse is
closed at the end of one row and opened again at the start of the next.

| Element | Meaning | In `text`? |
| --- | --- | --- |
| `<h>…</h>` | Section heading before or in the verse. `k="psalm"` marks a psalm title | no |
| `<p/>` | A paragraph or poetry line starts here | — |
| `<j>…</j>` | Words of Christ | yes |
| `<w s="H7225 H853" m="…">…</w>` | Tagged word: Strong's numbers (`s`) and morphology code as given in the source (`m`) | yes |
| `<add>…</add>` | Words added by the translators | yes |
| `<f>…</f>` | Footnote | no |
| `<x>…</x>` | Cross-reference note | no |
| `<r o="Rom.5.8">…</r>` | A reference, with its OSIS target | yes, outside notes |

Other source markup (poetry groups, divine names, emphasis, tables) is kept as plain text. Markup that
cannot be placed is reported as import warnings, never as an error.

## Import mapping

| Source | Book and verse | Headings | Notes | Words of Christ | Strong's |
| --- | --- | --- | --- | --- | --- |
| OSIS | `div type="book"`, `verse osisID` (container or `sID`/`eID` milestones; several IDs = combined) | `title` (`runningHead` dropped) | `note`; `type="crossReference"` gives `<x>` | `q who="Jesus"` (container or milestones) | `w lemma="strong:…"`, `morph` |
| USFM | `\id`, `\c`, `\v 1` / `\v 1-2` | `\s#`, `\ms#`, `\mr`, `\r`, `\d`, `\sp`, `\qa`, `\cd` | `\f…\f*`, `\x…\x*` (origin `\fr`/`\xo` dropped) | `\wj` | `\w word\|strong="H7225" x-morph="…"\w*` |
| USX | `book code`, `chapter number`, `verse number` | `para` with heading styles | `note` | `char style="wj"` | `char style="w" strong=…` |

USFM headings are placed before the next verse. Introduction and title material (`\imt`, `\ip`, `\mt`,
`\toc`, …), alternate numbering (`\va`, `\vp`, `\ca`) and figures are skipped.

## Commands

`bible.modules`, `bible.install`, `bible.remove`, `bible.read`, `bible.parallel` and `bible.search`. None
of them writes `library.db`. `bible.parallel` and `bible.search` take references in a stated numbering
(`versification`, default `english`) and convert them for each module.
