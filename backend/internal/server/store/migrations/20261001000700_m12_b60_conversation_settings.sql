-- +goose Up
ALTER TABLE ai_conversations ADD COLUMN permission TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE ai_conversations ADD COLUMN model TEXT NOT NULL DEFAULT '';
ALTER TABLE ai_conversations ADD COLUMN effort TEXT NOT NULL DEFAULT '';

-- +goose Down
-- 迁移只加不删，Down 留空
