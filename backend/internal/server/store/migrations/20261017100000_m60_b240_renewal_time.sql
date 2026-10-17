-- +goose Up
-- B240: 订阅可选的续费时间。renewal_time 是 HH:MM（服务器时区），空字符串表示只按天，和以前一样。
ALTER TABLE subscriptions ADD COLUMN renewal_time TEXT NOT NULL DEFAULT '';

-- +goose Down
SELECT 1;
