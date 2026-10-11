-- +goose Up
-- B151: one-time links for AI clients (MCP) to put a file into the drive or take one out.
-- Only the SHA-256 of the token is stored. A link works once, for 10 minutes.
CREATE TABLE drive_upload_links (
    id INTEGER PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    parent_id INTEGER REFERENCES drive_items(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    on_conflict TEXT NOT NULL DEFAULT 'fail',
    max_size INTEGER NOT NULL,
    expires_at DATETIME NOT NULL,
    used_at DATETIME,
    created_by TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);
CREATE INDEX drive_upload_links_expires ON drive_upload_links(expires_at);

CREATE TABLE drive_download_links (
    id INTEGER PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    item_id INTEGER NOT NULL REFERENCES drive_items(id) ON DELETE CASCADE,
    expires_at DATETIME NOT NULL,
    used_at DATETIME,
    created_by TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);
CREATE INDEX drive_download_links_expires ON drive_download_links(expires_at);

-- +goose Down
SELECT 1;
