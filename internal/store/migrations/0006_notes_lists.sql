-- library.db, schema version 6.
-- Note tags, and the extra details word and excerpt lists keep with each entry.

CREATE TABLE note_tags (
    note_id  TEXT NOT NULL REFERENCES notes(id),
    tag      TEXT NOT NULL,                     -- lower case, single spaces
    PRIMARY KEY (note_id, tag)
);
CREATE INDEX note_tags_by_tag ON note_tags(tag, note_id);

ALTER TABLE list_entries ADD COLUMN strong TEXT;       -- words: Strong's number, e.g. G26
ALTER TABLE list_entries ADD COLUMN lemma TEXT;        -- words: dictionary form
ALTER TABLE list_entries ADD COLUMN attribution TEXT;  -- excerpts: the credit that was copied with the text

CREATE INDEX notes_updated ON notes(updated_at);
