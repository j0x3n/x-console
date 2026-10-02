-- +goose Up
ALTER TABLE reminders ADD COLUMN icon TEXT NOT NULL DEFAULT '';

-- +goose Down
SELECT 1;
