-- library.db, schema version 3.
-- licences.origin records where an item's licence came from, so the interface can say so:
-- 'file' (the book's own details named it), 'default' (nothing was stated, so it is a personal
-- copy) or 'user' (the user chose it). Rows before this version were all chosen by the user.

ALTER TABLE licences ADD COLUMN origin TEXT NOT NULL DEFAULT 'user'
    CHECK (origin IN ('file', 'default', 'user'));
