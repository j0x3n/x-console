-- name: CreateRepo :one
INSERT INTO coding_repos (agent_id, path, name, default_branch, remote_url, github_repo, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateRepoInfo :one
UPDATE coding_repos SET name = ?, default_branch = ?, remote_url = ?, github_repo = ?
WHERE id = ?
RETURNING *;

-- name: GetRepo :one
SELECT * FROM coding_repos WHERE id = ?;

-- name: CreateRemoteRepo :one
INSERT INTO coding_repos (agent_id, path, name, default_branch, remote_url, github_repo, created_at,
    connection_id, owner, repo, clone_url)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetRemoteRepoOn :one
SELECT * FROM coding_repos WHERE agent_id = ? AND connection_id = ? AND owner = ? AND repo = ?;

-- name: SetRepoDefaultBranch :exec
UPDATE coding_repos SET default_branch = ? WHERE id = ?;

-- name: GetRepoByPath :one
SELECT * FROM coding_repos WHERE agent_id = ? AND path = ?;

-- name: ListRepos :many
SELECT * FROM coding_repos ORDER BY name, id;

-- name: ListRepoPaths :many
SELECT id, path FROM coding_repos WHERE agent_id = ?;

-- name: DeleteRepo :execrows
DELETE FROM coding_repos WHERE id = ?;

-- name: CountActiveTasksForRepo :one
SELECT COUNT(*) FROM coding_tasks WHERE repo_id = ? AND status IN ('queued', 'running');

-- name: CreateTask :one
INSERT INTO coding_tasks (repo_id, issue_key, executor, prompt, base_branch, status, timeout_minutes, created_at, updated_at,
    ai_agent_id, model, permission)
VALUES (?, ?, ?, ?, ?, 'queued', ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: SetTaskBranch :exec
UPDATE coding_tasks SET branch = ? WHERE id = ?;

-- name: GetTask :one
SELECT sqlc.embed(coding_tasks), coding_repos.name AS repo_name, coding_repos.path AS repo_path,
       coding_repos.agent_id, coding_repos.github_repo, coding_repos.connection_id AS repo_connection_id,
       coding_repos.owner AS repo_owner, coding_repos.repo AS repo_repo, coding_repos.clone_url AS repo_clone_url
FROM coding_tasks JOIN coding_repos ON coding_repos.id = coding_tasks.repo_id
WHERE coding_tasks.id = ?;

-- name: ListTasks :many
SELECT sqlc.embed(coding_tasks), coding_repos.name AS repo_name, coding_repos.path AS repo_path,
       coding_repos.agent_id, coding_repos.github_repo, coding_repos.connection_id AS repo_connection_id,
       coding_repos.owner AS repo_owner, coding_repos.repo AS repo_repo, coding_repos.clone_url AS repo_clone_url
FROM coding_tasks JOIN coding_repos ON coding_repos.id = coding_tasks.repo_id
WHERE coding_tasks.status IN (SELECT value FROM json_each(sqlc.arg(statuses)))
  AND (sqlc.narg(repo_id) IS NULL OR coding_tasks.repo_id = sqlc.narg(repo_id))
  AND (sqlc.narg(issue_key) IS NULL OR coding_tasks.issue_key = sqlc.narg(issue_key))
  AND (sqlc.narg(ai_agent_id) IS NULL OR coding_tasks.ai_agent_id = sqlc.narg(ai_agent_id))
ORDER BY CASE coding_tasks.status WHEN 'running' THEN 0 WHEN 'queued' THEN 1 ELSE 2 END,
         CASE WHEN coding_tasks.status = 'queued' THEN coding_tasks.id ELSE -coding_tasks.id END
LIMIT sqlc.arg(lim);

-- name: ListQueued :many
SELECT coding_tasks.id, coding_repos.agent_id, coding_tasks.ai_agent_id
FROM coding_tasks JOIN coding_repos ON coding_repos.id = coding_tasks.repo_id
WHERE coding_tasks.status = 'queued'
ORDER BY coding_tasks.id;

-- name: ListRunningIDs :many
SELECT id FROM coding_tasks WHERE status = 'running';

-- name: CountRunningForAgent :one
SELECT COUNT(*) FROM coding_tasks WHERE status = 'running' AND ai_agent_id = ?;

-- name: CountRunning :one
SELECT COUNT(*) FROM coding_tasks WHERE status = 'running';

-- name: StartTask :execrows
UPDATE coding_tasks SET status = 'running', started_at = sqlc.arg(now), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND status = 'queued';

-- name: SetBaseCommit :exec
UPDATE coding_tasks SET base_commit = ? WHERE id = ?;

-- name: FinishTask :execrows
UPDATE coding_tasks
SET status = sqlc.arg(status), exit_code = sqlc.narg(exit_code), error = sqlc.arg(error),
    changed_files = sqlc.arg(changed_files), finished_at = sqlc.arg(now), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND status IN ('queued', 'running');

-- name: SetTaskStatus :execrows
UPDATE coding_tasks SET status = sqlc.arg(status), error = sqlc.arg(error), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND status = sqlc.arg(from_status);

-- name: SetCommitted :exec
UPDATE coding_tasks SET status = 'committed', commit_sha = ?, error = '', updated_at = ? WHERE id = ?;

-- name: SetPullRequest :exec
UPDATE coding_tasks SET status = 'pr_opened', pr_url = ?, updated_at = ? WHERE id = ?;

-- name: FailRunning :many
UPDATE coding_tasks SET status = 'failed', error = sqlc.arg(error), finished_at = sqlc.arg(now), updated_at = sqlc.arg(now)
WHERE status = 'running'
RETURNING id;

-- name: QueuePosition :one
SELECT COUNT(*) FROM coding_tasks WHERE status = 'queued' AND id <= ?;

-- name: LastSeq :one
SELECT CAST(COALESCE(MAX(seq), 0) AS INTEGER) FROM coding_task_events WHERE task_id = ?;

-- name: InsertEvent :exec
INSERT INTO coding_task_events (task_id, seq, at, kind, text, data) VALUES (?, ?, ?, ?, ?, ?);

-- name: ListEvents :many
SELECT * FROM coding_task_events
WHERE task_id = sqlc.arg(task_id) AND seq > sqlc.arg(after)
ORDER BY seq
LIMIT sqlc.arg(lim);

-- name: CancelQueued :execrows
UPDATE coding_tasks SET status = 'canceled', error = sqlc.arg(error), finished_at = sqlc.arg(now), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND status = 'queued';
