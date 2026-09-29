-- B34 记录每个浏览器订阅上次推送的结果。

-- +goose Up
ALTER TABLE webpush_subscriptions ADD COLUMN last_ok_at DATETIME;
ALTER TABLE webpush_subscriptions ADD COLUMN last_error TEXT;
ALTER TABLE webpush_subscriptions ADD COLUMN last_error_at DATETIME;

-- +goose Down
-- 新增的列在回滚时保留。
SELECT 1;
