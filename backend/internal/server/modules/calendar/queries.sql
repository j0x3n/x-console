-- name: ListCalendars :many
SELECT * FROM calendars ORDER BY id;

-- name: GetCalendar :one
SELECT * FROM calendars WHERE id = ?;

-- name: CreateCalendar :one
INSERT INTO calendars (name, kind, url, username, secret, color, enabled, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateCalendar :one
UPDATE calendars
SET name = ?, url = ?, username = ?, secret = ?, color = ?, enabled = ?
WHERE id = ?
RETURNING *;

-- name: SetCalendarSync :one
UPDATE calendars SET last_synced_at = ?, last_error = ? WHERE id = ?
RETURNING *;

-- name: DeleteCalendar :execrows
DELETE FROM calendars WHERE id = ?;

-- name: DeleteCalendarEvents :exec
DELETE FROM calendar_events WHERE calendar_id = ?;

-- name: InsertCalendarEvent :one
INSERT INTO calendar_events (calendar_id, uid, title, starts_at, ends_at, all_day, tzid, location,
                             description, rrule, rdates, exdates, recurrence_id, href, etag)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetCalendarEvent :one
SELECT * FROM calendar_events WHERE id = ?;

-- name: UpdateCalendarEvent :one
UPDATE calendar_events SET calendar_id = ?, title = ?, starts_at = ?, ends_at = ?, all_day = ?,
  tzid = ?, location = ?, description = ?, href = ?, etag = ? WHERE id = ? RETURNING *;

-- name: DeleteCalendarEvent :execrows
DELETE FROM calendar_events WHERE id = ?;

-- name: CountCalendarEvents :one
SELECT count(*) FROM calendar_events WHERE calendar_id = ?;

-- name: ListEventCandidates :many
-- Every repeating event plus single events near the range. Times are wall
-- clock in the event's own zone, so the caller passes a range widened by a
-- day on both sides and filters exactly in Go.
SELECT e.*, c.name AS calendar_name, c.color AS calendar_color, c.kind AS calendar_kind, c.read_only AS calendar_read_only
FROM calendar_events e
JOIN calendars c ON c.id = e.calendar_id
WHERE c.enabled = 1
  AND (e.rrule != '' OR (e.starts_at < sqlc.arg(until) AND e.ends_at >= sqlc.arg(since)) OR e.recurrence_id IS NOT NULL)
ORDER BY e.starts_at, e.id;

-- name: CreateAccountCalendar :one
INSERT INTO calendars (name, kind, url, color, enabled, created_at, account_id, remote_id, read_only)
VALUES (?, 'caldav', ?, ?, 1, ?, ?, ?, ?)
RETURNING *;

-- name: ListAccountCalendars :many
SELECT * FROM calendars WHERE account_id = ? ORDER BY id;

-- name: ListCalendarAccounts :many
SELECT * FROM calendar_accounts ORDER BY id;

-- name: GetCalendarAccount :one
SELECT * FROM calendar_accounts WHERE id = ?;

-- name: CreateCalendarAccount :one
INSERT INTO calendar_accounts (provider, client_id, created_at, updated_at)
VALUES ('google', ?, ?, ?)
RETURNING *;

-- name: UpdateCalendarAccountClient :one
UPDATE calendar_accounts SET client_id = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: SetCalendarAccountAuthorized :one
UPDATE calendar_accounts SET email = ?, auth_error = '', auth_notified_at = NULL, updated_at = ?
WHERE id = ? RETURNING *;

-- name: SetCalendarAccountAuthError :one
UPDATE calendar_accounts SET auth_error = ?, auth_notified_at = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: DeleteCalendarAccount :execrows
DELETE FROM calendar_accounts WHERE id = ?;
