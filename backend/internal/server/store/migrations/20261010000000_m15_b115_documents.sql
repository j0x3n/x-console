-- +goose Up
-- B115: 证件、合同和物品档案。日期都是 YYYY-MM-DD 的文本，空字符串表示没有。
-- number 用 Secrets 加密。files_json 记云盘文件的编号和名称。
-- notified_for 是已提醒记录对应的到期日，到期日变了就清空 notified_json。
CREATE TABLE documents (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 kind TEXT NOT NULL,
 name TEXT NOT NULL,
 holder TEXT NOT NULL DEFAULT '',
 number TEXT NOT NULL DEFAULT '',
 issued_on TEXT NOT NULL DEFAULT '',
 expires_on TEXT NOT NULL DEFAULT '',
 price REAL,
 currency TEXT NOT NULL DEFAULT '',
 serial TEXT NOT NULL DEFAULT '',
 remind_days TEXT NOT NULL DEFAULT '90,30,7',
 notes TEXT NOT NULL DEFAULT '',
 files_json TEXT NOT NULL DEFAULT '[]',
 notified_for TEXT NOT NULL DEFAULT '',
 notified_json TEXT NOT NULL DEFAULT '[]',
 archived_at DATETIME,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
);
CREATE INDEX documents_expires ON documents(expires_on);

-- +goose Down
SELECT 1;
