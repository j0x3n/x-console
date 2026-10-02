-- +goose Up
CREATE TABLE github_event_states (
    connection_id INTEGER NOT NULL DEFAULT 0,
    repo TEXT NOT NULL,
    resource TEXT NOT NULL,
    object_id TEXT NOT NULL,
    data TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (connection_id, repo, resource, object_id)
);
CREATE TABLE github_event_seen (
    connection_id INTEGER NOT NULL DEFAULT 0,
    repo TEXT NOT NULL,
    event TEXT NOT NULL,
    object_id TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT '',
    expires_at DATETIME NOT NULL,
    PRIMARY KEY (connection_id, repo, event, object_id, state)
);
CREATE INDEX github_event_seen_expiry ON github_event_seen(expires_at);
CREATE TABLE github_notify_windows (
    connection_id INTEGER NOT NULL DEFAULT 0,
    repo TEXT NOT NULL,
    event TEXT NOT NULL,
    started_at DATETIME NOT NULL,
    sent INTEGER NOT NULL DEFAULT 0,
    suppressed INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (connection_id, repo, event)
);

-- +goose Down
