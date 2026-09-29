-- ---- scripts ----

-- name: CreateScript :one
INSERT INTO scripts (name, description, shell, body, default_host_ids, timeout_seconds, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetScript :one
SELECT * FROM scripts WHERE id = ?;

-- name: ListScripts :many
SELECT * FROM scripts ORDER BY name COLLATE NOCASE, id;

-- name: UpdateScript :one
UPDATE scripts
SET name = ?, description = ?, shell = ?, body = ?, default_host_ids = ?, timeout_seconds = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteScript :execrows
DELETE FROM scripts WHERE id = ?;

-- name: CreateScriptRun :one
INSERT INTO script_runs (script_id, host_id, host_name, started_at, triggered_by)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: FinishScriptRun :one
UPDATE script_runs
SET finished_at = ?, exit_code = ?, stdout = ?, stderr = ?, error = ?
WHERE id = ?
RETURNING *;

-- name: GetScriptRun :one
SELECT * FROM script_runs WHERE id = ?;

-- name: ListScriptRuns :many
SELECT * FROM script_runs
-- before is the cursor; pass math.MaxInt64 for the first page.
WHERE script_id = sqlc.arg(script_id) AND id < sqlc.arg(before)
ORDER BY id DESC
LIMIT sqlc.arg(lim);

-- name: AbortUnfinishedRuns :execrows
UPDATE script_runs SET finished_at = ?, error = ? WHERE finished_at IS NULL;

-- ---- monitors ----

-- name: CreateMonitor :one
INSERT INTO monitors (kind, name, target, interval_seconds, expected_status, keyword, timeout_ms, enabled, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetMonitor :one
SELECT * FROM monitors WHERE id = ?;

-- name: ListMonitors :many
SELECT * FROM monitors ORDER BY name COLLATE NOCASE, id;

-- name: ListEnabledMonitors :many
SELECT * FROM monitors WHERE enabled = 1 ORDER BY id;

-- name: UpdateMonitor :one
UPDATE monitors
SET name = ?, target = ?, interval_seconds = ?, expected_status = ?, keyword = ?, timeout_ms = ?, enabled = ?
WHERE id = ?
RETURNING *;

-- name: ResetMonitorState :exec
-- The target changed: forget the old state so alerts start fresh.
UPDATE monitors
SET last_status = 'unknown', last_checked_at = NULL, last_error = '', consecutive_failures = 0,
    expires_at = NULL, expiry_notified = '[]'
WHERE id = ?;

-- name: SetMonitorState :one
UPDATE monitors
SET last_status = ?, last_checked_at = ?, last_error = ?, consecutive_failures = ?, expires_at = ?, expiry_notified = ?
WHERE id = ?
RETURNING *;

-- name: DeleteMonitor :execrows
DELETE FROM monitors WHERE id = ?;

-- name: InsertMonitorResult :one
INSERT INTO monitor_results (monitor_id, at, ok, status_code, latency_ms, error, detail)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListMonitorResults :many
SELECT * FROM monitor_results WHERE monitor_id = ? AND at >= ? ORDER BY at, id;

-- name: DeleteMonitorResultsBefore :execrows
DELETE FROM monitor_results WHERE at < ?;

-- ---- subscriptions ----

-- name: CreateSubscription :one
INSERT INTO subscriptions (name, category, category_id, amount, currency, cycle, cycle_days, cycle_count, cycle_unit,
                           next_renewal, remind_days_before, url, note, auto_renew, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSubscription :one
SELECT * FROM subscriptions WHERE id = ?;

-- name: ListSubscriptions :many
SELECT * FROM subscriptions ORDER BY next_renewal, id;

-- name: UpdateSubscription :one
UPDATE subscriptions
SET name = ?, category = ?, category_id = ?, amount = ?, currency = ?, cycle = ?, cycle_days = ?, cycle_count = ?,
    cycle_unit = ?, next_renewal = ?,
    remind_days_before = ?, reminded = ?, url = ?, note = ?, auto_renew = ?, archived_at = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteSubscription :execrows
DELETE FROM subscriptions WHERE id = ?;

-- name: InsertSubscriptionEvent :one
INSERT INTO subscription_events (subscription_id, at, kind, detail)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: ListSubscriptionEvents :many
SELECT * FROM subscription_events WHERE subscription_id = ? ORDER BY id DESC LIMIT 100;

-- ---- subscription categories ----

-- name: ListSubscriptionCategories :many
SELECT c.id, c.name, c.builtin, c.position,
       (SELECT COUNT(*) FROM subscriptions s WHERE s.category_id = c.id AND s.archived_at IS NULL) AS count
FROM subscription_categories c
ORDER BY c.position, c.id;

-- name: GetSubscriptionCategory :one
SELECT * FROM subscription_categories WHERE id = ?;

-- name: GetSubscriptionCategoryByBuiltin :one
SELECT * FROM subscription_categories WHERE builtin = ?;

-- name: NextSubscriptionCategoryPosition :one
SELECT CAST(COALESCE(MAX(position), -1) + 1 AS INTEGER) FROM subscription_categories;

-- name: CreateSubscriptionCategory :one
INSERT INTO subscription_categories (name, position, created_at) VALUES (?, ?, ?)
RETURNING *;

-- name: UpdateSubscriptionCategory :one
UPDATE subscription_categories SET name = ?, position = ? WHERE id = ?
RETURNING *;

-- name: MoveSubscriptionsToCategory :exec
UPDATE subscriptions SET category_id = ?, category = ?, updated_at = ? WHERE category_id = ?;

-- name: DeleteSubscriptionCategory :execrows
DELETE FROM subscription_categories WHERE id = ?;
