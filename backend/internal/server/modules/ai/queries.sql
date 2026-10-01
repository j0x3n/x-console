-- name: GetConversation :one
SELECT * FROM ai_conversations WHERE id=? LIMIT 1;

-- name: CreateConversation :one
INSERT INTO ai_conversations(title,created_at,updated_at,permission,model,effort) VALUES(?,?,?,?,?,?) RETURNING *;

-- name: ListConversations :many
SELECT * FROM ai_conversations WHERE host_id IS NULL ORDER BY updated_at DESC,id DESC;

-- name: DeleteConversation :execrows
DELETE FROM ai_conversations WHERE id=?;
