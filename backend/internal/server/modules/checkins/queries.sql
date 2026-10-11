-- name: ListCheckinQuestions :many
SELECT * FROM checkin_questions ORDER BY sort_order, id;

-- name: GetCheckinQuestion :one
SELECT * FROM checkin_questions WHERE id = ?;

-- name: CountCheckinQuestions :one
SELECT count(*) FROM checkin_questions;

-- name: MaxCheckinSort :one
SELECT CAST(COALESCE(MAX(sort_order), 0) AS INTEGER) FROM checkin_questions;

-- name: InsertCheckinQuestion :one
INSERT INTO checkin_questions (label, low_label, high_label, times, enabled, sort_order, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateCheckinQuestion :one
UPDATE checkin_questions SET label = ?, low_label = ?, high_label = ?, times = ?, enabled = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: DeleteCheckinQuestion :execrows
DELETE FROM checkin_questions WHERE id = ?;

-- name: UpsertCheckinAnswer :one
INSERT INTO checkin_answers (question_id, value, date, slot, answered_at, source)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (question_id, date, slot) DO UPDATE SET value = excluded.value, answered_at = excluded.answered_at, source = excluded.source
RETURNING *;

-- name: ListCheckinAnswersOn :many
SELECT * FROM checkin_answers WHERE date = ?;

-- name: ListCheckinAnswersBetween :many
SELECT * FROM checkin_answers WHERE date >= ? AND date <= ? ORDER BY date, id;

-- name: InsertCheckinAsk :execrows
INSERT OR IGNORE INTO checkin_asks (question_id, date, slot, asked_at) VALUES (?, ?, ?, ?);

-- name: PruneCheckinAsks :exec
DELETE FROM checkin_asks WHERE date < ?;
