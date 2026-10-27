# 0004: Every write is a logged command

## Decision

Every change to `library.db` is made by a command and recorded in the append-only `changes` table:
a sortable ID, the device, a hybrid logical clock timestamp, the command, the object and field changed,
and the values before and after. The other tables hold the current state.

## Consequences

- Any earlier version of a note, highlight or collection can be restored by applying history in reverse.
- Sync (ADR 0005) exchanges change records between devices rather than copying databases.
- Deletions are recorded as changes, so they also reach other devices.
- History grows; a compaction command condenses changes older than a set period into snapshots.
- Long text fields store differences rather than full copies.
