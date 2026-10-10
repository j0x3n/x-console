-- name: SaveScreenMinute :exec
INSERT INTO screen_minutes (host_id, minute, app, category, title) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(host_id, minute) DO UPDATE SET app = excluded.app, category = excluded.category, title = excluded.title;

-- name: ScreenCategoryTotals :many
SELECT category, COUNT(*) AS minutes FROM screen_minutes
WHERE minute >= sqlc.arg(from_minute) AND minute < sqlc.arg(to_minute) AND (sqlc.arg(host) = '' OR host_id = sqlc.arg(host))
GROUP BY category;

-- name: ScreenAppTotals :many
SELECT app, category, COUNT(*) AS minutes FROM screen_minutes
WHERE minute >= sqlc.arg(from_minute) AND minute < sqlc.arg(to_minute) AND (sqlc.arg(host) = '' OR host_id = sqlc.arg(host))
GROUP BY app, category;

-- name: CountScreenMinutes :one
SELECT COUNT(*) FROM screen_minutes;

-- name: ScreenDistinctApps :many
SELECT DISTINCT app, title FROM screen_minutes;

-- name: ReclassifyScreenMinutes :exec
UPDATE screen_minutes SET category = ? WHERE app = ? AND title = ? AND category <> ?;

-- name: DeleteScreenMinutesBefore :exec
DELETE FROM screen_minutes WHERE minute < ?;

-- name: ClearScreenTitlesBefore :exec
UPDATE screen_minutes SET title = '' WHERE title <> '' AND minute < ?;

-- name: ClearScreenTitles :exec
UPDATE screen_minutes SET title = '' WHERE title <> '';

-- name: DeleteAllScreenMinutes :exec
DELETE FROM screen_minutes;

-- name: ListScreenRules :many
SELECT * FROM screen_rules ORDER BY id;

-- name: InsertScreenRule :one
INSERT INTO screen_rules (field, pattern, category, created_at) VALUES (?, ?, ?, ?) RETURNING *;

-- name: DeleteScreenRule :execrows
DELETE FROM screen_rules WHERE id = ?;
