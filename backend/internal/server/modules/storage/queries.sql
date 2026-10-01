-- name: ListRemotes :many
SELECT * FROM storage_remotes ORDER BY id;

-- name: GetRemote :one
SELECT * FROM storage_remotes WHERE id = ?;

-- name: CreateRemote :one
INSERT INTO storage_remotes (kind, name, config, show_in_drive, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateRemote :one
UPDATE storage_remotes SET name = ?, config = ?, show_in_drive = ?, updated_at = ? WHERE id = ?
RETURNING *;

-- name: DeleteRemote :exec
DELETE FROM storage_remotes WHERE id = ?;
