-- Initial schema. See DESIGN.md §5.
-- Timestamps are stored as unix seconds (INTEGER) to keep scanning
-- driver-agnostic. `online` is maintained by the drive monitor.

CREATE TABLE drives (
    id              INTEGER PRIMARY KEY,
    label           TEXT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('source','destination','both')),
    marker_id       TEXT UNIQUE,                 -- destination identity (.archivarr-drive-id)
    root_path       TEXT,                        -- source identity (configured, stable)
    fs_uuid         TEXT,                        -- metadata only, when resolvable
    last_mount_path TEXT,                         -- where it was last seen
    capacity_bytes  INTEGER,
    free_bytes      INTEGER,
    online          INTEGER NOT NULL DEFAULT 0,   -- maintained by the monitor
    last_seen_at    INTEGER,
    notes           TEXT,
    created_at      INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE TABLE media_items (
    id              INTEGER PRIMARY KEY,
    source_drive_id INTEGER REFERENCES drives(id) ON DELETE SET NULL,
    rel_path        TEXT NOT NULL,
    size            INTEGER NOT NULL,
    mtime           INTEGER NOT NULL,
    content_hash    TEXT,
    hash_algo       TEXT DEFAULT 'xxh3',
    present         INTEGER NOT NULL DEFAULT 1,    -- 0 if gone at source (stale)
    last_scanned_at INTEGER,
    UNIQUE(source_drive_id, rel_path)
);
CREATE INDEX idx_media_hash    ON media_items(content_hash);
CREATE INDEX idx_media_present ON media_items(present);

CREATE TABLE backups (
    id            INTEGER PRIMARY KEY,
    media_item_id INTEGER NOT NULL REFERENCES media_items(id),
    dest_drive_id INTEGER NOT NULL REFERENCES drives(id) ON DELETE CASCADE,
    dest_rel_path TEXT NOT NULL,
    size          INTEGER NOT NULL,
    copied_at     INTEGER NOT NULL,
    verified_at   INTEGER,
    verify_hash   TEXT,
    status        TEXT NOT NULL CHECK (status IN ('ok','unverified','stale','missing','failed')),
    UNIQUE(media_item_id, dest_drive_id)
);
CREATE INDEX idx_backups_dest ON backups(dest_drive_id);
CREATE INDEX idx_backups_item ON backups(media_item_id);

CREATE TABLE jobs (
    id          INTEGER PRIMARY KEY,
    type        TEXT NOT NULL CHECK (type IN ('scan','backup','verify','prune','import')),
    status      TEXT NOT NULL CHECK (status IN ('queued','running','done','failed','cancelled')),
    params_json TEXT,
    progress    REAL NOT NULL DEFAULT 0,
    stats_json  TEXT,
    log         TEXT,
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    started_at  INTEGER,
    finished_at INTEGER
);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT
);
