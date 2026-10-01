-- +goose Up
CREATE TABLE note_shares (
    note_id INTEGER PRIMARY KEY REFERENCES notes(id) ON DELETE CASCADE,
    token TEXT NOT NULL UNIQUE,
    password_hash TEXT,
    expires_at DATETIME,
    visits INTEGER NOT NULL DEFAULT 0,
    last_visit_at DATETIME,
    created_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE note_shares;
