-- +goose Up
ALTER TABLE github_pulls ADD COLUMN state TEXT NOT NULL DEFAULT 'open';

-- +goose Down
-- SQLite 无法安全移除已有列。保留 state 不影响旧版本读取。
