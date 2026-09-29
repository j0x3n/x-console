-- B27 服务器月流量：按小时和按天累加的流量，以及每台机器的统计周期和上限。

-- +goose Up
CREATE TABLE host_traffic_hourly (
    host_id   TEXT    NOT NULL,
    hour      TEXT    NOT NULL,             -- UTC，YYYY-MM-DDTHH
    rx        INTEGER NOT NULL DEFAULT 0,
    tx        INTEGER NOT NULL DEFAULT 0,
    estimated INTEGER NOT NULL DEFAULT 0,   -- 这一小时有按网速估算的数据
    PRIMARY KEY (host_id, hour)
);

CREATE TABLE host_traffic_daily (
    host_id   TEXT    NOT NULL,
    day       TEXT    NOT NULL,             -- 服务器时区的日期，YYYY-MM-DD
    rx        INTEGER NOT NULL DEFAULT 0,
    tx        INTEGER NOT NULL DEFAULT 0,
    estimated INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (host_id, day)
);

CREATE TABLE host_traffic_plans (
    host_id        TEXT     PRIMARY KEY,
    start_day      INTEGER  NOT NULL DEFAULT 1,
    period_months  INTEGER  NOT NULL DEFAULT 1,
    limit_bytes    INTEGER  NOT NULL DEFAULT 0,
    count_mode     TEXT     NOT NULL DEFAULT 'both',
    alert_percent  INTEGER  NOT NULL DEFAULT 80,
    alerted_cycle  TEXT     NOT NULL DEFAULT '', -- 已经提醒过的周期开始日
    alerted_level  INTEGER  NOT NULL DEFAULT 0,  -- 0 没提醒；1 到了提醒线；2 到了上限
    updated_at     DATETIME NOT NULL
);

-- +goose Down
-- 新增的表在回滚时保留。
SELECT 1;
