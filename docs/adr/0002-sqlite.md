# 0002: SQLite through modernc.org/sqlite

## Decision

All databases are SQLite files, accessed through `modernc.org/sqlite` (pure Go, BSD-3-Clause).

- `library.db`: user data and change history. Always on a local internal drive.
- `index/<folder>.db`: one library folder's search index (FTS5). Rebuildable.
- `modules/<id>.db`: one read-only Bible or dataset. Rebuildable.

## Reasons

- SQLite files open in any tool (DB Browser, Python, Datasette), which keeps user data readable.
- FTS5 provides full-text search.
- The pure-Go build needs no C compiler, so cross-platform builds stay simple.

## Consequences

- One writer at a time: all writes go through the command layer.
- Databases must never live in cloud-synced or network folders. Sync exchanges change records instead.
- The pure-Go build is somewhat slower than C SQLite; a C build can replace it without schema changes.
- any-store (MIT) was considered but uses its own on-disk format that other tools cannot read.
