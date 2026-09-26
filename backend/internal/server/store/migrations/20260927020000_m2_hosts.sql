-- M2 服务器 / M3 本机：指标汇总、SSH 主机、告警规则和告警记录。
-- host_id 是代理 id，SSH 主机是 "ssh:<id>"。10 秒一条的原始指标只在内存里。

-- +goose Up
CREATE TABLE host_metrics_1m (
    host_id   TEXT     NOT NULL,
    at        DATETIME NOT NULL, -- 这一分钟的开始时间
    cpu       REAL     NOT NULL,
    mem_used  INTEGER  NOT NULL,
    mem_total INTEGER  NOT NULL,
    disk_json TEXT     NOT NULL DEFAULT '[]', -- [{mount,fsType,used,total}]，取这一分钟最后一条
    net_rx    REAL     NOT NULL DEFAULT 0,    -- 字节/秒
    net_tx    REAL     NOT NULL DEFAULT 0,
    load1     REAL     NOT NULL DEFAULT 0,
    PRIMARY KEY (host_id, at)
);
CREATE INDEX host_metrics_1m_at ON host_metrics_1m (at);

CREATE TABLE host_metrics_1h (
    host_id   TEXT     NOT NULL,
    at        DATETIME NOT NULL, -- 这一小时的开始时间
    cpu       REAL     NOT NULL,
    mem_used  INTEGER  NOT NULL,
    mem_total INTEGER  NOT NULL,
    disk_json TEXT     NOT NULL DEFAULT '[]',
    net_rx    REAL     NOT NULL DEFAULT 0,
    net_tx    REAL     NOT NULL DEFAULT 0,
    load1     REAL     NOT NULL DEFAULT 0,
    PRIMARY KEY (host_id, at)
);
CREATE INDEX host_metrics_1h_at ON host_metrics_1h (at);

CREATE TABLE ssh_hosts (
    id         INTEGER PRIMARY KEY,
    name       TEXT     NOT NULL,
    address    TEXT     NOT NULL,
    port       INTEGER  NOT NULL DEFAULT 22,
    username   TEXT     NOT NULL,
    auth       TEXT     NOT NULL CHECK (auth IN ('password', 'key')),
    secret     TEXT     NOT NULL,             -- secrets.Box 加密：密码，或 JSON {key, passphrase}
    host_key   TEXT     NOT NULL DEFAULT '',  -- 第一次连接时记下的主机公钥（authorized_keys 格式）
    created_at DATETIME NOT NULL
);

CREATE TABLE alert_rules (
    id               INTEGER PRIMARY KEY,
    host_id          TEXT,                     -- 空表示所有机器
    metric           TEXT     NOT NULL CHECK (metric IN ('cpu', 'memory', 'disk', 'offline')),
    op               TEXT     NOT NULL DEFAULT 'gt' CHECK (op IN ('gt', 'lt')),
    threshold        REAL     NOT NULL DEFAULT 0,
    duration_seconds INTEGER  NOT NULL DEFAULT 0,
    severity         TEXT     NOT NULL DEFAULT 'warning' CHECK (severity IN ('warning', 'critical')),
    enabled          INTEGER  NOT NULL DEFAULT 1,
    created_at       DATETIME NOT NULL
);

CREATE TABLE alert_events (
    id          INTEGER PRIMARY KEY,
    rule_id     INTEGER REFERENCES alert_rules (id) ON DELETE SET NULL,
    host_id     TEXT     NOT NULL,
    host_name   TEXT     NOT NULL DEFAULT '',
    metric      TEXT     NOT NULL,
    severity    TEXT     NOT NULL,
    value       REAL     NOT NULL DEFAULT 0,
    message     TEXT     NOT NULL,
    fired_at    DATETIME NOT NULL,
    resolved_at DATETIME
);
CREATE INDEX alert_events_fired ON alert_events (fired_at DESC);
CREATE INDEX alert_events_open ON alert_events (host_id) WHERE resolved_at IS NULL;

-- +goose Down
DROP TABLE alert_events;
DROP TABLE alert_rules;
DROP TABLE ssh_hosts;
DROP TABLE host_metrics_1h;
DROP TABLE host_metrics_1m;
