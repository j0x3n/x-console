-- +goose Up
-- B117: 稍后读。url 是去掉 # 和 utm_ 参数后的网址，唯一。
-- content 是抽取出的纯文本正文，summary 是 AI 写的三行摘要，tags_json 是标签数组。
-- title_locked 为 1 表示标题是用户改的，重新抓取不覆盖。
-- status: queued 等着抓，ready 抓好了，failed 抓取失败（error 写原因）。
CREATE TABLE read_items (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 url TEXT NOT NULL UNIQUE,
 title TEXT NOT NULL DEFAULT '',
 title_locked INTEGER NOT NULL DEFAULT 0,
 site TEXT NOT NULL DEFAULT '',
 excerpt TEXT NOT NULL DEFAULT '',
 content TEXT NOT NULL DEFAULT '',
 summary TEXT NOT NULL DEFAULT '',
 tags_json TEXT NOT NULL DEFAULT '[]',
 note TEXT NOT NULL DEFAULT '',
 source TEXT NOT NULL DEFAULT 'web',
 status TEXT NOT NULL DEFAULT 'queued',
 error TEXT NOT NULL DEFAULT '',
 attempts INTEGER NOT NULL DEFAULT 0,
 read_at DATETIME,
 created_at DATETIME NOT NULL,
 fetched_at DATETIME,
 updated_at DATETIME NOT NULL
);
CREATE INDEX read_items_created ON read_items(created_at);
CREATE INDEX read_items_status ON read_items(status);

-- +goose Down
SELECT 1;
