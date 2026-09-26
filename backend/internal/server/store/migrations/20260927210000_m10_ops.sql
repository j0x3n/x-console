-- M10 运维监控：脚本库、网站/证书/域名监控、订阅与续费。
-- 时间一律由 Go 写入 UTC。Docker 部分没有表。

-- +goose Up
CREATE TABLE scripts (
    id               INTEGER PRIMARY KEY,
    name             TEXT     NOT NULL,
    description      TEXT     NOT NULL DEFAULT '',
    shell            TEXT     NOT NULL DEFAULT 'bash', -- bash、sh、powershell
    body             TEXT     NOT NULL,
    default_host_ids TEXT     NOT NULL DEFAULT '[]',   -- JSON 字符串数组
    timeout_seconds  INTEGER  NOT NULL DEFAULT 300,
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL
);

CREATE TABLE script_runs (
    id           INTEGER PRIMARY KEY,
    script_id    INTEGER  NOT NULL REFERENCES scripts (id) ON DELETE CASCADE,
    host_id      TEXT     NOT NULL,
    host_name    TEXT     NOT NULL DEFAULT '',
    started_at   DATETIME NOT NULL,
    finished_at  DATETIME,                      -- NULL 表示还在运行
    exit_code    INTEGER,                       -- NULL 表示没能执行（见 error）
    stdout       TEXT     NOT NULL DEFAULT '',
    stderr       TEXT     NOT NULL DEFAULT '',
    error        TEXT     NOT NULL DEFAULT '',
    triggered_by TEXT     NOT NULL DEFAULT 'user' -- user、automation、ai
);
CREATE INDEX script_runs_script ON script_runs (script_id, id);

CREATE TABLE monitors (
    id                   INTEGER PRIMARY KEY,
    kind                 TEXT     NOT NULL,              -- http、tls、domain
    name                 TEXT     NOT NULL,
    target               TEXT     NOT NULL,
    interval_seconds     INTEGER  NOT NULL,
    expected_status      INTEGER  NOT NULL DEFAULT 0,    -- 0 表示 200 到 399
    keyword              TEXT     NOT NULL DEFAULT '',
    timeout_ms           INTEGER  NOT NULL DEFAULT 10000,
    enabled              INTEGER  NOT NULL DEFAULT 1,
    last_status          TEXT     NOT NULL DEFAULT 'unknown', -- unknown、up、down
    last_checked_at      DATETIME,
    last_error           TEXT     NOT NULL DEFAULT '',
    consecutive_failures INTEGER  NOT NULL DEFAULT 0,
    expires_at           DATETIME,                       -- 证书或域名到期时间
    expiry_notified      TEXT     NOT NULL DEFAULT '[]', -- 这个到期时间已经提醒过的天数
    created_at           DATETIME NOT NULL
);

CREATE TABLE monitor_results (
    id          INTEGER PRIMARY KEY,
    monitor_id  INTEGER  NOT NULL REFERENCES monitors (id) ON DELETE CASCADE,
    at          DATETIME NOT NULL,
    ok          INTEGER  NOT NULL,
    status_code INTEGER,
    latency_ms  INTEGER  NOT NULL DEFAULT 0,
    error       TEXT     NOT NULL DEFAULT '',
    detail      TEXT     NOT NULL DEFAULT '{}'  -- JSON：证书到期、域名到期
);
CREATE INDEX monitor_results_monitor_at ON monitor_results (monitor_id, at);
CREATE INDEX monitor_results_at ON monitor_results (at);

CREATE TABLE subscriptions (
    id                 INTEGER PRIMARY KEY,
    name               TEXT     NOT NULL,
    category           TEXT     NOT NULL DEFAULT 'other',   -- server、domain、saas、other
    amount             REAL     NOT NULL DEFAULT 0,
    currency           TEXT     NOT NULL DEFAULT 'CNY',
    cycle              TEXT     NOT NULL DEFAULT 'monthly', -- monthly、yearly、custom_days
    cycle_days         INTEGER  NOT NULL DEFAULT 0,
    next_renewal       TEXT     NOT NULL,                   -- 本地日期 YYYY-MM-DD
    remind_days_before TEXT     NOT NULL DEFAULT '[7,1]',   -- JSON 整数数组
    reminded           TEXT     NOT NULL DEFAULT '[]',      -- 这次续费已经提醒过的天数
    url                TEXT     NOT NULL DEFAULT '',
    note               TEXT     NOT NULL DEFAULT '',
    auto_renew         INTEGER  NOT NULL DEFAULT 0,
    archived_at        DATETIME,
    created_at         DATETIME NOT NULL,
    updated_at         DATETIME NOT NULL
);
CREATE INDEX subscriptions_next_renewal ON subscriptions (next_renewal);

CREATE TABLE subscription_events (
    id              INTEGER PRIMARY KEY,
    subscription_id INTEGER  NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
    at              DATETIME NOT NULL,
    kind            TEXT     NOT NULL, -- created、reminded、renewed、updated
    detail          TEXT     NOT NULL DEFAULT ''
);
CREATE INDEX subscription_events_sub ON subscription_events (subscription_id, id);

-- +goose Down
DROP TABLE subscription_events;
DROP TABLE subscriptions;
DROP TABLE monitor_results;
DROP TABLE monitors;
DROP TABLE script_runs;
DROP TABLE scripts;
