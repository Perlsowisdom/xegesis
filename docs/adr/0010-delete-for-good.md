# 0010: Deleting for good erases values from the history

Status: Accepted (approved by the user, 3 November 2026; T34c). Extends 0004.

## Context

ADR 0004 makes the `changes` table append-only, and every delete is soft, so anything can be restored.
The bin (T34c) also offers "Delete for good". If that only removed the current row, the note's text would
still be readable from its earlier changes, so it would not really be gone.

## Decision

- `store.Tx.Purge` removes a row for good, even from a table with `deleted_at`.
- The row's earlier change records stay, in order, so history and sync still know the object existed and
  was removed. Their `before_json` and `after_json` are set to NULL: the values are erased.
- The purge is logged as an ordinary `delete` change whose `field` is `purge`, with no values, so the
  `op` check on `changes` is unchanged and no migration of the history is needed.
- Only things already in the bin can be deleted for good (`bin.purge`), and the interface asks first.

## Consequences

- Erasing values is the one edit allowed to an existing change record. The record's identity, time,
  device, command and object stay.
- Restoring a purged object from its history brings nothing back.
- A sync add-on (ADR 0005) must apply a `delete`/`purge` change by removing the row and erasing its values
  locally too.
