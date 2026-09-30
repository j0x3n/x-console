-- +goose Up
ALTER TABLE ai_providers ADD COLUMN api_style TEXT NOT NULL DEFAULT 'chat' CHECK (api_style IN ('chat', 'responses'));
CREATE TABLE ai_attachments (
    id INTEGER PRIMARY KEY,
    conversation_id INTEGER REFERENCES ai_conversations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    mime TEXT NOT NULL,
    size INTEGER NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('image', 'text')),
    created_at DATETIME NOT NULL
);
CREATE INDEX ai_attachments_conversation ON ai_attachments(conversation_id);
CREATE INDEX ai_attachments_unsent ON ai_attachments(created_at) WHERE conversation_id IS NULL;

-- +goose Down
-- 只加不删。
SELECT 1;
