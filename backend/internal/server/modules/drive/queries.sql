-- name: GetItem :one
SELECT * FROM drive_items WHERE id = ? LIMIT 1;

-- name: InsertItem :one
INSERT INTO drive_items (parent_id, name, is_dir, size, mime, sha256, hidden, hidden_from, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: CountBlobReferences :one
SELECT count(*) FROM drive_items WHERE sha256 = ? AND is_dir = 0;
