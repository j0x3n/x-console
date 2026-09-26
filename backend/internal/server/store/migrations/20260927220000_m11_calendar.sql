-- M11 日历：订阅的日历和同步下来的事件。
-- 事件的 starts_at/ends_at/rdates/exdates/recurrence_id 存的是“墙上时间”，按 tzid 指定的时区理解。
-- tzid 为空表示浮动时间，按用户时区理解。全天事件只看日期。

-- +goose Up
CREATE TABLE calendars (
    id             INTEGER PRIMARY KEY,
    name           TEXT     NOT NULL,
    kind           TEXT     NOT NULL CHECK (kind IN ('ics', 'caldav')),
    url            TEXT     NOT NULL,
    username       TEXT     NOT NULL DEFAULT '',
    secret         TEXT     NOT NULL DEFAULT '', -- 密码，用 secrets.Box 加密
    color          TEXT     NOT NULL DEFAULT '',
    enabled        INTEGER  NOT NULL DEFAULT 1,
    last_synced_at DATETIME,
    last_error     TEXT     NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL
);

CREATE TABLE calendar_events (
    id            INTEGER PRIMARY KEY,
    calendar_id   INTEGER  NOT NULL REFERENCES calendars (id) ON DELETE CASCADE,
    uid           TEXT     NOT NULL DEFAULT '',
    title         TEXT     NOT NULL DEFAULT '',
    starts_at     DATETIME NOT NULL,
    ends_at       DATETIME NOT NULL,
    all_day       INTEGER  NOT NULL DEFAULT 0,
    tzid          TEXT     NOT NULL DEFAULT '',
    location      TEXT     NOT NULL DEFAULT '',
    description   TEXT     NOT NULL DEFAULT '',
    rrule         TEXT     NOT NULL DEFAULT '',   -- 空表示不重复
    rdates        TEXT     NOT NULL DEFAULT '[]', -- JSON，额外的发生时间
    exdates       TEXT     NOT NULL DEFAULT '[]', -- JSON，排除的发生时间
    recurrence_id DATETIME                        -- 改过的单次发生：它原本的开始时间
);
CREATE INDEX calendar_events_calendar ON calendar_events (calendar_id);
CREATE INDEX calendar_events_start ON calendar_events (starts_at);

-- +goose Down
DROP TABLE calendar_events;
DROP TABLE calendars;
