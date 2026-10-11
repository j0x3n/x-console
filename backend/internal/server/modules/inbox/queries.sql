-- name: InsertInboxItem :one
INSERT INTO inbox_items (text, source, status, suggest_state, suggestion, created_at, updated_at)
VALUES (?, ?, 'open', ?, '', ?, ?) RETURNING *;

-- name: GetInboxItem :one
SELECT * FROM inbox_items WHERE id = ?;

-- name: ListOpenInboxItems :many
SELECT * FROM inbox_items WHERE status = 'open' ORDER BY created_at, id;

-- name: ListClosedInboxItems :many
SELECT * FROM inbox_items WHERE status <> 'open' ORDER BY closed_at DESC, id DESC LIMIT 100;

-- name: CountOpenInboxItems :one
SELECT count(*) FROM inbox_items WHERE status = 'open';

-- name: UpdateInboxText :one
UPDATE inbox_items SET text = ?, suggest_state = ?, suggestion = '', updated_at = ?
WHERE id = ? AND status = 'open' RETURNING *;

-- name: SetInboxSuggestion :exec
UPDATE inbox_items SET suggest_state = ?, suggestion = ?, updated_at = ?
WHERE id = ? AND status = 'open';

-- name: CloseInboxItem :one
UPDATE inbox_items SET status = ?, filed_kind = ?, filed_ref = ?, closed_at = ?, updated_at = ?
WHERE id = ? AND status = 'open' RETURNING *;

-- name: DeleteInboxItem :execrows
DELETE FROM inbox_items WHERE id = ?;

-- name: PruneClosedInboxItems :exec
DELETE FROM inbox_items WHERE status <> 'open' AND closed_at < ?;
