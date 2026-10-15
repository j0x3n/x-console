-- name: ListWatchers :many
SELECT * FROM watchers ORDER BY id;

-- name: GetWatcher :one
SELECT * FROM watchers WHERE id = ?;

-- name: CreateWatcher :one
INSERT INTO watchers (kind, name, url, goal, allowed_hosts, interval_minutes, keepalive_minutes, mode,
  login_sealed, proxy_sealed, egress_agent_id, next_check_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateWatcherSettings :exec
UPDATE watchers SET name = ?, url = ?, goal = ?, allowed_hosts = ?, interval_minutes = ?, keepalive_minutes = ?,
  mode = ?, enabled = ?, login_sealed = ?, proxy_sealed = ?, egress_agent_id = ?, state_sealed = ?, recipe = ?, next_check_at = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteWatcher :exec
DELETE FROM watchers WHERE id = ?;

-- name: SetWatcherNextCheck :exec
UPDATE watchers SET status_detail = ?, next_check_at = ?, updated_at = ? WHERE id = ?;

-- name: SetWatcherStatus :exec
UPDATE watchers SET status = ?, status_detail = ?, updated_at = ? WHERE id = ?;

-- name: SetWatcherPending :exec
UPDATE watchers SET pending = ?, updated_at = ? WHERE id = ?;

-- name: SetWatcherAsks :exec
UPDATE watchers SET asks_day = ?, asks_count = ? WHERE id = ?;

-- name: FinishWatcherCheck :exec
UPDATE watchers SET status = ?, status_detail = ?, current_value = ?, fail_count = ?, pending = '',
  last_checked_at = ?, next_check_at = ?, last_change_at = ?, updated_at = ?
WHERE id = ?;

-- name: SetWatcherRecipe :exec
UPDATE watchers SET recipe = ?, updated_at = ? WHERE id = ?;

-- name: SetWatcherState :exec
UPDATE watchers SET state_sealed = ? WHERE id = ?;

-- name: SetWatcherKeepalive :exec
UPDATE watchers SET last_keepalive_at = ? WHERE id = ?;

-- name: ResetBusyWatchers :exec
UPDATE watchers SET status = 'error', status_detail = ?, pending = ''
WHERE status IN ('exploring', 'checking', 'waiting');

-- name: CreateRun :one
INSERT INTO watcher_runs (watcher_id, kind, started_at) VALUES (?, ?, ?) RETURNING id;

-- name: UpdateRun :exec
UPDATE watcher_runs SET outcome = ?, detail = ?, steps = ?, tokens = ?, finished_at = ? WHERE id = ?;

-- name: ListRuns :many
SELECT * FROM watcher_runs WHERE watcher_id = ? ORDER BY id DESC LIMIT 20;

-- name: FinishStaleRuns :exec
UPDATE watcher_runs SET outcome = 'failed', detail = ?, finished_at = ? WHERE outcome = 'running';

-- name: CreateShot :one
INSERT INTO watcher_shots (watcher_id, data, created_at) VALUES (?, ?, ?) RETURNING id;

-- name: GetShot :one
SELECT data FROM watcher_shots WHERE id = ?;

-- name: CreateChange :one
INSERT INTO watcher_changes (watcher_id, old_value, new_value, summary, important, shot_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: GetChange :one
SELECT * FROM watcher_changes WHERE id = ?;

-- name: ListChanges :many
SELECT * FROM watcher_changes WHERE watcher_id = ? ORDER BY id DESC LIMIT 50;

-- name: LastChanges :many
SELECT watcher_id, summary, important, created_at FROM watcher_changes
WHERE id IN (SELECT MAX(id) FROM watcher_changes GROUP BY watcher_id);

-- name: LastChange :one
SELECT summary, important, created_at FROM watcher_changes WHERE watcher_id = ? ORDER BY id DESC LIMIT 1;

-- name: ListRunsPage :many
SELECT * FROM watcher_runs WHERE watcher_id = ? AND id < ? ORDER BY id DESC LIMIT ?;

-- name: GetRun :one
SELECT * FROM watcher_runs WHERE id = ?;

-- name: ShotsUsage :one
SELECT COUNT(*) AS count, CAST(COALESCE(SUM(length(data)), 0) AS INTEGER) AS bytes FROM watcher_shots WHERE watcher_id = ?;

-- name: ListShotSizes :many
SELECT id, CAST(length(data) AS INTEGER) AS size, created_at FROM watcher_shots WHERE watcher_id = ?;

-- name: ListRunSteps :many
SELECT id, outcome, started_at, steps FROM watcher_runs WHERE watcher_id = ? ORDER BY id DESC;

-- name: ListChangeShots :many
SELECT shot_id FROM watcher_changes WHERE watcher_id = ? AND shot_id IS NOT NULL;

-- name: ClearChangeShot :exec
UPDATE watcher_changes SET shot_id = NULL WHERE shot_id = ?;

-- name: DeleteShot :exec
DELETE FROM watcher_shots WHERE id = ? AND watcher_id = ?;

-- name: SetRunSteps :exec
UPDATE watcher_runs SET steps = ? WHERE id = ? AND outcome != 'running';
