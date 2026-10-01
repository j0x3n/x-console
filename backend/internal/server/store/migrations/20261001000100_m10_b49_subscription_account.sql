-- +goose Up
ALTER TABLE subscriptions ADD COLUMN account TEXT NOT NULL DEFAULT '';

-- +goose Down
-- 迁移只加不删，Down 留空
