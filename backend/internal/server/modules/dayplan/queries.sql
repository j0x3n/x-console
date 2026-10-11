-- name: GetDayPlan :one
SELECT * FROM dayplans WHERE date = ?;

-- name: UpsertDayPlan :exec
INSERT INTO dayplans (date, status, source, generated_at, confirmed_at, review_sent)
VALUES (?, ?, ?, ?, ?, 0)
ON CONFLICT (date) DO UPDATE SET status = excluded.status, source = excluded.source, generated_at = excluded.generated_at,
 confirmed_at = excluded.confirmed_at, review_sent = 0;

-- name: SetDayPlanSource :exec
UPDATE dayplans SET source = ? WHERE date = ?;

-- name: ConfirmDayPlan :execrows
UPDATE dayplans SET status = 'confirmed', confirmed_at = ? WHERE date = ? AND status = 'draft';

-- name: SetDayPlanReviewSent :exec
UPDATE dayplans SET review_sent = 1 WHERE date = ?;

-- name: ListDayPlanItems :many
SELECT * FROM dayplan_items WHERE plan_date = ? ORDER BY position, id;

-- name: GetDayPlanItem :one
SELECT * FROM dayplan_items WHERE id = ?;

-- name: DeleteDayPlanItems :exec
DELETE FROM dayplan_items WHERE plan_date = ?;

-- name: InsertDayPlanItem :exec
INSERT INTO dayplan_items (plan_date, position, title, kind, ref, start_time, end_time, reason, done, notified)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: SetDayPlanItemDone :exec
UPDATE dayplan_items SET done = ? WHERE id = ?;

-- name: SetDayPlanItemNotified :exec
UPDATE dayplan_items SET notified = 1 WHERE id = ?;

-- name: PruneDayPlans :exec
DELETE FROM dayplans WHERE date < ?;
