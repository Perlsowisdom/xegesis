-- library.db, schema version 1.
-- Current-state tables; every write to them is also recorded in `changes` (ADR 0004).
-- IDs are ULIDs stored as TEXT. Times are UTC milliseconds since the Unix epoch.
-- Verse IDs are integers BBCCCVVV (internal/verse).

PRAGMA foreign_keys = ON;

-- Devices and change history -------------------------------------------------

CREATE TABLE devices (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  INTEGER NOT NULL
);

CREATE TABLE changes (
    id           TEXT PRIMARY KEY,              -- ULID, sortable
    device_id    TEXT NOT NULL REFERENCES devices(id),
    hlc          TEXT NOT NULL,                 -- hybrid logical clock, orders changes across devices
    command      TEXT NOT NULL,                 -- command id that made the change
    object_type  TEXT NOT NULL,                 -- table name
    object_id    TEXT NOT NULL,
    op           TEXT NOT NULL CHECK (op IN ('create', 'update', 'delete')),
    field        TEXT,                          -- NULL for whole-object create or delete
    before_json  TEXT,                          -- previous value, or a reverse diff for long text
    after_json   TEXT,
    created_at   INTEGER NOT NULL
);
CREATE INDEX changes_object ON changes(object_type, object_id, hlc);
CREATE INDEX changes_hlc ON changes(hlc);

-- Library folders, items and files ------------------------------------------

CREATE TABLE library_folders (
    id            TEXT PRIMARY KEY,
    label         TEXT NOT NULL,
    volume_id     TEXT NOT NULL,                -- drive identity, survives a changed drive letter
    path_on_volume TEXT NOT NULL,               -- folder path relative to the volume root
    last_seen_at  INTEGER,
    deleted_at    INTEGER
);

CREATE TABLE items (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL,                  -- bible, commentary, monograph, dictionary, lexicon, ...
    title       TEXT NOT NULL,
    subtitle    TEXT,
    language    TEXT,                           -- BCP 47 tag
    year        INTEGER,
    publisher   TEXT,
    series      TEXT,
    series_no   TEXT,
    isbn        TEXT,
    source      TEXT NOT NULL,                  -- import, sword, stepbible, api_bible, dbl, own
    source_ref  TEXT,                           -- id at the source, if any
    module_id   TEXT,                           -- modules/<id>.db for Bibles and datasets
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    deleted_at  INTEGER
);
CREATE INDEX items_kind ON items(kind);
CREATE INDEX items_title ON items(title);

CREATE TABLE files (
    id          TEXT PRIMARY KEY,
    item_id     TEXT NOT NULL REFERENCES items(id),
    folder_id   TEXT NOT NULL REFERENCES library_folders(id),
    rel_path    TEXT NOT NULL,                  -- relative to the library folder
    format      TEXT NOT NULL,                  -- epub, pdf, docx, html, md, txt
    size_bytes  INTEGER,
    sha256      TEXT,
    indexed_at  INTEGER,
    deleted_at  INTEGER,
    UNIQUE (folder_id, rel_path)
);
CREATE INDEX files_item ON files(item_id);

CREATE TABLE people (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    sort_name  TEXT NOT NULL
);
CREATE INDEX people_sort ON people(sort_name);

CREATE TABLE item_people (
    item_id    TEXT NOT NULL REFERENCES items(id),
    person_id  TEXT NOT NULL REFERENCES people(id),
    role       TEXT NOT NULL DEFAULT 'author',  -- author, editor, translator
    position   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (item_id, person_id, role)
);

-- Licences --------------------------------------------------------------------

CREATE TABLE licences (
    id                 TEXT PRIMARY KEY,
    item_id            TEXT NOT NULL UNIQUE REFERENCES items(id),
    type               TEXT NOT NULL CHECK (type IN
                          ('public_domain', 'creative_commons', 'open_other',
                           'personal_purchase', 'own_work', 'unknown')),
    spdx_or_cc         TEXT,                    -- e.g. CC-BY-4.0
    rights_holder      TEXT,
    licence_url        TEXT,
    attribution_text   TEXT,
    print_limit        TEXT,                    -- JSON: {"pages":N} or {"percent":N}; NULL = unlimited
    export_limit       TEXT,                    -- same shape as print_limit
    commercial_allowed INTEGER NOT NULL DEFAULT 0,
    share_allowed      INTEGER NOT NULL DEFAULT 0,
    store_allowed      INTEGER NOT NULL DEFAULT 1,
    confirmed_by_user  INTEGER NOT NULL DEFAULT 0,
    updated_at         INTEGER NOT NULL
);

-- Groupings and collections --------------------------------------------------

CREATE TABLE item_facets (
    item_id  TEXT NOT NULL REFERENCES items(id),
    facet    TEXT NOT NULL,                     -- type, focus, tradition, era, ...
    value    TEXT NOT NULL,
    origin   TEXT NOT NULL CHECK (origin IN ('metadata', 'bisac', 'user')),
    PRIMARY KEY (item_id, facet, value)
);
CREATE INDEX item_facets_value ON item_facets(facet, value);

CREATE TABLE collections (
    id          TEXT PRIMARY KEY,
    parent_id   TEXT REFERENCES collections(id),
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('manual', 'rule')),
    rule_json   TEXT,                           -- for kind = 'rule'
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    deleted_at  INTEGER
);

CREATE TABLE collection_items (
    collection_id  TEXT NOT NULL REFERENCES collections(id),
    item_id        TEXT NOT NULL REFERENCES items(id),
    position       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (collection_id, item_id)
);

-- Anchors, notes, highlights and lists ---------------------------------------

CREATE TABLE anchors (
    id            TEXT PRIMARY KEY,
    item_id       TEXT REFERENCES items(id),    -- NULL for a verse-only anchor
    verse_start   INTEGER,                      -- BBCCCVVV
    verse_end     INTEGER,
    versification TEXT,                         -- numbering tradition of the verse IDs
    locator_json  TEXT,                         -- {"cfi":...} for EPUB, {"page":N,"rect":[...]} for PDF
    CHECK (item_id IS NOT NULL OR verse_start IS NOT NULL)
);
CREATE INDEX anchors_verse ON anchors(verse_start, verse_end);
CREATE INDEX anchors_item ON anchors(item_id);

CREATE TABLE notes (
    id          TEXT PRIMARY KEY,
    anchor_id   TEXT REFERENCES anchors(id),    -- NULL for a free-standing note
    title       TEXT,
    body_md     TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    deleted_at  INTEGER
);
CREATE INDEX notes_anchor ON notes(anchor_id);

CREATE TABLE highlights (
    id          TEXT PRIMARY KEY,
    anchor_id   TEXT NOT NULL REFERENCES anchors(id),
    style       TEXT NOT NULL DEFAULT 'yellow',
    created_at  INTEGER NOT NULL,
    deleted_at  INTEGER
);

CREATE TABLE lists (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL CHECK (kind IN ('verses', 'words', 'excerpts')),
    name        TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    deleted_at  INTEGER
);

CREATE TABLE list_entries (
    id          TEXT PRIMARY KEY,
    list_id     TEXT NOT NULL REFERENCES lists(id),
    anchor_id   TEXT REFERENCES anchors(id),    -- verses and excerpts
    word        TEXT,                           -- words: surface form or lemma
    language    TEXT,
    note        TEXT,
    position    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX list_entries_list ON list_entries(list_id, position);

-- Reading plans and documents ------------------------------------------------

CREATE TABLE reading_plans (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    start_date  TEXT,                           -- ISO date
    created_at  INTEGER NOT NULL,
    deleted_at  INTEGER
);

CREATE TABLE plan_days (
    plan_id     TEXT NOT NULL REFERENCES reading_plans(id),
    day_no      INTEGER NOT NULL,
    verse_start INTEGER,
    verse_end   INTEGER,
    item_id     TEXT REFERENCES items(id),
    locator_json TEXT,
    PRIMARY KEY (plan_id, day_no, verse_start)
);

CREATE TABLE plan_progress (
    plan_id      TEXT NOT NULL REFERENCES reading_plans(id),
    day_no       INTEGER NOT NULL,
    completed_at INTEGER NOT NULL,
    PRIMARY KEY (plan_id, day_no)
);

CREATE TABLE documents (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL DEFAULT 'sermon', -- sermon, lesson, study
    title        TEXT NOT NULL,
    body_md      TEXT NOT NULL DEFAULT '',
    outline_json TEXT,
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    deleted_at   INTEGER
);

-- Settings --------------------------------------------------------------------

CREATE TABLE settings (
    key         TEXT PRIMARY KEY,
    value_json  TEXT NOT NULL
);

PRAGMA user_version = 1;
