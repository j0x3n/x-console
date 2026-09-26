-- +goose Up
CREATE TABLE ai_conversations (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE ai_messages (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    UNIQUE (conversation_id, seq)
);
CREATE INDEX ai_messages_conversation ON ai_messages(conversation_id, seq);

CREATE TABLE ai_pending_actions (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    tool_use_id TEXT NOT NULL,
    action TEXT NOT NULL,
    input TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'done', 'failed')),
    result TEXT,
    created_at DATETIME NOT NULL,
    UNIQUE (conversation_id, tool_use_id)
);
CREATE INDEX ai_pending_actions_conversation ON ai_pending_actions(conversation_id, status);

-- +goose Down
DROP TABLE ai_pending_actions;
DROP TABLE ai_messages;
DROP TABLE ai_conversations;
