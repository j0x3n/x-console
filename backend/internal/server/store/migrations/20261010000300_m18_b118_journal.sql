-- +goose Up
-- B118: 每日时间线和日记。
-- journal_items 是各模块当天发生的事的存档，(source, ref) 唯一。
-- day 是采集时按服务器时区算出的日期（YYYY-MM-DD）。module 是来源对应的可隐藏模块，
-- 隐藏内容锁着时按它过滤。minutes 只有专注、电脑时间这类有时长的才填。
CREATE TABLE journal_items (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 day TEXT NOT NULL,
 at DATETIME NOT NULL,
 module TEXT NOT NULL,
 source TEXT NOT NULL,
 ref TEXT NOT NULL,
 kind TEXT NOT NULL,
 title TEXT NOT NULL,
 detail TEXT NOT NULL DEFAULT '',
 link TEXT NOT NULL DEFAULT '',
 minutes INTEGER NOT NULL DEFAULT 0,
 UNIQUE (source, ref)
);
CREATE INDEX journal_items_day ON journal_items(day, at);
CREATE INDEX journal_items_source_at ON journal_items(source, at);

-- 日记：一天一篇，内容为空就删掉这一行。
CREATE TABLE journal_diary (
 day TEXT PRIMARY KEY,
 body TEXT NOT NULL,
 updated_at DATETIME NOT NULL
);

-- +goose Down
SELECT 1;
