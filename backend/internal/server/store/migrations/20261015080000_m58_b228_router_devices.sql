-- B228、B229、B230、B232：路由器主动上报（B114）模式下的设备、设备流量、事件和系统日志。
-- 四张表放在同一个迁移里，只加不删。

-- +goose Up
-- 面板见过的每台设备。时间是 Unix 秒。
CREATE TABLE router_devices (
    mac              TEXT PRIMARY KEY,           -- 大写，冒号分隔
    note             TEXT NOT NULL DEFAULT '',   -- 备注，最多 40 个字
    notify           INTEGER NOT NULL DEFAULT 0,
    cooldown_minutes INTEGER,                    -- NULL 表示跟随默认
    conn             TEXT NOT NULL DEFAULT '',   -- wifi、wired，没判断出来是空
    wifi_iface       TEXT NOT NULL DEFAULT '',
    last_name        TEXT NOT NULL DEFAULT '',
    last_ip          TEXT NOT NULL DEFAULT '',
    online           INTEGER NOT NULL DEFAULT 0,
    online_since     INTEGER,                    -- 这次上线的时间
    offline_since    INTEGER,                    -- 这次离线的时间
    first_seen       INTEGER NOT NULL,
    last_seen        INTEGER NOT NULL,
    notified_at      INTEGER,                    -- 上一次推送的时间
    notified_online  INTEGER                     -- 上一次推送的是上线（1）还是离线（0）
);

-- 每台设备每小时的流量（nlbwmon），留 30 天。hour 是整点的 Unix 秒。
CREATE TABLE router_device_traffic (
    mac  TEXT NOT NULL,
    hour INTEGER NOT NULL,
    rx   INTEGER NOT NULL DEFAULT 0,
    tx   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (mac, hour)
) WITHOUT ROWID;
CREATE INDEX router_device_traffic_hour ON router_device_traffic(hour);

-- 面板自己记的事件。
CREATE TABLE router_events (
    id     INTEGER PRIMARY KEY,
    at     INTEGER NOT NULL,
    kind   TEXT NOT NULL,   -- device_new、device_online、device_offline、devices_seeded、wan_down、wan_up、report_lost、report_back、interface_restart、reboot
    mac    TEXT,            -- 设备事件才有
    title  TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT ''
);
CREATE INDEX router_events_at ON router_events(at);
CREATE INDEX router_events_mac ON router_events(mac, id);

-- 路由器的系统日志（logread）。
CREATE TABLE router_logs (
    id       INTEGER PRIMARY KEY,
    at       INTEGER NOT NULL,
    level    TEXT NOT NULL DEFAULT '', -- emerg、alert、crit、err、warn、notice、info、debug
    facility TEXT NOT NULL DEFAULT '',
    program  TEXT NOT NULL DEFAULT '',
    message  TEXT NOT NULL
);
CREATE INDEX router_logs_at ON router_logs(at);

-- +goose Down
DROP TABLE router_logs;
DROP TABLE router_events;
DROP TABLE router_device_traffic;
DROP TABLE router_devices;
