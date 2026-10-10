-- name: ListDocuments :many
SELECT * FROM documents ORDER BY id;

-- name: GetDocument :one
SELECT * FROM documents WHERE id = ?;

-- name: InsertDocument :one
INSERT INTO documents (kind, name, holder, number, issued_on, expires_on, price, currency, serial, remind_days, notes, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateDocument :one
-- A new expiry date starts the reminders over.
UPDATE documents SET kind = ?1, name = ?2, holder = ?3, number = ?4, issued_on = ?5, expires_on = ?6, price = ?7,
  currency = ?8, serial = ?9, remind_days = ?10, notes = ?11, archived_at = ?12, updated_at = ?13,
  notified_for = CASE WHEN expires_on = ?6 THEN notified_for ELSE '' END,
  notified_json = CASE WHEN expires_on = ?6 THEN notified_json ELSE '[]' END
WHERE id = ?14 RETURNING *;

-- name: DeleteDocument :execrows
DELETE FROM documents WHERE id = ?;

-- name: SetDocumentFiles :one
UPDATE documents SET files_json = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: SetDocumentNotified :exec
UPDATE documents SET notified_for = ?, notified_json = ? WHERE id = ?;
