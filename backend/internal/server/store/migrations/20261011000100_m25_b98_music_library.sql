-- +goose Up
-- B98: the music library. Files stay in the drive; this only indexes them.
CREATE TABLE music_tracks (
    id INTEGER PRIMARY KEY,
    drive_item_id INTEGER NOT NULL UNIQUE REFERENCES drive_items(id) ON DELETE CASCADE,
    -- sha256 of the content and the sibling files (.lrc, cover image) the
    -- row was built from. A scan reads the file again only when one changes.
    sha256 TEXT NOT NULL DEFAULT '',
    companion TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    artist TEXT NOT NULL DEFAULT '',
    album TEXT NOT NULL DEFAULT '',
    album_artist TEXT NOT NULL DEFAULT '',
    -- manual = 1: the user edited title, artist or album, so rescans keep them.
    manual INTEGER NOT NULL DEFAULT 0,
    track_no INTEGER NOT NULL DEFAULT 0,
    disc_no INTEGER NOT NULL DEFAULT 0,
    year INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    bitrate INTEGER NOT NULL DEFAULT 0,
    format TEXT NOT NULL DEFAULT '',
    has_cover INTEGER NOT NULL DEFAULT 0,
    cover_source TEXT NOT NULL DEFAULT 'none',
    cover_key TEXT NOT NULL DEFAULT '',
    lyrics_source TEXT NOT NULL DEFAULT 'none',
    lyrics_synced INTEGER NOT NULL DEFAULT 0,
    lyrics_text TEXT NOT NULL DEFAULT '',
    favorite INTEGER NOT NULL DEFAULT 0,
    play_count INTEGER NOT NULL DEFAULT 0,
    last_played_at DATETIME,
    match_state TEXT NOT NULL DEFAULT 'none',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CHECK (has_cover IN (0, 1)),
    CHECK (lyrics_synced IN (0, 1)),
    CHECK (favorite IN (0, 1)),
    CHECK (manual IN (0, 1))
);
CREATE INDEX music_tracks_artist ON music_tracks(artist, album, disc_no, track_no, title);
CREATE INDEX music_tracks_album ON music_tracks(album);
CREATE INDEX music_tracks_played ON music_tracks(last_played_at) WHERE last_played_at IS NOT NULL;
CREATE INDEX music_tracks_favorite ON music_tracks(favorite) WHERE favorite = 1;

CREATE TABLE music_playlists (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CHECK (name <> '')
);

CREATE TABLE music_playlist_items (
    playlist_id INTEGER NOT NULL REFERENCES music_playlists(id) ON DELETE CASCADE,
    track_id INTEGER NOT NULL REFERENCES music_tracks(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    PRIMARY KEY (playlist_id, track_id)
);
CREATE INDEX music_playlist_items_order ON music_playlist_items(playlist_id, position);

-- Search. The trigram tokenizer matches any substring of three or more
-- characters, Chinese included. Shorter words are matched with LIKE on the
-- title, artist and album only (see modules/music/tracks.go).
CREATE VIRTUAL TABLE music_fts USING fts5 (
    title, artist, album, lyrics_text,
    content = 'music_tracks', content_rowid = 'id',
    tokenize = 'trigram'
);

-- +goose StatementBegin
CREATE TRIGGER music_fts_insert AFTER INSERT ON music_tracks BEGIN
    INSERT INTO music_fts (rowid, title, artist, album, lyrics_text) VALUES (new.id, new.title, new.artist, new.album, new.lyrics_text);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER music_fts_delete AFTER DELETE ON music_tracks BEGIN
    INSERT INTO music_fts (music_fts, rowid, title, artist, album, lyrics_text) VALUES ('delete', old.id, old.title, old.artist, old.album, old.lyrics_text);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER music_fts_update AFTER UPDATE OF title, artist, album, lyrics_text ON music_tracks BEGIN
    INSERT INTO music_fts (music_fts, rowid, title, artist, album, lyrics_text) VALUES ('delete', old.id, old.title, old.artist, old.album, old.lyrics_text);
    INSERT INTO music_fts (rowid, title, artist, album, lyrics_text) VALUES (new.id, new.title, new.artist, new.album, new.lyrics_text);
END;
-- +goose StatementEnd

-- +goose Down
SELECT 1;
