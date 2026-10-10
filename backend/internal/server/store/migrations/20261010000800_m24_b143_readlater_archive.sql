-- +goose Up
-- B143: 稍后阅读的完整图文存档和推文。
-- content_html 是清理过的正文 HTML，图片地址指向 read_assets。content 仍是纯文本，用来搜索。
-- kind: page 普通网页，tweet X 推文。meta_json 放作者、发布时间、用哪一步读到的等。
ALTER TABLE read_items ADD COLUMN content_html TEXT NOT NULL DEFAULT '';
ALTER TABLE read_items ADD COLUMN kind TEXT NOT NULL DEFAULT 'page';
ALTER TABLE read_items ADD COLUMN meta_json TEXT NOT NULL DEFAULT '{}';

-- 存下来的图片。文件在 Files.For("readlater") 里，键是 {item_id}/{hash}。
CREATE TABLE read_assets (
 item_id INTEGER NOT NULL REFERENCES read_items(id) ON DELETE CASCADE,
 hash TEXT NOT NULL,
 mime TEXT NOT NULL,
 size INTEGER NOT NULL,
 src_url TEXT NOT NULL DEFAULT '',
 created_at DATETIME NOT NULL,
 PRIMARY KEY (item_id, hash)
);

-- +goose Down
SELECT 1;
