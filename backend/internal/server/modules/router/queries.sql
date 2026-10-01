-- name: InsertTraffic :exec
INSERT INTO router_traffic (at, seconds, rx, tx) VALUES (?, ?, ?, ?)
ON CONFLICT (at) DO UPDATE SET seconds = excluded.seconds, rx = excluded.rx, tx = excluded.tx;

-- name: ListTraffic :many
SELECT at, seconds, rx, tx FROM router_traffic WHERE at >= ? ORDER BY at;

-- name: PruneTraffic :exec
DELETE FROM router_traffic WHERE at < ?;
