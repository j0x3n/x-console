-- +goose Up
-- B243: 分享链接可以允许编辑。allow_edit 为 1 时，打开链接的人能改标题和正文，默认 0 和以前一样只能看。
ALTER TABLE note_shares ADD COLUMN allow_edit INTEGER NOT NULL DEFAULT 0;

-- +goose Down
SELECT 1;
