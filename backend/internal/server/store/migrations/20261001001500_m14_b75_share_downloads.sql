-- +goose Up
CREATE TABLE drive_share_downloads (
    id INTEGER PRIMARY KEY,
    share_id INTEGER NOT NULL REFERENCES drive_shares(id) ON DELETE CASCADE,
    at DATETIME NOT NULL,
    ip TEXT NOT NULL,
    user_agent TEXT NOT NULL,
    item_id INTEGER REFERENCES drive_items(id) ON DELETE SET NULL,
    item_name TEXT NOT NULL DEFAULT ''
);
CREATE INDEX drive_share_downloads_recent ON drive_share_downloads(share_id,at DESC,id DESC);

-- +goose Down
DROP TABLE drive_share_downloads;
