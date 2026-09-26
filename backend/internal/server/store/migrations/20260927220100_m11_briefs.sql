-- M11 每日早报：每天一份，date 是用户时区的日期。

-- +goose Up
CREATE TABLE briefs (
    id         INTEGER PRIMARY KEY,
    date       TEXT     NOT NULL UNIQUE,       -- YYYY-MM-DD
    content    TEXT     NOT NULL,              -- Markdown
    sections   TEXT     NOT NULL DEFAULT '[]', -- JSON：生成出来的各部分
    created_at DATETIME NOT NULL,
    sent_at    DATETIME
);

-- +goose Down
DROP TABLE briefs;
