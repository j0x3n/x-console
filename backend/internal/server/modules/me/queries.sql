-- name: OpenMeState :one
SELECT * FROM me_states WHERE ended_at IS NULL ORDER BY id DESC LIMIT 1;

-- name: InsertMeState :one
INSERT INTO me_states (state, source, guessed, manual, started_at) VALUES (?, ?, ?, ?, ?) RETURNING *;

-- name: CloseMeState :exec
UPDATE me_states SET ended_at = ? WHERE id = ?;

-- name: UpdateMeStateFlags :exec
UPDATE me_states SET source = ?, guessed = ?, manual = ? WHERE id = ?;

-- name: ListMeStatesSince :many
SELECT * FROM me_states WHERE ended_at IS NULL OR ended_at > ? ORDER BY started_at, id;

-- name: LastMeStateEnd :one
SELECT ended_at FROM me_states WHERE state = ? AND ended_at IS NOT NULL ORDER BY ended_at DESC LIMIT 1;

-- name: DeleteMeStatesBefore :exec
DELETE FROM me_states WHERE ended_at IS NOT NULL AND ended_at < ?;

-- name: InsertMeReport :one
INSERT INTO me_reports (event, source, at, created_at) VALUES (?, ?, ?, ?) RETURNING *;

-- name: CountMeReportsNear :one
SELECT COUNT(*) FROM me_reports WHERE event = sqlc.arg(event) AND at > sqlc.arg(from_at) AND at < sqlc.arg(to_at);

-- The newest sleep report and the newest location report stay however old
-- they are: they still say where the user is.
-- name: DeleteMeReportsBefore :exec
DELETE FROM me_reports WHERE me_reports.at < ?
 AND me_reports.id NOT IN (SELECT s.id FROM me_reports s WHERE s.event IN ('sleep.start', 'sleep.end') ORDER BY s.at DESC, s.id DESC LIMIT 1)
 AND me_reports.id NOT IN (SELECT p.id FROM me_reports p WHERE p.event IN ('home.arrive', 'home.leave') ORDER BY p.at DESC, p.id DESC LIMIT 1);

-- name: RecentMeSleeps :many
SELECT * FROM me_states WHERE state = 'sleeping' AND ended_at IS NOT NULL ORDER BY ended_at DESC LIMIT 20;

-- name: LatestMeSleepReport :one
SELECT * FROM me_reports WHERE event IN ('sleep.start', 'sleep.end') ORDER BY at DESC, id DESC LIMIT 1;

-- name: LatestMePlaceReport :one
SELECT * FROM me_reports WHERE event IN ('home.arrive', 'home.leave') ORDER BY at DESC, id DESC LIMIT 1;
