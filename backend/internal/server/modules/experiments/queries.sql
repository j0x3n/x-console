-- name: ListExperiments :many
SELECT * FROM experiments ORDER BY created_at DESC, id DESC;

-- name: GetExperiment :one
SELECT * FROM experiments WHERE id = ?;

-- name: CountExperiments :one
SELECT count(*) FROM experiments;

-- name: InsertExperiment :one
INSERT INTO experiments (title, change_text, baseline_start, change_start, end_date, note, conclusion, stopped, retrospective, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateExperiment :one
UPDATE experiments SET title = ?, change_text = ?, baseline_start = ?, change_start = ?, end_date = ?, note = ?, conclusion = ?, stopped = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: DeleteExperiment :execrows
DELETE FROM experiments WHERE id = ?;

-- name: ListExperimentMetrics :many
SELECT * FROM experiment_metrics WHERE experiment_id = ? ORDER BY sort_order, id;

-- name: ListAllExperimentMetrics :many
SELECT * FROM experiment_metrics ORDER BY experiment_id, sort_order, id;

-- name: InsertExperimentMetric :exec
INSERT INTO experiment_metrics (experiment_id, key, label, unit, subjective, sort_order) VALUES (?, ?, ?, ?, ?, ?);

-- name: DeleteExperimentMetrics :exec
DELETE FROM experiment_metrics WHERE experiment_id = ?;
