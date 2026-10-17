-- name: GetFeedsSettings :one
SELECT * FROM feeds_settings WHERE id = 1;

-- name: UpsertFeedsSettings :exec
INSERT INTO feeds_settings (id, agent_id, base_url, token_enc, updated_at)
VALUES (1, ?1, ?2, ?3, ?4)
ON CONFLICT(id) DO UPDATE SET agent_id = excluded.agent_id, base_url = excluded.base_url,
  token_enc = excluded.token_enc, updated_at = excluded.updated_at;

-- name: DeleteFeedsSettings :exec
DELETE FROM feeds_settings WHERE id = 1;
