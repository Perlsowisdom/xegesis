-- library.db, schema version 9 (8 is reports, T13).
-- copy_log now also counts prints and exports (T18), and Bibles as well as library books, so each
-- item's licence limits apply to everything taken from it in a day:
--   action  export (copies, list excerpts and exported files) or print, each with its own limit;
--   module  the Bible module the text came from, when it is not a library book.

CREATE TABLE copy_log_new (
    id          TEXT PRIMARY KEY,
    item_id     TEXT REFERENCES items(id),      -- a library book, or
    module      TEXT,                           -- a Bible module
    action      TEXT NOT NULL DEFAULT 'export' CHECK (action IN ('export', 'print')),
    day         TEXT NOT NULL,                  -- local date, YYYY-MM-DD
    chars       INTEGER NOT NULL,               -- characters taken, before any credit line
    created_at  INTEGER NOT NULL,
    CHECK ((item_id IS NULL) <> (module IS NULL))
);
INSERT INTO copy_log_new (id, item_id, action, day, chars, created_at)
    SELECT id, item_id, 'export', day, chars, created_at FROM copy_log;
DROP TABLE copy_log;
ALTER TABLE copy_log_new RENAME TO copy_log;
CREATE INDEX copy_log_item_day ON copy_log(item_id, day);
CREATE INDEX copy_log_module_day ON copy_log(module, day);

PRAGMA user_version = 9;
