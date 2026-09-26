-- M8 习惯与健康：习惯、打卡、训练计划、训练记录。

-- +goose Up
CREATE TABLE habits (
    id                      INTEGER PRIMARY KEY,
    name                    TEXT     NOT NULL,
    icon                    TEXT     NOT NULL DEFAULT '',
    color                   TEXT     NOT NULL DEFAULT '',
    unit                    TEXT     NOT NULL DEFAULT '次',
    daily_target            REAL     NOT NULL DEFAULT 1,
    remind_mode             TEXT     NOT NULL DEFAULT 'none' CHECK (remind_mode IN ('none', 'interval', 'times')),
    remind_interval_minutes INTEGER  NOT NULL DEFAULT 0,
    remind_window           TEXT     NOT NULL DEFAULT '',   -- 09:00-21:00
    remind_times            TEXT     NOT NULL DEFAULT '[]', -- JSON，例如 ["08:00","20:00"]
    ha_entity_id            TEXT     NOT NULL DEFAULT '',
    archived_at             DATETIME,
    sort_order              INTEGER  NOT NULL DEFAULT 0,
    created_at              DATETIME NOT NULL,
    last_reminded_at        DATETIME,                       -- 上次发提醒的时间
    quiet_until             DATETIME                        -- 通知上点了“跳过”，这之前不再提醒
);

CREATE TABLE habit_logs (
    id       INTEGER PRIMARY KEY,
    habit_id INTEGER  NOT NULL REFERENCES habits (id) ON DELETE CASCADE,
    at       DATETIME NOT NULL,
    amount   REAL     NOT NULL DEFAULT 1,
    source   TEXT     NOT NULL DEFAULT 'web', -- web/telegram/webpush/ha/ai/automation
    note     TEXT     NOT NULL DEFAULT ''
);
CREATE INDEX habit_logs_habit_at ON habit_logs (habit_id, at);

CREATE TABLE workout_plans (
    id      INTEGER PRIMARY KEY,
    weekday INTEGER NOT NULL CHECK (weekday BETWEEN 1 AND 7), -- 1 是周一
    title   TEXT    NOT NULL DEFAULT '',
    items   TEXT    NOT NULL DEFAULT '[]' -- JSON：[{name, sets, reps, weight, note}]
);
CREATE INDEX workout_plans_weekday ON workout_plans (weekday);

CREATE TABLE workout_logs (
    id               INTEGER PRIMARY KEY,
    date             TEXT     NOT NULL, -- 用户时区的日期 YYYY-MM-DD
    plan_id          INTEGER  REFERENCES workout_plans (id) ON DELETE SET NULL,
    items            TEXT     NOT NULL DEFAULT '[]',
    duration_minutes INTEGER  NOT NULL DEFAULT 0,
    note             TEXT     NOT NULL DEFAULT '',
    created_at       DATETIME NOT NULL
);
CREATE INDEX workout_logs_date ON workout_logs (date);

-- +goose Down
DROP TABLE workout_logs;
DROP TABLE workout_plans;
DROP TABLE habit_logs;
DROP TABLE habits;
