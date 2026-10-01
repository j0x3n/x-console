-- +goose Up
ALTER TABLE monitors ADD COLUMN manual_expires_at DATE;
ALTER TABLE monitors ADD COLUMN expiry_source TEXT NOT NULL DEFAULT '';

-- +goose Down
-- 迁移只加不删，Down 留空
