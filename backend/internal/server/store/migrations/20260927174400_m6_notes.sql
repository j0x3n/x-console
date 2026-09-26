-- M6 备忘：笔记、标签和全文索引。
-- notes_fts 用 trigram 分词器，三个字以上的中文词也能搜到；更短的查询在 Go 里退回 LIKE。

-- +goose Up
CREATE TABLE notes (
    id          INTEGER PRIMARY KEY,
    title       TEXT     NOT NULL DEFAULT '',
    body        TEXT     NOT NULL DEFAULT '', -- Markdown
    pinned      INTEGER  NOT NULL DEFAULT 0,
    archived_at DATETIME,
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL
);
CREATE INDEX notes_list ON notes (pinned DESC, updated_at DESC);

CREATE TABLE note_tags (
    note_id INTEGER NOT NULL REFERENCES notes (id) ON DELETE CASCADE,
    tag     TEXT    NOT NULL,
    PRIMARY KEY (note_id, tag)
);
CREATE INDEX note_tags_tag ON note_tags (tag);

CREATE VIRTUAL TABLE notes_fts USING fts5 (
    title, body,
    content = 'notes', content_rowid = 'id',
    tokenize = 'trigram'
);

-- +goose StatementBegin
CREATE TRIGGER notes_fts_insert AFTER INSERT ON notes BEGIN
    INSERT INTO notes_fts (rowid, title, body) VALUES (new.id, new.title, new.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER notes_fts_delete AFTER DELETE ON notes BEGIN
    INSERT INTO notes_fts (notes_fts, rowid, title, body) VALUES ('delete', old.id, old.title, old.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER notes_fts_update AFTER UPDATE OF title, body ON notes BEGIN
    INSERT INTO notes_fts (notes_fts, rowid, title, body) VALUES ('delete', old.id, old.title, old.body);
    INSERT INTO notes_fts (rowid, title, body) VALUES (new.id, new.title, new.body);
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER notes_fts_update;
DROP TRIGGER notes_fts_delete;
DROP TRIGGER notes_fts_insert;
DROP TABLE notes_fts;
DROP TABLE note_tags;
DROP TABLE notes;
