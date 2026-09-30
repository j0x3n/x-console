-- +goose Up
CREATE TABLE drive_file_versions (
    id         INTEGER PRIMARY KEY,
    item_id    INTEGER NOT NULL REFERENCES drive_items (id) ON DELETE CASCADE,
    size       INTEGER NOT NULL,
    sha256     TEXT    NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX drive_file_versions_item ON drive_file_versions (item_id, created_at DESC);
CREATE INDEX drive_file_versions_hash ON drive_file_versions (sha256);

CREATE TABLE drive_shares (
    id             INTEGER PRIMARY KEY,
    item_id        INTEGER NOT NULL REFERENCES drive_items (id) ON DELETE CASCADE,
    token          TEXT    NOT NULL UNIQUE,
    code_sealed    TEXT,
    expires_at     DATETIME,
    max_downloads  INTEGER,
    visits         INTEGER NOT NULL DEFAULT 0,
    downloads      INTEGER NOT NULL DEFAULT 0,
    created_at     DATETIME NOT NULL,
    last_access_at DATETIME
);
CREATE INDEX drive_shares_item ON drive_shares (item_id);

-- +goose Down
DROP TABLE drive_shares;
DROP TABLE drive_file_versions;
