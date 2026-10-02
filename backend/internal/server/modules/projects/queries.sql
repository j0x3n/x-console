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
SET name = ?, description = ?, color = ?, icon = ?, archived_at = ?, layout_locked = ?, updated_at = ?
WHERE id = ?;

-- name: TakeIssueNumber :one
UPDATE projects SET next_number = next_number + 1 WHERE id = ?
RETURNING next_number;

-- name: InsertIssue :one
INSERT INTO issues (project_id, number, title, description, status, priority, due_date, milestone_id,
                    sort_order, external_source, external_id, created_at, updated_at, completed_at,
                    category_id, due_at, due_remind, board_id, list_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
    sort_order = ?, updated_at = ?, completed_at = ?, category_id = ?, due_at = ?, due_remind = ?, due_notified_at = ?
WHERE id = ?;

-- name: GetCategoryProject :one
SELECT project_id FROM project_categories WHERE id=?;

-- name: ChecklistProgressForIssues :many
SELECT c.issue_id, count(it.id) AS total, sum(CASE WHEN it.done=1 THEN 1 ELSE 0 END) AS done
FROM issue_checklists c JOIN issue_checklist_items it ON it.checklist_id=c.id
WHERE c.issue_id IN (sqlc.slice(issue_ids)) GROUP BY c.issue_id;

-- name: SetIssueExternal :execrows
UPDATE issues SET external_source = ?, external_id = ? WHERE id = ?;

-- name: DeleteIssue :exec
DELETE FROM issues WHERE id = ?;

-- B46: a column is a list (list_id), not a status. Archived cards do not count.

-- name: MinSortOrder :one
SELECT CAST(COALESCE(MIN(sort_order), 0) AS REAL) FROM issues WHERE list_id = ? AND id <> ? AND archived_at IS NULL;

-- name: MaxSortOrder :one
SELECT CAST(COALESCE(MAX(sort_order), 0) AS REAL) FROM issues WHERE list_id = ? AND id <> ? AND archived_at IS NULL;

-- name: CountInColumn :one
SELECT count(*) FROM issues WHERE list_id = ? AND id <> ? AND archived_at IS NULL;

-- name: NextInColumn :one
SELECT sort_order FROM issues
WHERE list_id = ? AND id <> ? AND sort_order > ? AND archived_at IS NULL
ORDER BY sort_order LIMIT 1;

-- name: PrevInColumn :one
SELECT sort_order FROM issues
WHERE list_id = ? AND id <> ? AND sort_order < ? AND archived_at IS NULL
ORDER BY sort_order DESC LIMIT 1;

-- name: ListColumn :many
SELECT id FROM issues WHERE list_id = ? AND id <> ? AND archived_at IS NULL ORDER BY sort_order, id;

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
WHERE issues.due_at IS NOT NULL AND issues.due_at <= ?
  AND issues.status NOT IN ('done', 'canceled') AND projects.archived_at IS NULL AND issues.archived_at IS NULL
ORDER BY issues.due_at, CASE issues.priority WHEN 0 THEN 5 ELSE issues.priority END, issues.id;

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

-- name: CreateCommentBy :one
INSERT INTO issue_comments (issue_id, body, created_at, author) VALUES (?, ?, ?, ?) RETURNING *;

-- name: RecentComments :many
SELECT * FROM issue_comments WHERE issue_id = ? ORDER BY id DESC LIMIT 10;

-- name: OpenChecklistItems :many
SELECT it.text FROM issue_checklist_items it JOIN issue_checklists c ON c.id = it.checklist_id
WHERE c.issue_id = ? AND it.done = 0
ORDER BY c.position, c.id, it.position, it.id;

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

-- ---- B46 boards and lists ----

-- name: ListBoards :many
SELECT * FROM project_boards WHERE project_id = ? AND (archived_at IS NULL OR sqlc.arg(archived) = 1)
ORDER BY position, id;

-- name: GetBoard :one
SELECT * FROM project_boards WHERE id = ?;

-- name: StarredBoards :many
SELECT project_boards.*, projects.key AS project_key, projects.name AS project_name
FROM project_boards JOIN projects ON projects.id = project_boards.project_id
WHERE project_boards.starred = 1 AND project_boards.archived_at IS NULL AND projects.archived_at IS NULL
ORDER BY project_boards.updated_at DESC LIMIT 20;

-- name: FirstBoard :one
SELECT * FROM project_boards WHERE project_id = ? AND archived_at IS NULL ORDER BY position, id LIMIT 1;

-- name: MaxBoardPosition :one
SELECT CAST(COALESCE(MAX(position), 0) AS REAL) FROM project_boards WHERE project_id = ?;

-- name: CreateBoard :one
INSERT INTO project_boards (project_id, name, icon, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateBoard :one
UPDATE project_boards SET name = ?, icon = ?, position = ?, starred = ?, archived_at = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: DeleteBoard :exec
DELETE FROM project_boards WHERE id = ?;

-- name: ListLists :many
SELECT * FROM board_lists WHERE board_id = ? AND (archived_at IS NULL OR sqlc.arg(archived) = 1)
ORDER BY position, id;

-- name: ListListsForProject :many
SELECT board_lists.* FROM board_lists JOIN project_boards ON project_boards.id = board_lists.board_id
WHERE project_boards.project_id = ? AND board_lists.archived_at IS NULL
ORDER BY board_lists.board_id, board_lists.position, board_lists.id;

-- name: GetList :one
SELECT board_lists.*, project_boards.project_id FROM board_lists
JOIN project_boards ON project_boards.id = board_lists.board_id WHERE board_lists.id = ?;

-- name: ListForStatus :one
SELECT * FROM board_lists WHERE board_id = ? AND status = ? AND archived_at IS NULL
ORDER BY position, id LIMIT 1;

-- name: FirstList :one
SELECT * FROM board_lists WHERE board_id = ? AND archived_at IS NULL ORDER BY position, id LIMIT 1;

-- name: MaxListPosition :one
SELECT CAST(COALESCE(MAX(position), 0) AS REAL) FROM board_lists WHERE board_id = ?;

-- name: CreateList :one
INSERT INTO board_lists (board_id, name, position, status, color, wip_limit, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateList :one
UPDATE board_lists SET name = ?, position = ?, status = ?, color = ?, wip_limit = ?, collapsed = ?, archived_at = ?
WHERE id = ? RETURNING *;

-- name: DeleteList :exec
DELETE FROM board_lists WHERE id = ?;

-- name: CountListCards :many
SELECT list_id, count(*) AS n FROM issues
WHERE board_id = ? AND archived_at IS NULL AND list_id IS NOT NULL GROUP BY list_id;

-- name: SetIssuePlace :exec
UPDATE issues SET project_id = ?, number = ?, board_id = ?, list_id = ?, status = ?, sort_order = ?,
    completed_at = ?, updated_at = ?
WHERE id = ?;

-- name: SetIssueArchived :exec
UPDATE issues SET archived_at = ?, updated_at = ? WHERE id = ?;

-- name: ArchiveListCards :execrows
UPDATE issues SET archived_at = ?, updated_at = ? WHERE list_id = ? AND archived_at IS NULL;

-- name: ListArchivedIssues :many
SELECT sqlc.embed(issues), projects.key AS project_key
FROM issues JOIN projects ON projects.id = issues.project_id
WHERE issues.board_id = ? AND issues.archived_at IS NOT NULL ORDER BY issues.archived_at DESC LIMIT 200;

-- name: MembersForIssues :many
SELECT * FROM issue_members WHERE issue_id IN (sqlc.slice(issue_ids)) ORDER BY member_kind DESC, member_id;

-- name: ClearIssueMembers :exec
DELETE FROM issue_members WHERE issue_id = ?;

-- name: AddIssueMember :exec
INSERT OR IGNORE INTO issue_members (issue_id, member_kind, member_id) VALUES (?, ?, ?);

-- name: AddActivity :exec
INSERT INTO issue_activity (issue_id, at, actor, kind, data) VALUES (?, ?, ?, ?, ?);

-- name: ListActivity :many
SELECT * FROM issue_activity WHERE issue_id = ? ORDER BY id DESC LIMIT 200;

-- name: CommentCountsForIssues :many
SELECT issue_id, count(*) AS n FROM issue_comments WHERE issue_id IN (sqlc.slice(issue_ids)) GROUP BY issue_id;

-- name: RemoveForeignLabels :exec
DELETE FROM issue_labels WHERE issue_id = ? AND label_id IN
  (SELECT id FROM labels WHERE project_id IS NOT NULL AND project_id <> ?);

-- name: SetBoardPosition :exec
UPDATE project_boards SET position = ? WHERE id = ?;

-- name: SetListPosition :exec
UPDATE board_lists SET position = ? WHERE id = ?;

-- name: IssueIDsOnBoard :many
SELECT id FROM issues WHERE board_id = ? ORDER BY sort_order, id;

-- name: CountOpenInList :one
SELECT count(*) FROM issues WHERE list_id = ? AND archived_at IS NULL;

-- name: CountAllInList :one
SELECT count(*) FROM issues WHERE list_id = ?;

-- name: CopyChecklists :exec
INSERT INTO issue_checklists (issue_id, title, position)
SELECT CAST(sqlc.arg(to_issue) AS INTEGER), src.title, src.position FROM issue_checklists AS src
WHERE src.issue_id = sqlc.arg(from_issue);

-- name: CopyChecklistItems :exec
INSERT INTO issue_checklist_items (checklist_id, text, done, position, done_at)
SELECT n.id, it.text, it.done, it.position, it.done_at
FROM issue_checklist_items it
JOIN issue_checklists o ON o.id = it.checklist_id AND o.issue_id = sqlc.arg(from_issue)
JOIN issue_checklists n ON n.issue_id = sqlc.arg(to_issue) AND n.position = o.position AND n.title = o.title;

-- name: SetIssueColor :exec
-- B85
UPDATE issues SET color = ?, updated_at = ? WHERE id = ?;
