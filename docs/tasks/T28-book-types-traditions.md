# T28: Book types and traditions

Brief for one agent (Sonnet), before T15 (collections). Read `AGENTS.md` first.

The default collections group books by type (monograph, Bible, commentary on OT or NT, dictionary and
so on), author, tradition (Catholic, Anglican, Baptist and so on) and year. Imported books are all
`kind = book` today, and their files rarely state a type or tradition.

## What to build

1. **A type vocabulary** (documented in `docs/facets.md`) and a classifier that suggests a type from
   the file's metadata: subjects, title words ("Commentary on Romans"), series names and folder
   names. Each suggestion carries a confidence and its reason, and is stored in `item_facets` with
   origin `metadata`. A user's choice (origin `user`) always wins and is never overwritten.
2. **Traditions**: an editable list, set by the user or by rules ("books by this author or publisher,
   or in this folder, are Reformed"). Do not embed the BISAC list, which needs a paid licence; map the
   few BISAC codes found in files where the mapping is factual.
3. **Commands**: `facet.suggest`, `facet.set` (bulk), `facet.rules` (list, add, remove, apply).
4. **Interface**: a review screen to accept or correct suggestions in bulk after a scan.

## Done when

A scan of mixed fixtures gets sensible type suggestions with reasons, bulk corrections stick across
rescans, and a tradition rule tags matching books.
