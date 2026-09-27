-- +goose Up
ALTER TABLE notes ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0;
CREATE INDEX notes_hidden_list ON notes (hidden, pinned DESC, updated_at DESC);
DROP TRIGGER notes_fts_insert;
DROP TRIGGER notes_fts_delete;
DROP TRIGGER notes_fts_update;

-- +goose StatementBegin
CREATE TRIGGER notes_fts_insert AFTER INSERT ON notes WHEN new.hidden = 0 BEGIN
    INSERT INTO notes_fts (rowid, title, body) VALUES (new.id, new.title, new.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER notes_fts_delete AFTER DELETE ON notes WHEN old.hidden = 0 BEGIN
    INSERT INTO notes_fts (notes_fts, rowid, title, body) VALUES ('delete', old.id, old.title, old.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER notes_fts_update AFTER UPDATE OF title, body, hidden ON notes BEGIN
    INSERT INTO notes_fts (notes_fts, rowid, title, body)
        SELECT 'delete', old.id, old.title, old.body WHERE old.hidden = 0;
    INSERT INTO notes_fts (rowid, title, body)
        SELECT new.id, new.title, new.body WHERE new.hidden = 0;
END;
-- +goose StatementEnd

-- +goose Down
