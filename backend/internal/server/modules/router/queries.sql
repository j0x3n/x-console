-- name: InsertTraffic :exec
INSERT INTO router_traffic (at, seconds, rx, tx) VALUES (?, ?, ?, ?)
-- B114: one row per minute; reports a few seconds apart add up into it
ON CONFLICT (at) DO UPDATE SET seconds = seconds + excluded.seconds, rx = rx + excluded.rx, tx = tx + excluded.tx;

-- name: ListTraffic :many
SELECT at, seconds, rx, tx FROM router_traffic WHERE at >= ? ORDER BY at;

-- name: PruneTraffic :exec
DELETE FROM router_traffic WHERE at < ?;
