-- name: ListQuotaAccounts :many
SELECT * FROM quota_accounts ORDER BY sort_order, id;

-- name: GetQuotaAccount :one
SELECT * FROM quota_accounts WHERE id = ?;

-- name: InsertQuotaAccount :one
INSERT INTO quota_accounts (kind, name, host_id, home, api_key, key_hash, sort_order, created_at)
VALUES (?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM quota_accounts), ?)
RETURNING *;

-- name: UpdateQuotaAccount :one
UPDATE quota_accounts SET name = ?, host_id = ?, home = ?, api_key = ?, key_hash = ? WHERE id = ? RETURNING *;

-- name: DeleteQuotaAccount :execrows
DELETE FROM quota_accounts WHERE id = ?;

-- name: SetQuotaOrder :exec
UPDATE quota_accounts SET sort_order = ? WHERE id = ?;

-- name: ListQuotaReadings :many
SELECT * FROM quota_readings;

-- name: GetQuotaReading :one
SELECT * FROM quota_readings WHERE account_id = ?;

-- name: DeleteQuotaReading :exec
DELETE FROM quota_readings WHERE account_id = ?;

-- name: SaveQuotaReading :exec
INSERT INTO quota_readings (account_id, ok, error, error_code, plan, user, credits, balances_json, windows_json, read_at, tried_at)
VALUES (?, 1, '', '', ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(account_id) DO UPDATE SET ok = 1, error = '', error_code = '', plan = excluded.plan, user = excluded.user,
  credits = excluded.credits, balances_json = excluded.balances_json, windows_json = excluded.windows_json,
  read_at = excluded.read_at, tried_at = excluded.tried_at;

-- name: SaveQuotaFailure :exec
-- A failed read keeps the last numbers and says why it failed.
INSERT INTO quota_readings (account_id, ok, error, error_code, tried_at)
VALUES (?, 0, ?, ?, ?)
ON CONFLICT(account_id) DO UPDATE SET ok = 0, error = excluded.error, error_code = excluded.error_code, tried_at = excluded.tried_at;
