# 0006: One search index per library folder

## Decision

Books may live in any number of library folders, internal or external. Each folder has its own index
file, `index/<folder>.db`, stored on the internal drive. Every search queries all indexes and merges the
ranked results.

## Consequences

- Books on an unplugged external drive still appear in search results, marked as offline.
- Rebuilding one folder's index leaves the others alone.
- Folders are identified by drive volume ID, not drive letter, so a changed letter breaks nothing.
- Indexing runs in the background, resumes after a restart, and makes books searchable one by one.
