-- +goose Up
ALTER TABLE notes ADD COLUMN suggested_tags TEXT;
ALTER TABLE notes ADD COLUMN ai_checked_hash TEXT;

-- +goose Down
-- SQLite does not remove added columns in a rollback; old code ignores them.
