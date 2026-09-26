-- M11 番茄钟：每次专注一行。ended_at 为空表示正在进行。

-- +goose Up
CREATE TABLE focus_sessions (
    id              INTEGER PRIMARY KEY,
    issue_key       TEXT     NOT NULL DEFAULT '',
    started_at      DATETIME NOT NULL,
    ended_at        DATETIME,
    planned_minutes INTEGER  NOT NULL,
    actual_seconds  INTEGER  NOT NULL DEFAULT 0,
    completed       INTEGER  NOT NULL DEFAULT 0,
    note            TEXT     NOT NULL DEFAULT '',
    notified_at     DATETIME -- 到点通知已发出
);
CREATE INDEX focus_sessions_started_at ON focus_sessions (started_at);
CREATE INDEX focus_sessions_ended_at ON focus_sessions (ended_at);

-- +goose Down
DROP TABLE focus_sessions;
