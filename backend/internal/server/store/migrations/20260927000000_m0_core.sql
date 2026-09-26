-- M0 基础：用户、会话、审计、设置、代理、通知。
-- 时间一律由 Go 写入 UTC，列类型用 DATETIME，驱动会读回 time.Time。

-- +goose Up
CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    username      TEXT     NOT NULL UNIQUE,
    password_hash TEXT     NOT NULL,
    totp_secret   TEXT     NOT NULL DEFAULT '', -- secrets.Box 加密后的密文
    totp_enabled  INTEGER  NOT NULL DEFAULT 0,
    created_at    DATETIME NOT NULL
);

CREATE TABLE sessions (
    id             TEXT     PRIMARY KEY, -- 会话令牌的 SHA-256，令牌原文只在 Cookie 里
    user_id        INTEGER  NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at     DATETIME NOT NULL,
    expires_at     DATETIME NOT NULL,
    elevated_until DATETIME,
    user_agent     TEXT     NOT NULL DEFAULT '',
    ip             TEXT     NOT NULL DEFAULT ''
);

CREATE TABLE audit_log (
    id     INTEGER PRIMARY KEY,
    at     DATETIME NOT NULL,
    actor  TEXT     NOT NULL, -- 用户名，或 system、automation:<id>、agent:<id>
    action TEXT     NOT NULL, -- 例如 auth.login、host.exec、ha.call_service
    target TEXT     NOT NULL DEFAULT '',
    detail TEXT     NOT NULL DEFAULT '{}', -- JSON
    result TEXT     NOT NULL DEFAULT 'ok'  -- ok / error: <消息>
);
CREATE INDEX audit_log_at ON audit_log (at DESC);

CREATE TABLE settings (
    key        TEXT     PRIMARY KEY, -- <模块>.<名称>，例如 ha.url
    value      TEXT     NOT NULL,    -- JSON；encrypted=1 时是密文
    encrypted  INTEGER  NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL
);

CREATE TABLE agents (
    id           TEXT     PRIMARY KEY,
    name         TEXT     NOT NULL,
    kind         TEXT     NOT NULL CHECK (kind IN ('server', 'desktop')),
    os           TEXT     NOT NULL DEFAULT '',
    arch         TEXT     NOT NULL DEFAULT '',
    hostname     TEXT     NOT NULL DEFAULT '',
    version      TEXT     NOT NULL DEFAULT '',
    capabilities TEXT     NOT NULL DEFAULT '[]', -- JSON 字符串数组
    token_hash   TEXT     NOT NULL UNIQUE,
    created_at   DATETIME NOT NULL,
    last_seen_at DATETIME,
    revoked_at   DATETIME
);

CREATE TABLE pairing_codes (
    code_hash  TEXT     PRIMARY KEY,
    name       TEXT     NOT NULL,
    kind       TEXT     NOT NULL,
    expires_at DATETIME NOT NULL,
    used_at    DATETIME
);

CREATE TABLE notifications (
    id         INTEGER PRIMARY KEY,
    created_at DATETIME NOT NULL,
    kind       TEXT     NOT NULL,
    title      TEXT     NOT NULL,
    body       TEXT     NOT NULL DEFAULT '',
    link       TEXT     NOT NULL DEFAULT '',
    priority   TEXT     NOT NULL DEFAULT 'normal',
    source     TEXT     NOT NULL DEFAULT '',
    data       TEXT     NOT NULL DEFAULT '{}', -- JSON，渠道按钮等附加信息
    read_at    DATETIME
);
CREATE INDEX notifications_created ON notifications (created_at DESC);

-- +goose Down
DROP TABLE notifications;
DROP TABLE pairing_codes;
DROP TABLE agents;
DROP TABLE settings;
DROP TABLE audit_log;
DROP TABLE sessions;
DROP TABLE users;
