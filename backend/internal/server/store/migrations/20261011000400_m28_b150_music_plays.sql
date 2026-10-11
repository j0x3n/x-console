-- +goose Up
-- B150: one row for every time a song was listened to (30 seconds or half of it).
-- The title and artist are copied so the statistics survive a song leaving the library.
CREATE TABLE music_plays (
    id INTEGER PRIMARY KEY,
    track_id INTEGER REFERENCES music_tracks(id) ON DELETE SET NULL,
    title TEXT NOT NULL DEFAULT '',
    artist TEXT NOT NULL DEFAULT '',
    played_at DATETIME NOT NULL,
    seconds INTEGER NOT NULL DEFAULT 0,
    focus_session_id INTEGER
);
CREATE INDEX music_plays_time ON music_plays(played_at);
CREATE INDEX music_plays_focus ON music_plays(focus_session_id) WHERE focus_session_id IS NOT NULL;

-- +goose Down
SELECT 1;
