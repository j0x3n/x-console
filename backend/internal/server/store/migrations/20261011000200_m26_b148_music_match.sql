-- +goose Up
-- B148: songs the online match could not decide on keep their candidates here
-- until the user picks one or skips.
CREATE TABLE music_candidates (
    id INTEGER PRIMARY KEY,
    track_id INTEGER NOT NULL REFERENCES music_tracks(id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    source_id TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    artist TEXT NOT NULL DEFAULT '',
    album TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    has_lyrics INTEGER NOT NULL DEFAULT 0,
    has_cover INTEGER NOT NULL DEFAULT 0,
    lyrics_text TEXT NOT NULL DEFAULT '',
    cover_ref TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL DEFAULT 0,
    UNIQUE (track_id, source, source_id)
);
CREATE INDEX music_candidates_track ON music_candidates(track_id, position);
CREATE INDEX music_tracks_match ON music_tracks(match_state);

-- +goose Down
SELECT 1;
