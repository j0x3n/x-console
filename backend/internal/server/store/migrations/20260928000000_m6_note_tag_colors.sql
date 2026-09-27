-- 笔记标签的颜色。没设过的标签不在这张表里，前端按标签名选一个颜色。

-- +goose Up
CREATE TABLE note_tag_colors (
    tag   TEXT PRIMARY KEY,
    color TEXT NOT NULL -- 形如 #cc7752
);

-- +goose Down
DROP TABLE note_tag_colors;
