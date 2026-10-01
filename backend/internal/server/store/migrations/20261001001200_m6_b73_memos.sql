-- +goose Up
ALTER TABLE notes ADD COLUMN kind TEXT NOT NULL DEFAULT 'note' CHECK (kind IN ('note','memo'));
ALTER TABLE notes ADD COLUMN color TEXT NOT NULL DEFAULT '';
CREATE INDEX notes_kind_list ON notes(kind, hidden, archived_at, pinned DESC, updated_at DESC, id DESC);

-- +goose Down
DROP INDEX notes_kind_list;
