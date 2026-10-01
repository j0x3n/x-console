-- B47 构建失败的原因。

-- +goose Up
ALTER TABLE coding_tasks ADD COLUMN build_error TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE coding_tasks DROP COLUMN build_error;
