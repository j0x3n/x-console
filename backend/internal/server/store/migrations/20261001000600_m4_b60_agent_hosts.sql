-- +goose Up
ALTER TABLE ai_agents ADD COLUMN host_ids TEXT NOT NULL DEFAULT '[]';

-- +goose Down
-- 迁移只加不删，Down 留空
