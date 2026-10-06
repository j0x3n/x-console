-- name: CreateReminder :one
INSERT INTO reminders (title, body, link, rrule, dtstart, next_at, enabled, created_at, icon)
VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
RETURNING *;

-- name: GetReminder :one
SELECT * FROM reminders WHERE id = ?;

-- name: ListReminders :many
-- A personal list stays small; ranges are filtered in Go.
SELECT * FROM reminders ORDER BY id;

-- name: ListDueReminders :many
SELECT * FROM reminders
WHERE enabled = 1
  AND ((next_at IS NOT NULL AND next_at <= sqlc.arg(now)) OR (snoozed_until IS NOT NULL AND snoozed_until <= sqlc.arg(now)))
ORDER BY id;

-- name: UpdateReminder :one
UPDATE reminders
SET title = ?, body = ?, link = ?, rrule = ?, dtstart = ?, next_at = ?, last_fired_at = ?,
    snoozed_until = ?, done_at = ?, enabled = ?, icon = ?
WHERE id = ?
RETURNING *;

-- name: DeleteReminder :execrows
DELETE FROM reminders WHERE id = ?;

-- name: ListRoutes :many
SELECT * FROM notification_routes ORDER BY sort_order, id;

-- name: DeleteRoutes :exec
DELETE FROM notification_routes;

-- name: InsertRoute :one
INSERT INTO notification_routes (kind_pattern, min_priority, channels, enabled, sort_order)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: UpsertPushSubscription :exec
INSERT INTO webpush_subscriptions (endpoint, p256dh, auth, user_agent, created_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (endpoint) DO UPDATE SET p256dh = excluded.p256dh, auth = excluded.auth, user_agent = excluded.user_agent;

-- name: ListPushSubscriptions :many
SELECT * FROM webpush_subscriptions ORDER BY id;

-- name: ListPushSubscriptionsNewestFirst :many
SELECT * FROM webpush_subscriptions ORDER BY created_at DESC, id DESC;

-- name: MarkPushOK :exec
UPDATE webpush_subscriptions SET last_ok_at = ?, last_error = NULL, last_error_at = NULL WHERE id = ?;

-- name: MarkPushError :exec
UPDATE webpush_subscriptions SET last_error = ?, last_error_at = ? WHERE id = ?;

-- name: DeletePushSubscriptionByID :execrows
DELETE FROM webpush_subscriptions WHERE id = ?;

-- name: CountPushSubscriptions :one
SELECT count(*) FROM webpush_subscriptions;

-- name: DeletePushSubscription :execrows
DELETE FROM webpush_subscriptions WHERE endpoint = ?;

-- name: ListMutes :many
SELECT * FROM notification_mutes ORDER BY id;

-- name: ListMutesInScope :many
SELECT * FROM notification_mutes WHERE scope = ? ORDER BY id;

-- name: InsertMute :one
INSERT INTO notification_mutes (kind_pattern, scope, target, created_at) VALUES (?, ?, ?, ?) RETURNING *;

-- name: DeleteMute :execrows
DELETE FROM notification_mutes WHERE id = ?;

-- name: DeleteMutesOfScopeAndKind :exec
DELETE FROM notification_mutes WHERE kind_pattern = ? AND scope = ?;

-- name: DeleteMutesOfScope :exec
DELETE FROM notification_mutes WHERE scope = ?;

-- name: PruneDeviceMutes :exec
-- A rule that points at a Web Push device that is gone has nothing to do.
DELETE FROM notification_mutes
WHERE target LIKE 'webpush:%' AND CAST(substr(target, 9) AS INTEGER) NOT IN (SELECT id FROM webpush_subscriptions);

-- name: GetPushSubscriptionByID :one
SELECT * FROM webpush_subscriptions WHERE id = ?;
