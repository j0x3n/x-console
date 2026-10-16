-- +goose Up
-- B236: 网站自动化的检查时段。active_from、active_to 是 HH:MM（服务器时区），都为空表示全天检查。
-- active_to 比 active_from 小表示跨午夜。active_days 是逗号分隔的星期（1 到 7，周一是 1），空表示每天。
ALTER TABLE watchers ADD COLUMN active_from TEXT NOT NULL DEFAULT '';
ALTER TABLE watchers ADD COLUMN active_to TEXT NOT NULL DEFAULT '';
ALTER TABLE watchers ADD COLUMN active_days TEXT NOT NULL DEFAULT '';

-- +goose Down
SELECT 1;
