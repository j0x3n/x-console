-- +goose Up
CREATE TABLE board_repos (
    board_id INTEGER PRIMARY KEY REFERENCES project_boards(id) ON DELETE CASCADE,
    connection_id INTEGER NOT NULL REFERENCES git_connections(id),
    full_name TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL,
    html_url TEXT NOT NULL DEFAULT '',
    clone_url TEXT NOT NULL DEFAULT '',
    default_branch TEXT NOT NULL DEFAULT 'main',
    sync_issues INTEGER NOT NULL DEFAULT 1,
    last_synced_at DATETIME,
    last_error TEXT NOT NULL DEFAULT ''
);
ALTER TABLE issues ADD COLUMN external_url TEXT NOT NULL DEFAULT '';

-- +goose Down
SELECT 1;
