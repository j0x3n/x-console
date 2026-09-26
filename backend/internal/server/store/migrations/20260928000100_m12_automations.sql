-- +goose Up
CREATE TABLE automations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    trigger TEXT NOT NULL,
    conditions TEXT NOT NULL,
    actions TEXT NOT NULL,
    cooldown_seconds INTEGER NOT NULL DEFAULT 0,
    dangerous_authorized INTEGER NOT NULL DEFAULT 0,
    webhook_token_hash TEXT,
    last_run_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX automations_webhook_token ON automations(webhook_token_hash) WHERE webhook_token_hash IS NOT NULL;

CREATE TABLE automation_runs (
    id TEXT PRIMARY KEY,
    automation_id TEXT NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
    started_at DATETIME NOT NULL,
    finished_at DATETIME,
    trigger_data TEXT NOT NULL,
    steps TEXT NOT NULL,
    status TEXT NOT NULL
);
CREATE INDEX automation_runs_rule ON automation_runs(automation_id, started_at DESC);

-- +goose Down
DROP TABLE automation_runs;
DROP TABLE automations;
