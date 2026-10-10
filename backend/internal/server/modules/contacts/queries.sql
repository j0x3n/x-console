-- name: ListContacts :many
SELECT * FROM contacts ORDER BY id;

-- name: GetContact :one
SELECT * FROM contacts WHERE id = ?;

-- name: InsertContact :one
INSERT INTO contacts (name, group_kind, events, last_contact_on, contact_every_days, remind_days, notes, phones, emails, source, external_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateContact :one
UPDATE contacts SET name = ?1, group_kind = ?2, events = ?3, last_contact_on = ?4, contact_every_days = ?5,
  remind_days = ?6, notes = ?7, archived_at = ?8, updated_at = ?9, phones = ?10, emails = ?11, source = ?12, external_id = ?13
WHERE id = ?14 RETURNING *;

-- name: DeleteContact :execrows
DELETE FROM contacts WHERE id = ?;

-- name: SetContactNotified :exec
UPDATE contacts SET notified_json = ? WHERE id = ?;

-- name: SetContactLost :exec
UPDATE contacts SET lost_for = ? WHERE id = ?;
