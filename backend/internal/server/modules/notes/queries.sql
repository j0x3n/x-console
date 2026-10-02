-- M6 notes queries. Full-text search (FTS5 MATCH) and the LIKE fallback live in
-- search.go (database/sql). Keep this file ASCII only: sqlc miscounts offsets
-- after multi-byte characters.

-- name: ListNotes :many
SELECT * FROM notes
WHERE (archived_at IS NOT NULL) = CAST(sqlc.arg(archived) AS BOOLEAN)
  AND hidden = sqlc.arg(hidden)
  AND (sqlc.narg(kind) IS NULL OR kind = sqlc.narg(kind))
  AND (sqlc.narg(pinned) IS NULL OR pinned = sqlc.narg(pinned))
  AND (sqlc.narg(tag) IS NULL OR id IN (SELECT note_id FROM note_tags WHERE note_tags.tag = sqlc.narg(tag)))
ORDER BY pinned DESC, updated_at DESC, id DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetNote :one
SELECT * FROM notes WHERE id = ?;

-- name: CreateNote :one
INSERT INTO notes (title, body, pinned, hidden, created_at, updated_at, kind, color) VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateNote :exec
UPDATE notes SET title = ?, body = ?, pinned = ?, archived_at = ?, hidden = ?, updated_at = ?, kind = ?, color = ? WHERE id = ?;

-- name: DeleteNote :execrows
DELETE FROM notes WHERE id = ?;

-- name: ListNoteTags :many
SELECT tag FROM note_tags WHERE note_id = ? ORDER BY tag;

-- name: ListTagsForNotes :many
SELECT note_id, tag FROM note_tags WHERE note_id IN (sqlc.slice(ids)) ORDER BY tag;

-- name: ClearNoteTags :exec
DELETE FROM note_tags WHERE note_id = ?;

-- name: AddNoteTag :exec
INSERT OR IGNORE INTO note_tags (note_id, tag) VALUES (?, ?);

-- name: SetSuggestedTags :exec
UPDATE notes SET suggested_tags = ? WHERE id = ?;

-- name: TagCounts :many
SELECT note_tags.tag, count(*) AS count, CAST(sum(CASE WHEN notes.kind = 'memo' THEN 1 ELSE 0 END) AS INTEGER) AS memo_count, CAST(coalesce(max(note_tag_colors.color), '') AS TEXT) AS color
FROM note_tags
JOIN notes ON notes.id = note_tags.note_id
LEFT JOIN note_tag_colors ON note_tag_colors.tag = note_tags.tag
WHERE notes.archived_at IS NULL AND notes.hidden = sqlc.arg(hidden)
GROUP BY note_tags.tag
ORDER BY count(*) DESC, note_tags.tag;

-- name: SetTagColor :exec
INSERT INTO note_tag_colors (tag, color) VALUES (?, ?)
ON CONFLICT (tag) DO UPDATE SET color = excluded.color;

-- name: ClearTagColor :exec
DELETE FROM note_tag_colors WHERE tag = ?;

-- name: DeleteNoteShare :execrows
DELETE FROM note_shares WHERE note_id = ?;
