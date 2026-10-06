-- +goose Up
CREATE TABLE quota_accounts (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 kind TEXT NOT NULL CHECK (kind IN ('claude','codex','grok','deepseek')),
 name TEXT NOT NULL,
 host_id TEXT NOT NULL DEFAULT '',
 home TEXT NOT NULL DEFAULT '',
 api_key TEXT NOT NULL DEFAULT '',
 key_hash TEXT NOT NULL DEFAULT '',
 sort_order INTEGER NOT NULL DEFAULT 0,
 created_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX quota_accounts_identity ON quota_accounts(kind, host_id, home, key_hash);

CREATE TABLE quota_readings (
 account_id INTEGER PRIMARY KEY REFERENCES quota_accounts(id) ON DELETE CASCADE,
 ok INTEGER NOT NULL DEFAULT 0,
 error TEXT NOT NULL DEFAULT '',
 error_code TEXT NOT NULL DEFAULT '',
 plan TEXT NOT NULL DEFAULT '',
 user TEXT NOT NULL DEFAULT '',
 credits TEXT NOT NULL DEFAULT '',
 balances_json TEXT NOT NULL DEFAULT '[]',
 windows_json TEXT NOT NULL DEFAULT '[]',
 read_at DATETIME,
 tried_at DATETIME NOT NULL
);

-- +goose Down
SELECT 1;
