-- name: ListCredentials :many
SELECT * FROM credentials ORDER BY id;

-- name: GetCredential :one
SELECT * FROM credentials WHERE id = ?;

-- name: InsertCredential :one
INSERT INTO credentials (kind, name, platform, account, used_by, scopes, hint, created_on, rotated_on, expires_on, rotate_every_days, remind_days, notes, secret_enc, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateCredential :one
-- A new expiry date starts the expiry reminders over. The reminders for a
-- missed rotation key on the date they were told for, so they need no reset.
UPDATE credentials SET kind = ?1, name = ?2, platform = ?3, account = ?4, used_by = ?5, scopes = ?6, hint = ?7,
  created_on = ?8, rotated_on = ?9, expires_on = ?10, rotate_every_days = ?11, remind_days = ?12, notes = ?13,
  archived_at = ?14, updated_at = ?15, secret_enc = ?16,
  notified_for = CASE WHEN expires_on = ?10 THEN notified_for ELSE '' END,
  notified_json = CASE WHEN expires_on = ?10 THEN notified_json ELSE '[]' END
WHERE id = ?17 RETURNING *;

-- name: DeleteCredential :execrows
DELETE FROM credentials WHERE id = ?;

-- name: SetCredentialNotified :exec
UPDATE credentials SET notified_for = ?, notified_json = ? WHERE id = ?;

-- name: SetCredentialStale :exec
UPDATE credentials SET stale_for = ?, stale_notified_on = ? WHERE id = ?;
