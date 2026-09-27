-- +goose Up
ALTER TABLE sessions ADD COLUMN vault_until DATETIME;

-- +goose Down
