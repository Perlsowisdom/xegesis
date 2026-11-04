-- library.db, schema version 10 (8 was never used).
-- Folders for the user's own writing: a folder holds notes and documents (sermons, lessons and
-- studies) together, and can sit inside one other folder. Sermons gain the date they were preached
-- and a theme.

CREATE TABLE folders (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    parent_id   TEXT REFERENCES folders(id),    -- NULL at the top; folders nest one level
    sort        INTEGER NOT NULL DEFAULT 0,     -- order among its siblings
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    deleted_at  INTEGER
);
CREATE INDEX folders_parent ON folders(parent_id, sort);

ALTER TABLE notes ADD COLUMN folder_id TEXT REFERENCES folders(id);
ALTER TABLE documents ADD COLUMN folder_id TEXT REFERENCES folders(id);
ALTER TABLE documents ADD COLUMN preached_on TEXT;  -- ISO date, YYYY-MM-DD
ALTER TABLE documents ADD COLUMN theme TEXT;
CREATE INDEX notes_folder ON notes(folder_id);
CREATE INDEX documents_folder ON documents(folder_id);

PRAGMA user_version = 10;
