-- M7 提醒与通知：路由规则、提醒、Web Push 订阅。
-- 时间一律由 Go 写入 UTC。

-- +goose Up
CREATE TABLE notification_routes (
    id           INTEGER PRIMARY KEY,
    kind_pattern TEXT    NOT NULL,              -- 例如 host.alert*、*
    min_priority TEXT    NOT NULL DEFAULT 'normal',
    channels     TEXT    NOT NULL DEFAULT '[]', -- JSON 字符串数组
    enabled      INTEGER NOT NULL DEFAULT 1,
    sort_order   INTEGER NOT NULL DEFAULT 0
);
-- 默认规则：普通及以上的通知发到所有已配置好的渠道，没配置的渠道会被跳过。
INSERT INTO notification_routes (kind_pattern, min_priority, channels, enabled, sort_order)
VALUES ('*', 'normal', '["webpush","telegram","bark","serverchan"]', 1, 0);

CREATE TABLE reminders (
    id            INTEGER PRIMARY KEY,
    title         TEXT     NOT NULL,
    body          TEXT     NOT NULL DEFAULT '',
    link          TEXT     NOT NULL DEFAULT '',
    rrule         TEXT     NOT NULL DEFAULT '', -- 空表示一次性
    dtstart       DATETIME NOT NULL,
    next_at       DATETIME,                     -- 下一次按规则到点的时间；NULL 表示没有下一次
    last_fired_at DATETIME,
    snoozed_until DATETIME,
    done_at       DATETIME,
    enabled       INTEGER  NOT NULL DEFAULT 1,
    created_at    DATETIME NOT NULL
);
CREATE INDEX reminders_next_at ON reminders (next_at);
CREATE INDEX reminders_snoozed_until ON reminders (snoozed_until);

CREATE TABLE webpush_subscriptions (
    id         INTEGER PRIMARY KEY,
    endpoint   TEXT     NOT NULL UNIQUE,
    p256dh     TEXT     NOT NULL,
    auth       TEXT     NOT NULL,
    user_agent TEXT     NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE webpush_subscriptions;
DROP TABLE reminders;
DROP TABLE notification_routes;
