-- +goose Up
-- B149: public links to a playlist. The link plays only the songs in it.
CREATE TABLE music_playlist_shares (
    id INTEGER PRIMARY KEY,
    playlist_id INTEGER NOT NULL REFERENCES music_playlists(id) ON DELETE CASCADE,
    token TEXT NOT NULL UNIQUE,
    password_hash TEXT,
    expires_at DATETIME,
    view_count INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL
);
CREATE INDEX music_playlist_shares_playlist ON music_playlist_shares(playlist_id);

-- +goose Down
SELECT 1;
