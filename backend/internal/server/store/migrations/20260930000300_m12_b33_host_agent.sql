-- +goose Up
ALTER TABLE ai_conversations ADD COLUMN host_id TEXT;
ALTER TABLE ai_pending_actions ADD COLUMN effect TEXT;
CREATE INDEX ai_conversations_host ON ai_conversations(host_id, updated_at DESC) WHERE host_id IS NOT NULL;

-- +goose Down
