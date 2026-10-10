-- name: ListReadItems :many
-- The list leaves out the body text. has_content says whether one is stored.
SELECT id, url, title, title_locked, site, excerpt, summary, tags_json, note, source, status, error, attempts,
       read_at, created_at, fetched_at, updated_at, kind, meta_json,
       CAST(content <> '' AS INTEGER) AS has_content, CAST(content_html <> '' AS INTEGER) AS has_html
FROM read_items ORDER BY created_at DESC, id DESC;

-- name: GetReadItem :one
SELECT * FROM read_items WHERE id = ?;

-- name: GetReadItemByURL :one
SELECT * FROM read_items WHERE url = ?;

-- name: InsertReadItem :one
INSERT INTO read_items (url, title, site, note, source, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, 'queued', ?, ?) RETURNING *;

-- name: SaveReadFetched :exec
-- An empty title keeps the old one (the user edited it, or the page had none).
UPDATE read_items SET
  title = CASE WHEN title_locked = 1 OR sqlc.arg(title) = '' THEN title ELSE sqlc.arg(title) END,
  site = sqlc.arg(site), excerpt = sqlc.arg(excerpt), content = sqlc.arg(content),
  content_html = sqlc.arg(content_html), kind = sqlc.arg(kind), meta_json = sqlc.arg(meta_json),
  status = 'ready', error = '', attempts = attempts + 1, fetched_at = sqlc.arg(at), updated_at = sqlc.arg(at)
WHERE id = sqlc.arg(id);

-- name: SaveReadFailure :exec
UPDATE read_items SET status = 'failed', error = ?, attempts = attempts + 1, updated_at = ? WHERE id = ?;

-- name: QueueReadItem :exec
UPDATE read_items SET status = 'queued', error = '', updated_at = ? WHERE id = ?;

-- name: ResetReadAttempts :exec
UPDATE read_items SET attempts = 0 WHERE id = ?;

-- name: SaveReadSummary :exec
UPDATE read_items SET summary = ?, tags_json = ?, updated_at = ? WHERE id = ?;

-- name: UpdateReadItem :one
UPDATE read_items SET title = ?, title_locked = ?, tags_json = ?, note = ?, read_at = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: DeleteReadItem :execrows
DELETE FROM read_items WHERE id = ?;

-- name: ListStaleQueuedReadItems :many
-- Queued but never finished (for example after a restart). At most 3 tries.
SELECT id FROM read_items WHERE status = 'queued' AND updated_at < ? AND attempts < 3 ORDER BY id LIMIT 20;

-- name: OldestUnreadReadItems :many
SELECT id, title, url, site FROM read_items WHERE read_at IS NULL ORDER BY created_at ASC, id ASC LIMIT ?;

-- name: CountUnreadReadItems :one
SELECT COUNT(*) FROM read_items WHERE read_at IS NULL;

-- name: InsertReadAsset :exec
INSERT INTO read_assets (item_id, hash, mime, size, src_url, created_at) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(item_id, hash) DO NOTHING;

-- name: ListReadAssets :many
SELECT * FROM read_assets WHERE item_id = ? ORDER BY hash;

-- name: GetReadAsset :one
SELECT * FROM read_assets WHERE item_id = ? AND hash = ?;

-- name: DeleteReadAsset :exec
DELETE FROM read_assets WHERE item_id = ? AND hash = ?;
