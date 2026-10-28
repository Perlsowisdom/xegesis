-- library.db, schema version 4.
-- item_facets rows set automatically (origin 'metadata' or 'bisac') now say why and how sure the
-- guess was, so the interface can show the reason. Rows set by the user have neither.
-- facet_rules holds the user's tradition rules: "books by this author, publisher or series, or in
-- this folder, belong to this tradition".

ALTER TABLE item_facets ADD COLUMN reason TEXT;
ALTER TABLE item_facets ADD COLUMN confidence REAL;

CREATE TABLE facet_rules (
    id          TEXT PRIMARY KEY,
    facet       TEXT NOT NULL CHECK (facet IN ('tradition')),
    match_field TEXT NOT NULL CHECK (match_field IN ('author', 'publisher', 'series', 'folder')),
    match_text  TEXT NOT NULL,
    value       TEXT NOT NULL,
    created_at  INTEGER NOT NULL
);
