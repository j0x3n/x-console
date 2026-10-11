-- name: ListGoals :many
SELECT * FROM goals ORDER BY year DESC, quarter DESC, sort_order, id;

-- name: ListGoalsByPeriod :many
SELECT * FROM goals WHERE year = ? AND quarter = ? ORDER BY sort_order, id;

-- name: GetGoal :one
SELECT * FROM goals WHERE id = ?;

-- name: InsertGoal :one
INSERT INTO goals (title, year, quarter, note, status, sort_order, created_at, updated_at)
VALUES (?, ?, ?, ?, 'active', ?, ?, ?)
RETURNING *;

-- name: UpdateGoal :one
UPDATE goals SET title = ?, year = ?, quarter = ?, note = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: SetGoalStatus :one
UPDATE goals SET status = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: TouchGoal :exec
UPDATE goals SET updated_at = ? WHERE id = ?;

-- name: DeleteGoal :execrows
DELETE FROM goals WHERE id = ?;

-- name: MaxGoalSortOrder :one
SELECT CAST(coalesce(max(sort_order), 0) AS INTEGER) FROM goals;

-- name: ListGoalResults :many
SELECT * FROM goal_results WHERE goal_id = ? ORDER BY sort_order, id;

-- name: ListAllGoalResults :many
SELECT * FROM goal_results ORDER BY goal_id, sort_order, id;

-- name: GetGoalResult :one
SELECT * FROM goal_results WHERE id = ?;

-- name: InsertGoalResult :one
INSERT INTO goal_results (goal_id, name, target, unit, start_value, source, ref, manual_value, sort_order, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateGoalResult :one
UPDATE goal_results SET name = ?, target = ?, unit = ?, start_value = ?, source = ?, ref = ?, manual_value = ?, sort_order = ?, updated_at = ?
WHERE id = ? AND goal_id = ? RETURNING *;

-- name: DeleteGoalResult :exec
DELETE FROM goal_results WHERE id = ?;

-- name: SetGoalResultValue :one
UPDATE goal_results SET manual_value = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: InsertGoalResultLog :exec
INSERT INTO goal_result_logs (result_id, at, value) VALUES (?, ?, ?);

-- name: LatestGoalResultLog :one
SELECT value FROM goal_result_logs WHERE result_id = ? AND at <= ? ORDER BY at DESC, id DESC LIMIT 1;

-- name: GetGoalReview :one
SELECT * FROM goal_reviews WHERE week_start = ?;

-- name: UpsertGoalReview :one
INSERT INTO goal_reviews (week_start, went_well, fell_short, next_week, snapshot, progress, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(week_start) DO UPDATE SET went_well = excluded.went_well, fell_short = excluded.fell_short,
  next_week = excluded.next_week, snapshot = excluded.snapshot, progress = excluded.progress, updated_at = excluded.updated_at
RETURNING *;

-- name: ListGoalReviews :many
SELECT * FROM goal_reviews ORDER BY week_start DESC LIMIT ?;
