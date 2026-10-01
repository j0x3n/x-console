-- +goose Up
-- B69：网盘账号（WebDAV、Google Drive），备份和云盘页共用。
-- 密码、客户端密钥、令牌加密存在设置里：storage.remote.<id>.password 这类。
CREATE TABLE storage_remotes (
    id            INTEGER PRIMARY KEY,
    kind          TEXT     NOT NULL,              -- webdav、gdrive
    name          TEXT     NOT NULL,
    config        TEXT     NOT NULL DEFAULT '{}', -- 不含密码的连接信息，JSON
    show_in_drive INTEGER  NOT NULL DEFAULT 1,
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL
);

-- +goose Down
DROP TABLE storage_remotes;
