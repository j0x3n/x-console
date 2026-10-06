-- +goose Up
CREATE TABLE notification_mutes (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 kind_pattern TEXT NOT NULL,
 scope TEXT NOT NULL DEFAULT '',
 target TEXT NOT NULL,
 created_at DATETIME NOT NULL,
 UNIQUE (kind_pattern, scope, target)
);

-- +goose Down
SELECT 1;
