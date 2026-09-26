-- name: ListTeams :many
SELECT * FROM linear_teams ORDER BY team_key, team_id;

-- name: GetTeam :one
SELECT * FROM linear_teams WHERE team_id = ?;

-- name: GetTeamByProject :one
SELECT * FROM linear_teams WHERE project_id = ?;

-- name: InsertTeam :exec
INSERT INTO linear_teams (team_id, team_key, team_name, project_id, cursor, created_at) VALUES (?, ?, ?, ?, ?, ?);

-- name: DeleteTeams :exec
DELETE FROM linear_teams;

-- name: SetTeamCursor :exec
UPDATE linear_teams SET cursor = ? WHERE team_id = ?;

-- name: GetIssue :one
SELECT * FROM linear_issues WHERE linear_id = ?;

-- name: GetIssueByKey :one
SELECT * FROM linear_issues WHERE issue_key = ? ORDER BY synced_at DESC LIMIT 1;

-- name: UpsertIssue :exec
INSERT INTO linear_issues (linear_id, issue_key, identifier, remote_updated_at, local_updated_at, synced_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (linear_id) DO UPDATE SET
    issue_key = excluded.issue_key, identifier = excluded.identifier,
    remote_updated_at = excluded.remote_updated_at, local_updated_at = excluded.local_updated_at,
    synced_at = excluded.synced_at;
