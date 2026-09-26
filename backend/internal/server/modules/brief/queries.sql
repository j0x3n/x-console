-- name: UpsertBrief :one
INSERT INTO briefs (date, content, sections, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (date) DO UPDATE SET content = excluded.content, sections = excluded.sections, created_at = excluded.created_at
RETURNING *;

-- name: MarkBriefSent :one
UPDATE briefs SET sent_at = ? WHERE id = ?
RETURNING *;

-- name: GetBriefByDate :one
SELECT * FROM briefs WHERE date = ?;

-- name: ListBriefs :many
-- Newest first. before is the last id of the previous page, or a huge number.
SELECT * FROM briefs
WHERE id < sqlc.arg(before)
ORDER BY id DESC
LIMIT sqlc.arg(lim);
