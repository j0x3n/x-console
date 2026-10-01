-- +goose Up
ALTER TABLE projects ADD COLUMN layout_locked INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- 迁移只加不删，Down 留空
