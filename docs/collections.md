# Collections

Collections group books so you can browse, filter and search part of your library. Commands:
`collection.list`, `collection.items`, `collection.create`, `collection.update`, `collection.delete`,
`collection.add`, `collection.remove`, `collection.preview`. `library.items`, `search.query` and
`search.refs` also take a `collection_id`.

## Built-in groupings

They have no rows; they are computed from book details each time, so they stay current as books are
added or changed. Their IDs start with `b:`.

| ID | Books |
| --- | --- |
| `b:all` | Every book |
| `b:type`, `b:type:<type>`, `b:type:none` | Books with a type; with that type; with no type yet |
| `b:tradition`, `b:tradition:<name>`, `b:tradition:none` | The same for tradition |
| `b:year`, `b:year:c16`, `b:year:d1560` | Books with a year; in the 16th century (1500 to 1599); in the 1560s |
| `b:author`, `b:author:<name>` | Books with a credited author; by the author with exactly that name |

`collection.list` returns them with your own collections as one tree. Authors are listed under
Author only when `authors` is true, as there can be thousands. Types and traditions with no books are
left out. A book has one type and one tradition, so the counts under Type add up to the Type count.

## Your collections

Stored in `collections` (`parent_id` nests them, `position` orders them, `deleted_at` is a soft
delete) and `collection_items`.

- **Hand-picked** (`kind` = `manual`): the books are rows in `collection_items`.
  `collection.add` and `collection.remove` change them; `collection.create` can start with `item_ids`.
- **By rule** (`kind` = `rule`): the books are those matching the rule in `rule_json`, worked out
  when asked, so the collection follows the library. A rule collection cannot have books added.

A collection shows only its own books, not those of the collections inside it. Deleting a collection
deletes the collections inside it too (soft, one change each). `history.restore` with object type
`collections` brings each back; `collection_items` rows are kept.
`collection.update` renames, moves (`parent_id`, empty for the top level), reorders (`move` up or
down among its neighbours) and, for a rule collection, replaces the rule.

## Rule format, version 1

`rule_json` holds one JSON object. A book matches when it meets **every** condition given. A
condition left out matches any book, and a rule needs at least one.

```json
{
  "version": 1,
  "type": "Commentary (New Testament)",
  "tradition": "Reformed",
  "author": "calvin",
  "text": "romans",
  "language": "en",
  "year_from": 1500,
  "year_to": 1700,
  "folder_id": "01J...",
  "licence": "public_domain"
}
```

| Field | Matches |
| --- | --- |
| `type` | The book type (see `facet.vocabulary`), or `none` for no type yet |
| `tradition` | The tradition, or `none` for none yet |
| `author` | Text in a credited person's name, ignoring capitals |
| `text` | Text in the title or subtitle |
| `language` | The language code, including its regional forms (`en` also matches `en-GB`) |
| `year_from`, `year_to` | Published in or after, and in or before (books with no year never match a year limit) |
| `folder_id` | A file in that library folder |
| `licence` | The licence type: `public_domain`, `creative_commons`, `open_other`, `personal_purchase`, `own_work` or `unknown` |

`version` may be left out when writing (it is stored as 1). A rule with a higher version is refused.
`collection.preview` takes a rule and returns the number of books it matches and some examples,
without saving anything.
