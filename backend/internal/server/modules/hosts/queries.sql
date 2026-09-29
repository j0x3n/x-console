-- name: UpsertMetric1m :exec
INSERT INTO host_metrics_1m (host_id, at, cpu, mem_used, mem_total, disk_json, net_rx, net_tx, load1)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (host_id, at) DO UPDATE SET
    cpu = excluded.cpu, mem_used = excluded.mem_used, mem_total = excluded.mem_total,
    disk_json = excluded.disk_json, net_rx = excluded.net_rx, net_tx = excluded.net_tx, load1 = excluded.load1;

-- name: UpsertMetric1h :exec
INSERT INTO host_metrics_1h (host_id, at, cpu, mem_used, mem_total, disk_json, net_rx, net_tx, load1)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (host_id, at) DO UPDATE SET
    cpu = excluded.cpu, mem_used = excluded.mem_used, mem_total = excluded.mem_total,
    disk_json = excluded.disk_json, net_rx = excluded.net_rx, net_tx = excluded.net_tx, load1 = excluded.load1;

-- name: ListMetrics1m :many
SELECT * FROM host_metrics_1m
WHERE host_id = sqlc.arg(host_id) AND at >= sqlc.arg(since) AND at < sqlc.arg(until)
ORDER BY at;

-- name: ListMetrics1mWindow :many
SELECT * FROM host_metrics_1m WHERE at >= sqlc.arg(since) AND at < sqlc.arg(until) ORDER BY host_id, at;

-- name: ListMetrics1h :many
SELECT * FROM host_metrics_1h
WHERE host_id = sqlc.arg(host_id) AND at >= sqlc.arg(since) AND at < sqlc.arg(until)
ORDER BY at;

-- name: DeleteMetrics1mBefore :exec
DELETE FROM host_metrics_1m WHERE at < ?;

-- name: DeleteMetrics1hBefore :exec
DELETE FROM host_metrics_1h WHERE at < ?;

-- name: ListSSHHosts :many
SELECT * FROM ssh_hosts ORDER BY name, id;

-- name: GetSSHHost :one
SELECT * FROM ssh_hosts WHERE id = ?;

-- name: CreateSSHHost :one
INSERT INTO ssh_hosts (name, address, port, username, auth, secret, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateSSHHost :one
UPDATE ssh_hosts SET name = ?, address = ?, port = ?, username = ?, auth = ?, secret = ?, host_key = ?
WHERE id = ?
RETURNING *;

-- name: SetSSHHostKey :exec
UPDATE ssh_hosts SET host_key = ? WHERE id = ?;

-- name: DeleteSSHHost :execrows
DELETE FROM ssh_hosts WHERE id = ?;

-- name: ListAlertRules :many
SELECT * FROM alert_rules ORDER BY id;

-- name: ListEnabledAlertRules :many
SELECT * FROM alert_rules WHERE enabled = 1 ORDER BY id;

-- name: GetAlertRule :one
SELECT * FROM alert_rules WHERE id = ?;

-- name: CreateAlertRule :one
INSERT INTO alert_rules (host_id, metric, op, threshold, duration_seconds, severity, enabled, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateAlertRule :one
UPDATE alert_rules SET host_id = ?, metric = ?, op = ?, threshold = ?, duration_seconds = ?, severity = ?, enabled = ?
WHERE id = ?
RETURNING *;

-- name: DeleteAlertRule :execrows
DELETE FROM alert_rules WHERE id = ?;

-- name: InsertAlertEvent :one
INSERT INTO alert_events (rule_id, host_id, host_name, metric, severity, value, message, fired_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ResolveAlertEvent :one
UPDATE alert_events SET resolved_at = ? WHERE id = ? AND resolved_at IS NULL
RETURNING *;

-- name: ListOpenAlertEvents :many
SELECT * FROM alert_events WHERE resolved_at IS NULL ORDER BY id;

-- name: ListAlertEvents :many
SELECT * FROM alert_events
WHERE fired_at >= sqlc.arg(since)
  AND (sqlc.narg(host_id) IS NULL OR host_id = sqlc.narg(host_id))
  AND (sqlc.arg(open_only) = 0 OR resolved_at IS NULL)
ORDER BY fired_at DESC, id DESC
LIMIT sqlc.arg(lim);

-- name: DeleteAlertEventsBefore :exec
DELETE FROM alert_events WHERE fired_at < ? AND resolved_at IS NOT NULL;

-- name: AddTrafficHourly :exec
INSERT INTO host_traffic_hourly (host_id, hour, rx, tx, estimated) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (host_id, hour) DO UPDATE SET
    rx = rx + excluded.rx, tx = tx + excluded.tx, estimated = MAX(estimated, excluded.estimated);

-- name: AddTrafficDaily :exec
INSERT INTO host_traffic_daily (host_id, day, rx, tx, estimated) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (host_id, day) DO UPDATE SET
    rx = rx + excluded.rx, tx = tx + excluded.tx, estimated = MAX(estimated, excluded.estimated);

-- name: ListTrafficDays :many
SELECT * FROM host_traffic_daily
WHERE host_id = sqlc.arg(host_id) AND day >= sqlc.arg(from_day) AND day <= sqlc.arg(to_day)
ORDER BY day;

-- name: DeleteTrafficHourlyBefore :exec
DELETE FROM host_traffic_hourly WHERE hour < ?;

-- name: DeleteTrafficHostHourly :exec
DELETE FROM host_traffic_hourly WHERE host_id = ?;

-- name: DeleteTrafficHostDaily :exec
DELETE FROM host_traffic_daily WHERE host_id = ?;

-- name: DeleteTrafficPlan :exec
DELETE FROM host_traffic_plans WHERE host_id = ?;

-- name: GetTrafficPlan :one
SELECT * FROM host_traffic_plans WHERE host_id = ?;

-- name: ListTrafficPlansWithLimit :many
SELECT * FROM host_traffic_plans WHERE limit_bytes > 0;

-- name: UpsertTrafficPlan :exec
INSERT INTO host_traffic_plans (host_id, start_day, period_months, limit_bytes, count_mode, alert_percent, alerted_cycle, alerted_level, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (host_id) DO UPDATE SET
    start_day = excluded.start_day, period_months = excluded.period_months, limit_bytes = excluded.limit_bytes,
    count_mode = excluded.count_mode, alert_percent = excluded.alert_percent,
    alerted_cycle = excluded.alerted_cycle, alerted_level = excluded.alerted_level, updated_at = excluded.updated_at;

-- name: SetTrafficAlerted :exec
UPDATE host_traffic_plans SET alerted_cycle = ?, alerted_level = ? WHERE host_id = ?;
