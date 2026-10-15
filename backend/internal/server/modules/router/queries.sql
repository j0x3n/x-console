-- name: InsertTraffic :exec
INSERT INTO router_traffic (at, seconds, rx, tx) VALUES (?, ?, ?, ?)
-- B114: one row per minute; reports a few seconds apart add up into it
ON CONFLICT (at) DO UPDATE SET seconds = seconds + excluded.seconds, rx = rx + excluded.rx, tx = tx + excluded.tx;

-- name: ListTraffic :many
SELECT at, seconds, rx, tx FROM router_traffic WHERE at >= ? ORDER BY at;

-- name: PruneTraffic :exec
DELETE FROM router_traffic WHERE at < ?;

-- B228: the devices the panel has seen
-- name: ListRouterDevices :many
SELECT mac, note, notify, cooldown_minutes, conn, wifi_iface, last_name, last_ip, online, online_since, offline_since, first_seen, last_seen, notified_at, notified_online
FROM router_devices;

-- name: UpsertRouterDevice :exec
-- A new row gets all columns. For a known device only what the reports change is written: note, notify and cooldown are changed by the user, never here.
INSERT INTO router_devices (mac, note, notify, cooldown_minutes, conn, wifi_iface, last_name, last_ip, online, online_since, offline_since, first_seen, last_seen, notified_at, notified_online)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (mac) DO UPDATE SET conn = excluded.conn, wifi_iface = excluded.wifi_iface, last_name = excluded.last_name, last_ip = excluded.last_ip,
    online = excluded.online, online_since = excluded.online_since, offline_since = excluded.offline_since, last_seen = excluded.last_seen,
    notified_at = excluded.notified_at, notified_online = excluded.notified_online;

-- name: UpdateRouterDeviceSettings :exec
UPDATE router_devices SET note = ?, notify = ?, cooldown_minutes = ?, notified_at = ?, notified_online = ? WHERE mac = ?;

-- name: DeleteRouterDevice :exec
DELETE FROM router_devices WHERE mac = ?;

-- name: DeleteRouterDeviceTraffic :exec
DELETE FROM router_device_traffic WHERE mac = ?;

-- name: PruneRouterDevices :exec
DELETE FROM router_devices WHERE last_seen < ? AND note = '' AND online = 0;

-- name: PruneOrphanDeviceTraffic :exec
DELETE FROM router_device_traffic WHERE mac NOT IN (SELECT mac FROM router_devices);

-- B228, B230: what happened
-- name: InsertRouterEvent :exec
INSERT INTO router_events (at, kind, mac, title, detail) VALUES (?, ?, ?, ?, ?);

-- name: CountRouterDeviceFlips :one
SELECT COUNT(*) FROM router_events WHERE mac = ? AND at >= ? AND kind IN ('device_online', 'device_offline');

-- name: PruneRouterEvents :exec
DELETE FROM router_events WHERE at < ?;

-- name: ResetRouterNotified :exec
UPDATE router_devices SET notified_at = NULL, notified_online = NULL;
