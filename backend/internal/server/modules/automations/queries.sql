-- name: GetAutomation :one
SELECT * FROM automations WHERE id=? LIMIT 1;

-- name: ListAutomations :many
SELECT * FROM automations ORDER BY updated_at DESC,id DESC;

-- name: DeleteAutomation :execrows
DELETE FROM automations WHERE id=?;
