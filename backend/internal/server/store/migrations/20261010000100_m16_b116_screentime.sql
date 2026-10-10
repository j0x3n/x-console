-- +goose Up
-- B116: 电脑时间去向。每台电脑每分钟一行。minute 是 Unix 分钟数。
-- title 默认是空字符串，设置里打开“保存窗口标题”后才有内容，30 天后清掉。
CREATE TABLE screen_minutes (
 host_id TEXT NOT NULL,
 minute INTEGER NOT NULL,
 app TEXT NOT NULL,
 category TEXT NOT NULL,
 title TEXT NOT NULL DEFAULT '',
 PRIMARY KEY (host_id, minute)
) WITHOUT ROWID;
CREATE INDEX screen_minutes_minute ON screen_minutes(minute);

-- field 是 app（程序名相等）或 title（标题包含），都不分大小写。
CREATE TABLE screen_rules (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 field TEXT NOT NULL,
 pattern TEXT NOT NULL,
 category TEXT NOT NULL,
 created_at DATETIME NOT NULL
);

-- +goose Down
SELECT 1;
