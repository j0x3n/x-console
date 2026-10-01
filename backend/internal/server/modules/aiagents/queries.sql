-- B47 AI agents and Git connections.

-- name: ListAgents :many
SELECT * FROM ai_agents ORDER BY id;

-- name: GetAgent :one
SELECT * FROM ai_agents WHERE id = ?;

-- name: CreateAgent :one
INSERT INTO ai_agents (name, avatar, color, kind, model, instructions, runner_agent_id, access, cli_permission,
    repo_ids, max_parallel, monthly_budget_usd, auto_build, build_retries, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateAgent :one
UPDATE ai_agents SET name = ?, avatar = ?, color = ?, model = ?, instructions = ?, runner_agent_id = ?, access = ?,
    cli_permission = ?, repo_ids = ?, max_parallel = ?, monthly_budget_usd = ?, auto_build = ?, build_retries = ?,
    enabled = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteAgent :execrows
DELETE FROM ai_agents WHERE id = ?;

-- name: DetachAgentTasks :exec
UPDATE coding_tasks SET ai_agent_id = NULL WHERE ai_agent_id = ?;

-- name: AgentTaskCounts :many
SELECT ai_agent_id, status, count(*) AS n FROM coding_tasks
WHERE ai_agent_id IS NOT NULL AND status IN ('queued', 'running')
GROUP BY ai_agent_id, status;

-- name: AgentMonthCost :one
SELECT CAST(coalesce(sum(u.cost), 0) AS REAL) AS cost FROM ai_usage u
WHERE u.created_at >= sqlc.arg(since) AND (
    (u.source = 'coding' AND u.ref IN (SELECT CAST(t.id AS TEXT) FROM coding_tasks t WHERE t.ai_agent_id = sqlc.arg(agent_id)))
    OR (u.source = 'ai_agent' AND u.ref = CAST(sqlc.arg(agent_id) AS TEXT)));

-- name: AgentNames :many
SELECT id, name FROM agents;

-- name: RepoIDs :many
SELECT id FROM coding_repos;

-- name: ListConnections :many
SELECT * FROM git_connections ORDER BY id;

-- name: GetConnection :one
SELECT * FROM git_connections WHERE id = ?;

-- name: CreateConnection :one
INSERT INTO git_connections (kind, name, base_url, token_enc, use_github_module, webhook_secret_enc, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: RenameConnection :exec
UPDATE git_connections SET name = ? WHERE id = ?;

-- name: SetConnectionToken :exec
UPDATE git_connections SET token_enc = ? WHERE id = ?;

-- name: SetConnectionCheck :exec
UPDATE git_connections SET username = ?, last_checked_at = ?, last_error = ? WHERE id = ?;

-- name: DeleteConnection :execrows
DELETE FROM git_connections WHERE id = ?;

-- name: DetachConnectionRepos :exec
UPDATE coding_repos SET connection_id = NULL WHERE connection_id = ?;

-- name: TasksByPR :many
SELECT id, issue_key, ai_agent_id FROM coding_tasks WHERE pr_url = ? AND pr_url != '';

-- name: AdoptModuleToken :exec
-- B62: connections that borrowed the GitHub module's token get their own copy.
UPDATE git_connections SET token_enc = ?, use_github_module = 0 WHERE use_github_module = 1;
