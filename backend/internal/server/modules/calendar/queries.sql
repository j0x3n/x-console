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
SELECT e.*, c.name AS calendar_name, c.color AS calendar_color, c.kind AS calendar_kind
FROM calendar_events e
JOIN calendars c ON c.id = e.calendar_id
WHERE c.enabled = 1
  AND (e.rrule != '' OR (e.starts_at < sqlc.arg(until) AND e.ends_at >= sqlc.arg(since)) OR e.recurrence_id IS NOT NULL)
ORDER BY e.starts_at, e.id;
