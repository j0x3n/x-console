-- B43 API 令牌：给外部 AI 用 MCP 接口。库里只存哈希和前 8 位。

-- +goose Up
CREATE TABLE api_tokens (
    id           INTEGER  PRIMARY KEY,
    name         TEXT     NOT NULL,
    prefix       TEXT     NOT NULL,          -- 令牌前 8 位，列表里给人认
    token_hash   TEXT     NOT NULL UNIQUE,   -- SHA-256
    access       TEXT     NOT NULL CHECK (access IN ('read', 'write', 'write_delete')),
    modules      TEXT     NOT NULL DEFAULT '[]', -- JSON 数组，空表示全部模块
    expires_at   DATETIME,
    created_at   DATETIME NOT NULL,
    last_used_at DATETIME,
    last_used_ip TEXT     NOT NULL DEFAULT '',
    revoked_at   DATETIME
);

-- +goose Down
DROP TABLE api_tokens;
