-- M5 projects queries. The filtered issue list lives in list.go (database/sql).
-- Keep this file ASCII only: sqlc miscounts offsets after multi-byte characters.

-- name: ListProjects :many
SELECT sqlc.embed(projects),
    CAST((SELECT count(*) FROM issues i WHERE i.project_id = projects.id) AS INTEGER) AS issue_count,
    CAST((SELECT count(*) FROM issues i WHERE i.project_id = projects.id AND i.status NOT IN ('done', 'canceled')) AS INTEGER) AS open_count
FROM projects
WHERE (projects.archived_at IS NOT NULL) = CAST(sqlc.arg(archived) AS BOOLEAN)
ORDER BY projects.name COLLATE NOCASE, projects.id;

-- name: GetProject :one
SELECT sqlc.embed(projects),
    CAST((SELECT count(*) FROM issues i WHERE i.project_id = projects.id) AS INTEGER) AS issue_count,
    CAST((SELECT count(*) FROM issues i WHERE i.project_id = projects.id AND i.status NOT IN ('done', 'canceled')) AS INTEGER) AS open_count
FROM projects
WHERE projects.id = ?;

-- name: ProjectKeyExists :one
SELECT count(*) FROM projects WHERE key = ?;

-- name: GetProjectIDByKey :one
SELECT id FROM projects WHERE key = ?;

-- name: CreateProject :one
INSERT INTO projects (key, name, description, color, icon, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: UpdateProject :exec
UPDATE projects
SET name = ?, description = ?, color = ?, icon = ?, archived_at = ?, updated_at = ?
WHERE id = ?;

-- name: TakeIssueNumber :one
UPDATE projects SET next_number = next_number + 1 WHERE id = ?
RETURNING next_number;

-- name: InsertIssue :one
INSERT INTO issues (project_id, number, title, description, status, priority, due_date, milestone_id,
                    sort_order, external_source, external_id, created_at, updated_at, completed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: GetIssue :one
SELECT sqlc.embed(issues), projects.key AS project_key
FROM issues JOIN projects ON projects.id = issues.project_id
WHERE issues.id = ?;

-- name: GetIssueByKey :one
SELECT sqlc.embed(issues), projects.key AS project_key
FROM issues JOIN projects ON projects.id = issues.project_id
WHERE projects.key = ? AND issues.number = ?;

-- name: GetIssueByExternal :one
SELECT sqlc.embed(issues), projects.key AS project_key
FROM issues JOIN projects ON projects.id = issues.project_id
WHERE issues.external_source = ? AND issues.external_id = ?;

-- name: UpdateIssue :exec
UPDATE issues
SET title = ?, description = ?, status = ?, priority = ?, due_date = ?, milestone_id = ?,
    sort_order = ?, updated_at = ?, completed_at = ?
WHERE id = ?;

-- name: SetIssueExternal :execrows
UPDATE issues SET external_source = ?, external_id = ? WHERE id = ?;

-- name: DeleteIssue :exec
DELETE FROM issues WHERE id = ?;

-- name: MinSortOrder :one
SELECT CAST(COALESCE(MIN(sort_order), 0) AS REAL) FROM issues WHERE project_id = ? AND status = ? AND id <> ?;

-- name: MaxSortOrder :one
SELECT CAST(COALESCE(MAX(sort_order), 0) AS REAL) FROM issues WHERE project_id = ? AND status = ? AND id <> ?;

-- name: CountInColumn :one
SELECT count(*) FROM issues WHERE project_id = ? AND status = ? AND id <> ?;

-- name: NextInColumn :one
SELECT sort_order FROM issues
WHERE project_id = ? AND status = ? AND id <> ? AND sort_order > ?
ORDER BY sort_order LIMIT 1;

-- name: PrevInColumn :one
SELECT sort_order FROM issues
WHERE project_id = ? AND status = ? AND id <> ? AND sort_order < ?
ORDER BY sort_order DESC LIMIT 1;

-- name: ListColumn :many
SELECT id FROM issues WHERE project_id = ? AND status = ? AND id <> ? ORDER BY sort_order, id;

-- name: SetSortOrder :exec
UPDATE issues SET sort_order = ? WHERE id = ?;

-- name: ListLabelsForIssues :many
SELECT issue_labels.issue_id, sqlc.embed(labels)
FROM issue_labels JOIN labels ON labels.id = issue_labels.label_id
WHERE issue_labels.issue_id IN (sqlc.slice(ids))
ORDER BY labels.name COLLATE NOCASE;

-- name: ClearIssueLabels :exec
DELETE FROM issue_labels WHERE issue_id = ?;

-- name: AddIssueLabel :exec
INSERT OR IGNORE INTO issue_labels (issue_id, label_id) VALUES (?, ?);

-- name: ListDue :many
SELECT sqlc.embed(issues), projects.key AS project_key
FROM issues JOIN projects ON projects.id = issues.project_id
WHERE issues.due_date IS NOT NULL AND issues.due_date <= ?
  AND issues.status NOT IN ('done', 'canceled') AND projects.archived_at IS NULL
ORDER BY issues.due_date, CASE issues.priority WHEN 0 THEN 5 ELSE issues.priority END, issues.id;

-- name: ChangedSince :many
SELECT sqlc.embed(issues), projects.key AS project_key
FROM issues JOIN projects ON projects.id = issues.project_id
WHERE issues.project_id IN (sqlc.slice(project_ids)) AND issues.updated_at > sqlc.arg(since)
ORDER BY issues.updated_at, issues.id;

-- name: ListLabels :many
SELECT * FROM labels WHERE project_id = ? OR project_id IS NULL ORDER BY name COLLATE NOCASE, id;

-- name: GetLabel :one
SELECT * FROM labels WHERE id = ?;

-- name: CreateLabel :one
INSERT INTO labels (project_id, name, color) VALUES (?, ?, ?) RETURNING *;

-- name: UpdateLabel :one
UPDATE labels SET name = ?, color = ? WHERE id = ? RETURNING *;

-- name: DeleteLabel :exec
DELETE FROM labels WHERE id = ?;

-- name: ListMilestones :many
SELECT * FROM milestones WHERE project_id = ?
ORDER BY due_date IS NULL, due_date, name COLLATE NOCASE, id;

-- name: GetMilestone :one
SELECT * FROM milestones WHERE id = ?;

-- name: CreateMilestone :one
INSERT INTO milestones (project_id, name, due_date, created_at) VALUES (?, ?, ?, ?) RETURNING *;

-- name: UpdateMilestone :one
UPDATE milestones SET name = ?, due_date = ? WHERE id = ? RETURNING *;

-- name: DeleteMilestone :exec
DELETE FROM milestones WHERE id = ?;

-- name: ListComments :many
SELECT * FROM issue_comments WHERE issue_id = ? ORDER BY created_at, id;

-- name: CreateComment :one
INSERT INTO issue_comments (issue_id, body, created_at) VALUES (?, ?, ?) RETURNING *;

-- name: DeleteComment :execrows
DELETE FROM issue_comments WHERE id = ? AND issue_id = ?;

-- name: ListLinks :many
SELECT * FROM issue_links WHERE issue_id = ? ORDER BY created_at, id;

-- name: FindLink :one
SELECT * FROM issue_links WHERE issue_id = ? AND kind = ? AND url = ? LIMIT 1;

-- name: CreateLink :one
INSERT INTO issue_links (issue_id, kind, title, url, ref, created_at) VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: DeleteLink :execrows
DELETE FROM issue_links WHERE id = ? AND issue_id = ?;
