-- +goose Up
CREATE TABLE ai_memories (
    id         INTEGER  PRIMARY KEY,
    text       TEXT     NOT NULL,
    source     TEXT     NOT NULL DEFAULT 'user',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE ai_memories;
