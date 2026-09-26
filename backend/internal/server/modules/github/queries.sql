-- name: ListPulls :many
SELECT * FROM github_pulls ORDER BY repo, updated_at DESC, number DESC;

-- name: ListPullsByRepo :many
SELECT * FROM github_pulls WHERE repo = ? ORDER BY updated_at DESC, number DESC;

-- name: GetPull :one
SELECT * FROM github_pulls WHERE repo = ? AND number = ?;

-- name: UpsertPull :exec
INSERT INTO github_pulls (repo, number, title, author, url, head_ref, head_sha, base_ref, draft,
                          review_state, check_state, created_at, updated_at, synced_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (repo, number) DO UPDATE SET
    title = excluded.title, author = excluded.author, url = excluded.url, head_ref = excluded.head_ref,
    head_sha = excluded.head_sha, base_ref = excluded.base_ref, draft = excluded.draft,
    review_state = excluded.review_state, check_state = excluded.check_state,
    created_at = excluded.created_at, updated_at = excluded.updated_at, synced_at = excluded.synced_at;

-- name: DeletePullsExcept :many
DELETE FROM github_pulls WHERE repo = sqlc.arg(repo) AND number NOT IN (sqlc.slice(keep)) RETURNING number;

-- name: DeletePullsNotIn :exec
DELETE FROM github_pulls WHERE repo NOT IN (sqlc.slice(repos));

-- name: ListRuns :many
SELECT * FROM github_runs ORDER BY created_at DESC, id DESC LIMIT ?;

-- name: ListRunsByRepo :many
SELECT * FROM github_runs WHERE repo = ? ORDER BY created_at DESC, id DESC LIMIT ?;

-- name: UpsertRun :exec
INSERT INTO github_runs (id, repo, workflow_id, name, branch, event, status, conclusion, url, head_sha,
                         default_branch, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    name = excluded.name, branch = excluded.branch, event = excluded.event, status = excluded.status,
    conclusion = excluded.conclusion, url = excluded.url, head_sha = excluded.head_sha,
    default_branch = excluded.default_branch, updated_at = excluded.updated_at;

-- name: DeleteRunsExcept :exec
DELETE FROM github_runs WHERE repo = sqlc.arg(repo) AND id NOT IN (sqlc.slice(keep));

-- name: DeleteRunsByRepo :exec
DELETE FROM github_runs WHERE repo = ?;

-- name: DeleteRunsNotIn :exec
DELETE FROM github_runs WHERE repo NOT IN (sqlc.slice(repos));

-- name: ListIssues :many
SELECT * FROM github_issues ORDER BY updated_at DESC, id DESC;

-- name: UpsertIssue :exec
INSERT INTO github_issues (repo, number, title, url, author, assignees, labels, relation, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (repo, number) DO UPDATE SET
    title = excluded.title, url = excluded.url, author = excluded.author, assignees = excluded.assignees,
    labels = excluded.labels, relation = excluded.relation, created_at = excluded.created_at,
    updated_at = excluded.updated_at;

-- name: DeleteIssuesExcept :exec
DELETE FROM github_issues WHERE repo = sqlc.arg(repo) AND number NOT IN (sqlc.slice(keep));

-- name: DeleteIssuesByRepo :exec
DELETE FROM github_issues WHERE repo = ?;

-- name: DeleteIssuesNotIn :exec
DELETE FROM github_issues WHERE repo NOT IN (sqlc.slice(repos));

-- name: ListLinks :many
SELECT * FROM github_links ORDER BY repo, number, kind, ref;

-- name: ListLinksForPull :many
SELECT * FROM github_links WHERE repo = ? AND number = ? ORDER BY kind, ref;

-- name: InsertLink :exec
INSERT INTO github_links (repo, number, kind, ref, created_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (repo, number, kind, ref) DO NOTHING;

-- name: LinkExists :one
SELECT COUNT(*) FROM github_links WHERE repo = ? AND number = ? AND kind = ? AND ref = ?;

-- name: GetCIState :one
SELECT state FROM github_ci_states WHERE key = ?;

-- name: SetCIState :exec
INSERT INTO github_ci_states (key, state, updated_at) VALUES (?, ?, ?)
ON CONFLICT (key) DO UPDATE SET state = excluded.state, updated_at = excluded.updated_at;

-- name: DeleteCIState :exec
DELETE FROM github_ci_states WHERE key = ?;
