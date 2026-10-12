-- +goose Up
ALTER TABLE ai_conversations ADD COLUMN host_permission TEXT NOT NULL DEFAULT 'confirm' CHECK (host_permission IN ('confirm', 'read_auto', 'all_auto'));
ALTER TABLE ai_conversations ADD COLUMN host_permission_until DATETIME DEFAULT NULL;

-- +goose Down
SELECT 1;
