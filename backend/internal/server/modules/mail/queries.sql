-- name: ListAccounts :many
SELECT * FROM mail_accounts ORDER BY id;

-- name: GetAccount :one
SELECT * FROM mail_accounts WHERE id = ?;

-- name: CreateAccount :one
INSERT INTO mail_accounts (name, email, provider, imap_host, imap_port, username, password_enc, notify, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateAccount :one
UPDATE mail_accounts
SET name = ?, imap_host = ?, imap_port = ?, username = ?, password_enc = ?, notify = ?
WHERE id = ?
RETURNING *;

-- name: DeleteAccount :exec
DELETE FROM mail_accounts WHERE id = ?;

-- name: SetSyncState :exec
UPDATE mail_accounts SET uid_validity = ?, last_uid = ?, last_sync_at = ? WHERE id = ?;

-- name: DeleteAccountMessages :exec
DELETE FROM mail_messages WHERE account_id = ?;

-- name: InsertMessage :execrows
INSERT INTO mail_messages (account_id, uid, message_id, from_name, from_address, subject, snippet, date, unread, flagged, has_attachments)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (account_id, uid) DO NOTHING;

-- name: GetMessageByUID :one
SELECT id, account_id, uid, message_id, from_name, from_address, subject, snippet, date, unread, flagged, has_attachments
FROM mail_messages WHERE account_id = ? AND uid = ?;

-- name: RecentFlags :many
SELECT uid, unread, flagged FROM mail_messages WHERE account_id = ? ORDER BY uid DESC LIMIT ?;

-- name: SetFlagsByUID :exec
UPDATE mail_messages SET unread = ?, flagged = ? WHERE account_id = ? AND uid = ?;

-- name: DeleteMessageByUID :exec
DELETE FROM mail_messages WHERE account_id = ? AND uid = ?;

-- name: GetMessage :one
SELECT * FROM mail_messages WHERE id = ?;

-- name: SetMessageFlags :exec
UPDATE mail_messages SET unread = ?, flagged = ? WHERE id = ?;

-- name: SetMessageBody :exec
UPDATE mail_messages SET body_json = ? WHERE id = ?;

-- name: ListMessages :many
SELECT id, account_id, uid, message_id, from_name, from_address, subject, snippet, date, unread, flagged, has_attachments
FROM mail_messages
WHERE (sqlc.narg(account_id) IS NULL OR account_id = sqlc.narg(account_id))
  AND (sqlc.arg(only_unread) = 0 OR unread = 1)
  AND (sqlc.narg(before) IS NULL OR date < sqlc.narg(before))
  AND (sqlc.arg(q) = '' OR from_name LIKE '%' || sqlc.arg(q) || '%' OR from_address LIKE '%' || sqlc.arg(q) || '%' OR subject LIKE '%' || sqlc.arg(q) || '%')
ORDER BY date DESC, id DESC
LIMIT sqlc.arg(lim);

-- name: UnreadCounts :many
SELECT account_id, COUNT(*) AS unread FROM mail_messages WHERE unread = 1 GROUP BY account_id;

-- name: LatestUnread :many
SELECT id, account_id, uid, message_id, from_name, from_address, subject, snippet, date, unread, flagged, has_attachments
FROM mail_messages WHERE unread = 1 ORDER BY date DESC, id DESC LIMIT ?;
