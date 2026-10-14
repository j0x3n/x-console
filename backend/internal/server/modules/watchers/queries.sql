-- name: ListWatchers :many
SELECT * FROM watchers ORDER BY id;

-- name: GetWatcher :one
SELECT * FROM watchers WHERE id = ?;

-- name: CreateWatcher :one
INSERT INTO watchers (kind, name, url, goal, allowed_hosts, interval_minutes, keepalive_minutes, mode,
  login_sealed, proxy_sealed, next_check_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateWatcherSettings :exec
UPDATE watchers SET name = ?, url = ?, goal = ?, allowed_hosts = ?, interval_minutes = ?, keepalive_minutes = ?,
  mode = ?, enabled = ?, login_sealed = ?, proxy_sealed = ?, state_sealed = ?, recipe = ?, next_check_at = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteWatcher :exec
DELETE FROM watchers WHERE id = ?;

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

-- name: CreateChange :exec
INSERT INTO watcher_changes (watcher_id, old_value, new_value, summary, important, shot_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListChanges :many
SELECT * FROM watcher_changes WHERE watcher_id = ? ORDER BY id DESC LIMIT 50;
