-- +goose Up
ALTER TABLE quota_accounts ADD COLUMN balance_low TEXT NOT NULL DEFAULT '';
ALTER TABLE quota_readings ADD COLUMN fail_count INTEGER NOT NULL DEFAULT 0;
CREATE TABLE quota_notify_state (
 account_id INTEGER NOT NULL REFERENCES quota_accounts(id) ON DELETE CASCADE,
 window_name TEXT NOT NULL,
 event TEXT NOT NULL,
 period_end TEXT NOT NULL DEFAULT '',
 sent_at DATETIME NOT NULL,
 PRIMARY KEY (account_id, window_name, event)
);

-- +goose Down
SELECT 1;
