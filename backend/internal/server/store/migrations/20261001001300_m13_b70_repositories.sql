-- +goose Up
CREATE TABLE github_cache_v2 (
    connection_id INTEGER NOT NULL DEFAULT 0,
    repo TEXT NOT NULL,
    kind TEXT NOT NULL,
    object_id TEXT NOT NULL,
    data TEXT NOT NULL DEFAULT '{}',
    updated_at DATETIME NOT NULL,
    PRIMARY KEY (connection_id, repo, kind, object_id)
);
CREATE TABLE github_synthetic_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    connection_id INTEGER NOT NULL DEFAULT 0,
    repo TEXT NOT NULL,
    sha TEXT NOT NULL,
    UNIQUE (connection_id, repo, sha)
);

-- +goose Down
