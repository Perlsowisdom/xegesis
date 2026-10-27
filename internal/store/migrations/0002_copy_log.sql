-- library.db, schema version 2.
-- copy_log records how much text was copied from each book on each day, so a licence's export
-- limit can apply to the day's copies together as well as to each copy (book.copy).

CREATE TABLE copy_log (
    id          TEXT PRIMARY KEY,
    item_id     TEXT NOT NULL REFERENCES items(id),
    day         TEXT NOT NULL,                  -- local date of the copy, YYYY-MM-DD
    chars       INTEGER NOT NULL,               -- characters copied, before the attribution line
    created_at  INTEGER NOT NULL
);
CREATE INDEX copy_log_item_day ON copy_log(item_id, day);
