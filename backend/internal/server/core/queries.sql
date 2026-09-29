-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: CreateUser :one
INSERT INTO users (username, password_hash, totp_secret, created_at, setup_completed)
VALUES (?, ?, ?, ?, 0)
RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = ?;

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: GetFirstUser :one
SELECT * FROM users ORDER BY id LIMIT 1;

-- name: EnableTOTP :execrows
UPDATE users SET totp_enabled = 1, setup_completed = 1 WHERE id = ? AND setup_completed = 0 AND totp_enabled = 0 AND totp_secret <> '';

-- name: FinishSetupWithoutTOTP :execrows
UPDATE users SET totp_secret = '', setup_completed = 1 WHERE id = ? AND setup_completed = 0 AND totp_enabled = 0;

-- name: ConfirmPendingTOTP :execrows
UPDATE users SET totp_enabled = 1, totp_secret = ? WHERE id = ? AND setup_completed = 1 AND totp_enabled = 0;

-- name: DisableTOTP :exec
UPDATE users SET totp_enabled = 0, totp_secret = '' WHERE id = ?;

-- name: UpdatePassword :exec
UPDATE users SET password_hash = ? WHERE id = ?;

-- name: DeleteOtherSessions :exec
DELETE FROM sessions WHERE user_id = ? AND id <> ?;

-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, created_at, expires_at, user_agent, ip)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetSession :one
SELECT sessions.*, users.username
FROM sessions JOIN users ON users.id = sessions.user_id
WHERE sessions.id = ? AND sessions.expires_at > ?;

-- name: TouchSession :exec
UPDATE sessions SET expires_at = ? WHERE id = ?;

-- name: ElevateSession :exec
UPDATE sessions SET elevated_until = ? WHERE id = ?;

-- name: SetVaultUntil :exec
UPDATE sessions SET vault_until = ? WHERE id = ?;

-- name: ClearVaultSessions :exec
UPDATE sessions SET vault_until = NULL WHERE vault_until IS NOT NULL;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= ?;

-- name: InsertAudit :exec
INSERT INTO audit_log (at, actor, action, target, detail, result)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListAudit :many
SELECT * FROM audit_log WHERE id < ? ORDER BY id DESC LIMIT ?;

-- name: GetSetting :one
SELECT * FROM settings WHERE key = ?;

-- name: InsertVaultPassword :execrows
INSERT INTO settings (key, value, encrypted, updated_at)
VALUES ('vault.password_hash', ?, 0, ?)
ON CONFLICT (key) DO NOTHING;

-- name: UpsertSetting :exec
INSERT INTO settings (key, value, encrypted, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value, encrypted = excluded.encrypted, updated_at = excluded.updated_at;

-- name: DeleteSetting :exec
DELETE FROM settings WHERE key = ?;

-- name: ListSettingsByPrefix :many
SELECT * FROM settings WHERE key LIKE ? || '%' ORDER BY key;

-- name: CreatePairingCode :exec
INSERT INTO pairing_codes (code_hash, name, kind, expires_at) VALUES (?, ?, ?, ?);

-- name: UsePairingCode :one
UPDATE pairing_codes SET used_at = ?
WHERE code_hash = ? AND used_at IS NULL AND expires_at > ?
RETURNING *;

-- name: PeekPairingCode :one
SELECT * FROM pairing_codes
WHERE code_hash = ? AND used_at IS NULL AND expires_at > ?;

-- name: CreateAgent :exec
INSERT INTO agents (id, name, kind, os, arch, hostname, version, capabilities, token_hash, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetAgentByTokenHash :one
SELECT * FROM agents WHERE token_hash = ? AND revoked_at IS NULL;

-- name: GetAgent :one
SELECT * FROM agents WHERE id = ?;

-- name: ListAgents :many
SELECT * FROM agents WHERE revoked_at IS NULL ORDER BY kind, name;

-- name: UpdateAgentSeen :exec
UPDATE agents SET last_seen_at = ?, os = ?, arch = ?, hostname = ?, version = ?, capabilities = ? WHERE id = ?;

-- name: RevokeAgent :execrows
UPDATE agents SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL;

-- name: InsertNotification :one
INSERT INTO notifications (created_at, kind, title, body, link, priority, source, data)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListNotifications :many
SELECT * FROM notifications
WHERE id < sqlc.arg(before) AND (CAST(sqlc.arg(unread_only) AS INTEGER) = 0 OR read_at IS NULL)
ORDER BY id DESC LIMIT sqlc.arg(lim);

-- name: CountUnreadNotifications :one
SELECT count(*) FROM notifications WHERE read_at IS NULL;

-- name: MarkNotificationRead :execrows
UPDATE notifications SET read_at = ? WHERE id = ? AND read_at IS NULL;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = ? WHERE read_at IS NULL;

-- name: DeleteNotification :execrows
DELETE FROM notifications WHERE id = ?;

-- name: DeleteAllUsers :exec
DELETE FROM users;
