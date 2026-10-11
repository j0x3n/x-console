-- name: ListFocusBlockAgents :many
SELECT * FROM focus_block_agents;

-- name: GetFocusBlockAgent :one
SELECT * FROM focus_block_agents WHERE agent_id = ?;

-- name: SaveFocusBlockAgent :exec
INSERT INTO focus_block_agents (agent_id, state, session_id, config_hash, hosts_status, hosts_error, message, sites, killed, refused, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(agent_id) DO UPDATE SET state = excluded.state, session_id = excluded.session_id, config_hash = excluded.config_hash,
    hosts_status = excluded.hosts_status, hosts_error = excluded.hosts_error, message = excluded.message, sites = excluded.sites,
    killed = excluded.killed, refused = excluded.refused, updated_at = excluded.updated_at;

-- name: DeleteFocusBlockAgent :exec
DELETE FROM focus_block_agents WHERE agent_id = ?;
