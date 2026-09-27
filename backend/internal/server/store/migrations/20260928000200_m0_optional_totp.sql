-- +goose Up
ALTER TABLE users ADD COLUMN setup_completed INTEGER NOT NULL DEFAULT 1;
UPDATE users SET setup_completed = 0 WHERE totp_enabled = 0 AND totp_secret <> '';

-- +goose Down
