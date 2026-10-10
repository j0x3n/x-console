-- +goose Up
-- 密钥可以直接存：secret_enc 是用 Secrets 加密后的密钥内容，空字符串表示没存。
ALTER TABLE credentials ADD COLUMN secret_enc TEXT NOT NULL DEFAULT '';

-- +goose Down
SELECT 1;
