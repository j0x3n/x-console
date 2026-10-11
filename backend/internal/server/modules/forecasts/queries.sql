-- name: InsertForecast :one
INSERT INTO forecasts (kind, statement, probability, due_date, reason, created_at) VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: GetForecast :one
SELECT * FROM forecasts WHERE id = ?;

-- name: CountForecasts :one
SELECT count(*) FROM forecasts;

-- name: ListOpenForecasts :many
SELECT * FROM forecasts WHERE outcome IS NULL ORDER BY due_date, id;

-- name: ListResolvedForecasts :many
SELECT * FROM forecasts WHERE outcome IS NOT NULL ORDER BY resolved_at DESC, id DESC;

-- name: UpdateOpenForecast :one
UPDATE forecasts SET statement = ?, probability = ?, due_date = ?, reason = ?, notified = ?
WHERE id = ? AND outcome IS NULL RETURNING *;

-- name: SetForecastNote :one
UPDATE forecasts SET note = ? WHERE id = ? RETURNING *;

-- name: ResolveForecast :one
UPDATE forecasts SET outcome = ?, note = ?, resolved_at = ? WHERE id = ? RETURNING *;

-- name: ReopenForecast :one
UPDATE forecasts SET outcome = NULL, resolved_at = NULL WHERE id = ? RETURNING *;

-- name: DeleteForecast :execrows
DELETE FROM forecasts WHERE id = ?;

-- name: ListDueUnnotified :many
SELECT * FROM forecasts WHERE outcome IS NULL AND notified = 0 AND due_date <= ? ORDER BY due_date, id;

-- name: SetForecastNotified :exec
UPDATE forecasts SET notified = 1 WHERE id = ?;

-- name: ListScoredForecasts :many
SELECT probability, outcome FROM forecasts WHERE probability IS NOT NULL AND outcome IN ('yes', 'no');
