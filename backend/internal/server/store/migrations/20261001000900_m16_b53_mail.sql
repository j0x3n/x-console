-- +goose Up
-- B53：邮箱账号和收件箱的本地缓存。正文第一次打开时才取，存在 body_json。
CREATE TABLE mail_accounts (
    id           INTEGER PRIMARY KEY,
    name         TEXT     NOT NULL,
    email        TEXT     NOT NULL,
    provider     TEXT     NOT NULL,          -- gmail、aliyun、other
    imap_host    TEXT     NOT NULL,
    imap_port    INTEGER  NOT NULL DEFAULT 993,
    username     TEXT     NOT NULL,
    password_enc TEXT     NOT NULL,          -- secrets.Box 加密
    notify       INTEGER  NOT NULL DEFAULT 1,
    uid_validity INTEGER  NOT NULL DEFAULT 0, -- 收件箱的 UIDVALIDITY，变了要重新同步
    last_uid     INTEGER  NOT NULL DEFAULT 0, -- 已经同步到的最大 UID
    last_sync_at DATETIME,
    created_at   DATETIME NOT NULL
);

CREATE TABLE mail_messages (
    id              INTEGER PRIMARY KEY,
    account_id      INTEGER  NOT NULL REFERENCES mail_accounts (id) ON DELETE CASCADE,
    uid             INTEGER  NOT NULL,
    message_id      TEXT     NOT NULL DEFAULT '',
    from_name       TEXT     NOT NULL DEFAULT '',
    from_address    TEXT     NOT NULL DEFAULT '',
    subject         TEXT     NOT NULL DEFAULT '',
    snippet         TEXT     NOT NULL DEFAULT '',
    date            DATETIME NOT NULL,
    unread          INTEGER  NOT NULL DEFAULT 1,
    flagged         INTEGER  NOT NULL DEFAULT 0,
    has_attachments INTEGER  NOT NULL DEFAULT 0,
    body_json       TEXT,
    UNIQUE (account_id, uid)
);
CREATE INDEX mail_messages_date ON mail_messages (account_id, date DESC);
CREATE INDEX mail_messages_all_date ON mail_messages (date DESC);

-- +goose Down
DROP TABLE mail_messages;
DROP TABLE mail_accounts;
