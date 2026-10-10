-- +goose Up
-- B120: 密钥和令牌台账。只记信息，不存密钥本身。日期都是 YYYY-MM-DD 的文本，空字符串表示没有。
-- used_by 是用在哪里的 JSON 数组。rotate_every_days 为 0 表示不设更换周期。
-- notified_for 是已提醒记录对应的到期日，到期日变了就清空 notified_json。
-- stale_for 是上次“久未更换”提醒对应的更换到期日，stale_notified_on 是提醒那天，
-- 更换到期日变了就当作新的一轮。
CREATE TABLE credentials (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 kind TEXT NOT NULL,
 name TEXT NOT NULL,
 platform TEXT NOT NULL DEFAULT '',
 account TEXT NOT NULL DEFAULT '',
 used_by TEXT NOT NULL DEFAULT '[]',
 scopes TEXT NOT NULL DEFAULT '',
 hint TEXT NOT NULL DEFAULT '',
 created_on TEXT NOT NULL DEFAULT '',
 rotated_on TEXT NOT NULL DEFAULT '',
 expires_on TEXT NOT NULL DEFAULT '',
 rotate_every_days INTEGER NOT NULL DEFAULT 0,
 remind_days TEXT NOT NULL DEFAULT '30,7',
 notes TEXT NOT NULL DEFAULT '',
 notified_for TEXT NOT NULL DEFAULT '',
 notified_json TEXT NOT NULL DEFAULT '[]',
 stale_for TEXT NOT NULL DEFAULT '',
 stale_notified_on TEXT NOT NULL DEFAULT '',
 archived_at DATETIME,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
);
CREATE INDEX credentials_expires ON credentials(expires_on);

-- +goose Down
SELECT 1;
