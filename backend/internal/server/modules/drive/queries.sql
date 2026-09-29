-- name: GetItem :one
SELECT * FROM drive_items WHERE id = ? LIMIT 1;

-- name: InsertItem :one
INSERT INTO drive_items (parent_id, name, is_dir, size, mime, sha256, hidden, hidden_from, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: CountBlobReferences :one
SELECT (SELECT count(*) FROM drive_items WHERE drive_items.sha256 = sqlc.arg(hash) AND is_dir = 0)
     + (SELECT count(*) FROM drive_file_versions WHERE drive_file_versions.sha256 = sqlc.arg(hash));
