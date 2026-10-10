-- name: ItemsOfDay :many
SELECT * FROM journal_items WHERE day = ? ORDER BY at, id;

-- name: DeleteSourceItems :exec
-- Drops what a source reported before for a time range, before the new list
-- is written.
DELETE FROM journal_items WHERE source = ? AND at >= ? AND at < ?;

-- name: UpsertItem :exec
INSERT INTO journal_items (day, at, module, source, ref, kind, title, detail, link, minutes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (source, ref) DO UPDATE SET
  day = excluded.day, at = excluded.at, module = excluded.module, kind = excluded.kind,
  title = excluded.title, detail = excluded.detail, link = excluded.link, minutes = excluded.minutes;

-- name: DayModuleCounts :many
-- Items per day and module since a day, so hidden modules can be left out.
SELECT day, module, COUNT(*) AS n FROM journal_items WHERE day >= ? GROUP BY day, module;

-- name: GetDiary :one
SELECT * FROM journal_diary WHERE day = ?;

-- name: SaveDiary :exec
INSERT INTO journal_diary (day, body, updated_at) VALUES (?, ?, ?)
ON CONFLICT (day) DO UPDATE SET body = excluded.body, updated_at = excluded.updated_at;

-- name: DeleteDiary :exec
DELETE FROM journal_diary WHERE day = ?;

-- name: DiaryDaysSince :many
SELECT day FROM journal_diary WHERE day >= ? ORDER BY day DESC;
