-- name: StartFocus :one
INSERT INTO focus_sessions (issue_key, started_at, planned_minutes, note)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetFocus :one
SELECT * FROM focus_sessions WHERE id = ?;

-- name: CurrentFocus :one
SELECT * FROM focus_sessions WHERE ended_at IS NULL ORDER BY id DESC LIMIT 1;

-- name: ListOpenFocus :many
SELECT * FROM focus_sessions WHERE ended_at IS NULL ORDER BY id;

-- name: StopFocus :one
UPDATE focus_sessions
SET ended_at = ?, actual_seconds = ?, completed = ?, note = ?
WHERE id = ? AND ended_at IS NULL
RETURNING *;

-- name: MarkFocusNotified :execrows
-- Only the first caller wins, so the timer and the sweep never notify twice.
UPDATE focus_sessions SET notified_at = ? WHERE id = ? AND notified_at IS NULL AND ended_at IS NULL;

-- name: ListFocusSince :many
SELECT * FROM focus_sessions WHERE started_at >= ? ORDER BY started_at;

-- name: ListRecentFocus :many
SELECT * FROM focus_sessions WHERE ended_at IS NOT NULL ORDER BY id DESC LIMIT ?;
