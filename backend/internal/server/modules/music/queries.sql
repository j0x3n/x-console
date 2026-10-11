-- name: GetTrack :one
SELECT * FROM music_tracks WHERE id = ? LIMIT 1;

-- name: GetTrackByItem :one
SELECT * FROM music_tracks WHERE drive_item_id = ? LIMIT 1;

-- name: ListTrackSync :many
SELECT id, drive_item_id, sha256, companion FROM music_tracks;

-- name: InsertTrack :one
INSERT INTO music_tracks (drive_item_id, sha256, companion, title, artist, album, album_artist, track_no, disc_no, year,
  duration_ms, bitrate, format, has_cover, cover_source, cover_key, lyrics_source, lyrics_synced, lyrics_text, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateTrackScan :exec
UPDATE music_tracks SET sha256 = ?, companion = ?, title = ?, artist = ?, album = ?, album_artist = ?, track_no = ?, disc_no = ?, year = ?,
  duration_ms = ?, bitrate = ?, format = ?, has_cover = ?, cover_source = ?, cover_key = ?, lyrics_source = ?, lyrics_synced = ?,
  lyrics_text = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteTrack :exec
DELETE FROM music_tracks WHERE id = ?;

-- name: SetTrackFavorite :exec
UPDATE music_tracks SET favorite = ?, updated_at = ? WHERE id = ?;

-- name: SetTrackManual :exec
UPDATE music_tracks SET title = ?, artist = ?, album = ?, album_artist = ?, manual = 1, updated_at = ? WHERE id = ?;

-- name: MarkTrackPlayed :exec
UPDATE music_tracks SET play_count = play_count + 1, last_played_at = ? WHERE id = ?;

-- name: ListAlbums :many
SELECT album,
  CAST(CASE WHEN album_artist <> '' THEN album_artist ELSE artist END AS TEXT) AS album_artist,
  CAST(MAX(year) AS INTEGER) AS year,
  CAST(COUNT(*) AS INTEGER) AS track_count,
  CAST(COALESCE(SUM(duration_ms), 0) AS INTEGER) AS duration_ms,
  CAST(COALESCE(MIN(CASE WHEN has_cover = 1 THEN id END), 0) AS INTEGER) AS cover_track_id
FROM music_tracks
GROUP BY album, CASE WHEN album_artist <> '' THEN album_artist ELSE artist END
ORDER BY album COLLATE NOCASE, 2 COLLATE NOCASE;

-- name: ListArtists :many
SELECT artist,
  CAST(COUNT(*) AS INTEGER) AS track_count,
  CAST(COUNT(DISTINCT album) AS INTEGER) AS album_count,
  CAST(COALESCE(MIN(CASE WHEN has_cover = 1 THEN id END), 0) AS INTEGER) AS cover_track_id
FROM music_tracks
GROUP BY artist
ORDER BY artist COLLATE NOCASE;

-- name: ListPlaylists :many
SELECT p.id, p.name, p.created_at, p.updated_at,
  CAST((SELECT COUNT(*) FROM music_playlist_items i WHERE i.playlist_id = p.id) AS INTEGER) AS track_count,
  CAST((SELECT COALESCE(SUM(t.duration_ms), 0) FROM music_playlist_items i JOIN music_tracks t ON t.id = i.track_id WHERE i.playlist_id = p.id) AS INTEGER) AS duration_ms
FROM music_playlists p
ORDER BY p.updated_at DESC, p.id DESC;

-- name: GetPlaylist :one
SELECT p.id, p.name, p.created_at, p.updated_at,
  CAST((SELECT COUNT(*) FROM music_playlist_items i WHERE i.playlist_id = p.id) AS INTEGER) AS track_count,
  CAST((SELECT COALESCE(SUM(t.duration_ms), 0) FROM music_playlist_items i JOIN music_tracks t ON t.id = i.track_id WHERE i.playlist_id = p.id) AS INTEGER) AS duration_ms
FROM music_playlists p WHERE p.id = ? LIMIT 1;

-- name: InsertPlaylist :one
INSERT INTO music_playlists (name, created_at, updated_at) VALUES (?, ?, ?) RETURNING id;

-- name: RenamePlaylist :exec
UPDATE music_playlists SET name = ?, updated_at = ? WHERE id = ?;

-- name: TouchPlaylist :exec
UPDATE music_playlists SET updated_at = ? WHERE id = ?;

-- name: DeletePlaylist :exec
DELETE FROM music_playlists WHERE id = ?;

-- name: ListPlaylistTracks :many
SELECT t.* FROM music_playlist_items i JOIN music_tracks t ON t.id = i.track_id
WHERE i.playlist_id = ? ORDER BY i.position;

-- name: ListPlaylistCovers :many
SELECT t.id FROM music_playlist_items i JOIN music_tracks t ON t.id = i.track_id
WHERE i.playlist_id = ? AND t.has_cover = 1 ORDER BY i.position LIMIT 4;

-- name: ClearPlaylistItems :exec
DELETE FROM music_playlist_items WHERE playlist_id = ?;

-- name: AddPlaylistItem :exec
INSERT OR IGNORE INTO music_playlist_items (playlist_id, track_id, position) VALUES (?, ?, ?);

-- name: NextPlaylistPosition :one
SELECT CAST(COALESCE(MAX(position), -1) + 1 AS INTEGER) FROM music_playlist_items WHERE playlist_id = ?;

-- name: CountTracksByIDs :one
SELECT CAST(COUNT(*) AS INTEGER) FROM music_tracks WHERE id IN (sqlc.slice('ids'));

-- name: ClearCandidates :exec
DELETE FROM music_candidates WHERE track_id = ?;

-- name: InsertCandidate :exec
INSERT OR REPLACE INTO music_candidates (track_id, source, source_id, title, artist, album, duration_ms, has_lyrics, has_cover, lyrics_text, cover_ref, position)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListCandidates :many
SELECT * FROM music_candidates WHERE track_id = ? ORDER BY position;

-- name: GetCandidate :one
SELECT * FROM music_candidates WHERE track_id = ? AND source = ? AND source_id = ? LIMIT 1;

-- name: ListPendingTracks :many
SELECT * FROM music_tracks WHERE match_state = 'pending' ORDER BY updated_at DESC, id DESC;

-- name: CountPendingTracks :one
SELECT CAST(COUNT(*) AS INTEGER) FROM music_tracks WHERE match_state = 'pending';

-- name: SetMatchState :exec
UPDATE music_tracks SET match_state = ?, updated_at = ? WHERE id = ?;

-- name: ListTracksToMatch :many
SELECT * FROM music_tracks
WHERE (match_state = 'none' OR (match_state = 'failed' AND sqlc.arg(retry_failed) = 1))
  AND (lyrics_source = 'none' OR cover_source = 'none')
ORDER BY id LIMIT sqlc.arg(max_rows);

-- name: CountTracksToMatch :one
SELECT CAST(COUNT(*) AS INTEGER) FROM music_tracks
WHERE (match_state = 'none' OR (match_state = 'failed' AND sqlc.arg(retry_failed) = 1))
  AND (lyrics_source = 'none' OR cover_source = 'none');

-- name: SetTrackLyrics :exec
UPDATE music_tracks SET lyrics_text = ?, lyrics_source = ?, lyrics_synced = ?, updated_at = ? WHERE id = ?;

-- name: SetTrackCover :exec
UPDATE music_tracks SET cover_key = ?, has_cover = 1, cover_source = ?, updated_at = ? WHERE id = ?;

-- name: SetTrackSha :exec
UPDATE music_tracks SET sha256 = ? WHERE id = ?;

-- name: InsertShare :one
INSERT INTO music_playlist_shares (playlist_id, token, password_hash, expires_at, created_at) VALUES (?, ?, ?, ?, ?) RETURNING *;

-- name: ListShares :many
SELECT * FROM music_playlist_shares WHERE playlist_id = ? ORDER BY id DESC;

-- name: GetShareByToken :one
SELECT * FROM music_playlist_shares WHERE token = ? LIMIT 1;

-- name: DeleteShare :execrows
DELETE FROM music_playlist_shares WHERE id = ?;

-- name: BumpShareViews :exec
UPDATE music_playlist_shares SET view_count = view_count + 1 WHERE id = ?;

-- name: PlaylistHasTrack :one
SELECT CAST(COUNT(*) AS INTEGER) FROM music_playlist_items WHERE playlist_id = ? AND track_id = ?;

-- name: InsertPlay :exec
INSERT INTO music_plays (track_id, title, artist, played_at, seconds, focus_session_id) VALUES (?, ?, ?, ?, ?, ?);
