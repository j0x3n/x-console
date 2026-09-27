-- +goose Up
CREATE TABLE note_attachments (
    id INTEGER PRIMARY KEY,
    note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    mime TEXT NOT NULL,
    size INTEGER NOT NULL,
    sha256 TEXT NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX note_attachments_note ON note_attachments(note_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE note_attachments;
