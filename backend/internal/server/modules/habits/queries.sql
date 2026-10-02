-- name: CreateHabit :one
INSERT INTO habits (name, icon, color, unit, daily_target, remind_mode, remind_interval_minutes,
                    remind_window, remind_times, ha_entity_id, sort_order, created_at, kind, remind_when, active_host_ids, remind_on_host, template)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetHabit :one
SELECT * FROM habits WHERE id = ?;

-- name: ListHabits :many
SELECT * FROM habits
WHERE CAST(sqlc.arg(include_archived) AS INTEGER) = 1 OR archived_at IS NULL
ORDER BY sort_order, id;

-- name: UpdateHabit :one
UPDATE habits
SET name = ?, icon = ?, color = ?, unit = ?, daily_target = ?, remind_mode = ?,
    remind_interval_minutes = ?, remind_window = ?, remind_times = ?, ha_entity_id = ?,
    archived_at = ?, sort_order = ?, kind = ?, remind_when = ?, active_host_ids = ?, remind_on_host = ?, template = ?, snoozed_until = NULL
WHERE id = ?
RETURNING *;

-- name: DeleteHabit :execrows
DELETE FROM habits WHERE id = ?;

-- name: MarkHabitReminded :exec
UPDATE habits SET last_reminded_at = ?, snoozed_until = NULL WHERE id = ?;

-- name: SetHabitQuietUntil :exec
UPDATE habits SET quiet_until = ? WHERE id = ?;

-- name: MaxHabitSortOrder :one
SELECT CAST(coalesce(max(sort_order), 0) AS INTEGER) FROM habits;

-- name: ListHabitsByEntity :many
SELECT * FROM habits WHERE ha_entity_id = ? AND archived_at IS NULL;

-- name: ListActiveWorkoutHabits :many
SELECT * FROM habits WHERE kind = 'workout' AND archived_at IS NULL ORDER BY id;

-- name: CreateHabitLog :one
INSERT INTO habit_logs (habit_id, at, amount, source, note, workout_log_id)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetHabitLog :one
SELECT * FROM habit_logs WHERE id = ?;

-- name: DeleteHabitLog :execrows
DELETE FROM habit_logs WHERE id = ?;

-- name: DeleteWorkoutCheckins :many
DELETE FROM habit_logs WHERE workout_log_id = ? RETURNING id, habit_id;

-- name: ListHabitLogsSince :many
SELECT * FROM habit_logs WHERE at >= ? ORDER BY at DESC, id DESC;

-- name: ListHabitLogsForHabitSince :many
SELECT * FROM habit_logs WHERE habit_id = ? AND at >= ? ORDER BY at DESC, id DESC;

-- name: ListWorkoutPlans :many
SELECT * FROM workout_plans ORDER BY weekday, id;

-- name: DeleteWorkoutPlans :exec
DELETE FROM workout_plans;

-- name: InsertWorkoutPlan :one
INSERT INTO workout_plans (weekday, title, items) VALUES (?, ?, ?) RETURNING *;

-- name: CreateWorkoutLog :one
INSERT INTO workout_logs (date, plan_id, items, duration_minutes, note, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListWorkoutLogsSince :many
SELECT * FROM workout_logs WHERE date >= ? ORDER BY date DESC, id DESC;

-- name: DeleteWorkoutLog :execrows
DELETE FROM workout_logs WHERE id = ?;

-- name: SetHabitSnoozedUntil :exec
UPDATE habits SET snoozed_until = ? WHERE id = ?;
