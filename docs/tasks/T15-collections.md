# T15: Collections

Brief for one agent (Sonnet or Opus), after T28. Read `AGENTS.md` first, especially the design
philosophy: simple things simple, complex things possible.

## Idea

Collections group books so you can browse, filter and search a part of your library.

- **Built-in groupings appear by themselves, with no setting up:**
  - **Type**, from T28: Bibles, Commentaries (Old Testament), (New Testament) and (whole Bible), and so
    on;
  - **Author**;
  - **Tradition**, from T28 and your tradition rules;
  - **Year**, grouped by century and then by decade.

  They are derived from book details, so they stay current as books are added or changed.
- **Your own collections** come in two kinds:
  - **Hand-picked:** add books by selecting them in the book list, and choose "Add to collection"
    or "New collection from selected";
  - **Rule-based:** "books where type is Commentary (New Testament) and tradition is Reformed". You build
    the rule with plain dropdowns (type, tradition, author, year range, folder, licence, language, text
    in title). Raw rule JSON is for the CLI only.

  Collections can be nested (`parent_id`), renamed, reordered and deleted (soft delete with history, as
  everywhere).

## What to build

1. **Commands:**
   - `collection.list` returns the built-in groupings and your collections as one tree, with book counts;
   - `collection.items` lists a collection's books, paged, using the same item shape as `library.items`;
   - `collection.create`, `collection.update`, `collection.delete`;
   - `collection.add` and `collection.remove` add and remove books in hand-picked collections;
   - `collection.preview` shows how many books a rule matches, and examples, before saving.

   Use the existing `collections` and `collection_items` tables. Store rules in `rule_json` with a
   documented, versioned shape (`docs/collections.md`). Built-in groupings need no rows; they are
   computed.
2. **Filters elsewhere:** `library.items` and `search.query` accept a `collection_id`, so a search can be
   limited to one collection, for example "search only my New Testament commentaries".
3. **Interface:**
   - The Library gets a collections sidebar (a tree with counts). Choosing an entry filters the book list.
   - "New collection" offers "From selected books" or "By rule". The rule builder shows a live count.
   - The Search section gets a "Search in" collection picker.
   - Plain words throughout, and no command names.
4. **Speed:** collection counts and rule matching must stay quick at 10,000 books. Use the scale library
   from T12 (`internal/scaletest`) or a smaller generated one, and keep counts under about 200 ms.

## Rules

- Work only in your worktree.
- You may change `internal/library` (collections may live in a new `internal/collections` package),
  `internal/search` (the collection filter only), `frontend/src`, and add one migration if needed. Update
  the schema-version tests if you do.
- Do not commit `frontend/dist`.
- Use a scratch library (`XEGESIS_LIBRARY`) for manual checks.

## Done when

- A fresh library with typed books shows the Type, Author, Tradition and Year groupings with correct
  counts, without any setting up.
- A hand-picked collection and a rule-based collection can each be made, nested, renamed and deleted
  (and restored from history).
- Book list and search filter by any collection.
- Counts stay under about 200 ms at 10,000 books.
- `gofmt`, `go vet`, `go test`, the frontend tests and build, and the desktop build pass.
