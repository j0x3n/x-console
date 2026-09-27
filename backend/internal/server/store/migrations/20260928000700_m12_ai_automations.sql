-- +goose Up
CREATE TABLE ai_conversations (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE TABLE ai_messages (
    id INTEGER PRIMARY KEY,
    conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('user','assistant')),
    content TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    UNIQUE(conversation_id,seq)
);
CREATE TABLE ai_pending_actions (
    id INTEGER PRIMARY KEY,
    conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    tool_use_id TEXT NOT NULL,
    action TEXT NOT NULL,
    input TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected','done','failed')),
    result TEXT,
    UNIQUE(conversation_id,tool_use_id)
);
CREATE INDEX ai_messages_conversation ON ai_messages(conversation_id,seq);
CREATE INDEX ai_pending_conversation ON ai_pending_actions(conversation_id,status);
CREATE TABLE automations (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    trigger TEXT NOT NULL,
    conditions TEXT NOT NULL,
    actions TEXT NOT NULL,
    cooldown_seconds INTEGER NOT NULL DEFAULT 0,
    authorized INTEGER NOT NULL DEFAULT 0,
    webhook_token_hash TEXT,
    webhook_token_enc TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX automations_webhook_token ON automations(webhook_token_hash) WHERE webhook_token_hash IS NOT NULL;
CREATE TABLE automation_runs (
    id INTEGER PRIMARY KEY,
    automation_id INTEGER NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
    started_at DATETIME NOT NULL,
    finished_at DATETIME,
    trigger_data TEXT NOT NULL,
    steps TEXT NOT NULL DEFAULT '[]',
    status TEXT NOT NULL CHECK(status IN ('running','ok','failed','skipped'))
);
CREATE INDEX automation_runs_automation ON automation_runs(automation_id,started_at DESC);

-- +goose Down
