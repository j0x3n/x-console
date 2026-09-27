-- +goose Up
CREATE TABLE drive_items (
    id INTEGER PRIMARY KEY,
    parent_id INTEGER REFERENCES drive_items(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    is_dir INTEGER NOT NULL DEFAULT 0,
    size INTEGER NOT NULL DEFAULT 0,
    mime TEXT NOT NULL DEFAULT '',
    sha256 TEXT NOT NULL DEFAULT '',
    hidden INTEGER NOT NULL DEFAULT 0,
    hidden_from INTEGER,
    trashed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    s3_synced_at DATETIME,
    s3_etag TEXT,
    s3_error TEXT,
    s3_key TEXT,
    s3_hash TEXT,
    CHECK (name <> ''),
    CHECK (is_dir IN (0, 1)),
    CHECK (hidden IN (0, 1))
);
CREATE UNIQUE INDEX drive_items_active_name ON drive_items (COALESCE(parent_id, 0), hidden, name) WHERE trashed_at IS NULL;
CREATE INDEX drive_items_parent ON drive_items(parent_id, hidden, trashed_at);
CREATE INDEX drive_items_hash ON drive_items(sha256) WHERE is_dir = 0;
CREATE INDEX drive_items_trash ON drive_items(trashed_at);
CREATE TABLE drive_s3_deletions (
    key TEXT PRIMARY KEY,
    created_at DATETIME NOT NULL
);

-- +goose Down
