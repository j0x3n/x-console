-- B164 按状态推送：队列里的通知等什么。digest 等合并时间点，working 等回到电脑前。

-- +goose Up
ALTER TABLE notify_digest_queue ADD COLUMN hold_for TEXT NOT NULL DEFAULT 'digest';

-- +goose Down
SELECT 1;
