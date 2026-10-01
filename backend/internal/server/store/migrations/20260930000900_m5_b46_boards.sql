-- B46 项目多看板：项目 → 看板 → 列表 → 卡片（卡片就是 issues）。
-- 列表可以对应一个状态：卡片拖进去时状态改成它。B36 的分类保留不删，界面不再用。

-- +goose Up
CREATE TABLE project_boards (
    id          INTEGER  PRIMARY KEY,
    project_id  INTEGER  NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name        TEXT     NOT NULL,
    icon        TEXT     NOT NULL DEFAULT '', -- 一个 emoji
    position    REAL     NOT NULL DEFAULT 0,
    starred     INTEGER  NOT NULL DEFAULT 0,
    archived_at DATETIME,
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL
);
CREATE INDEX project_boards_project ON project_boards (project_id, position);

CREATE TABLE board_lists (
    id          INTEGER  PRIMARY KEY,
    board_id    INTEGER  NOT NULL REFERENCES project_boards (id) ON DELETE CASCADE,
    name        TEXT     NOT NULL,
    position    REAL     NOT NULL DEFAULT 0,
    status      TEXT, -- 为空表示不改状态；否则是 issues.status 的一种
    color       TEXT     NOT NULL DEFAULT '',
    wip_limit   INTEGER  NOT NULL DEFAULT 0, -- 0 表示不限
    collapsed   INTEGER  NOT NULL DEFAULT 0,
    archived_at DATETIME,
    created_at  DATETIME NOT NULL
);
CREATE INDEX board_lists_board ON board_lists (board_id, position);

ALTER TABLE issues ADD COLUMN board_id INTEGER REFERENCES project_boards (id) ON DELETE SET NULL;
ALTER TABLE issues ADD COLUMN list_id INTEGER REFERENCES board_lists (id) ON DELETE SET NULL;
ALTER TABLE issues ADD COLUMN archived_at DATETIME;
ALTER TABLE issues ADD COLUMN cover_file_id INTEGER;
CREATE INDEX issues_list ON issues (list_id, sort_order);

CREATE TABLE issue_members (
    issue_id    INTEGER NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    member_kind TEXT    NOT NULL CHECK (member_kind IN ('me', 'agent')),
    member_id   TEXT    NOT NULL DEFAULT '', -- me 时为空，agent 时是 Agent id（B47）
    PRIMARY KEY (issue_id, member_kind, member_id)
);

CREATE TABLE issue_activity (
    id       INTEGER  PRIMARY KEY,
    issue_id INTEGER  NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    at       DATETIME NOT NULL,
    actor    TEXT     NOT NULL DEFAULT '', -- me、agent:<id>、token:<名字>、automation:<id>
    kind     TEXT     NOT NULL,
    data     TEXT     NOT NULL DEFAULT '{}'
);
CREATE INDEX issue_activity_issue ON issue_activity (issue_id, id);

-- 每个项目一个默认看板，6 个状态各一个列表，已有的 Issue 按状态放进去。
INSERT INTO project_boards (project_id, name, icon, position, created_at, updated_at)
SELECT id, '看板', '', 1024, created_at, created_at FROM projects;

INSERT INTO board_lists (board_id, name, position, status, created_at)
SELECT b.id, s.name, s.pos, s.status, b.created_at
FROM project_boards b
CROSS JOIN (
    SELECT 'backlog' AS status, '待规划' AS name, 1024 AS pos
    UNION ALL SELECT 'todo', '待办', 2048
    UNION ALL SELECT 'in_progress', '进行中', 3072
    UNION ALL SELECT 'in_review', '待审核', 4096
    UNION ALL SELECT 'done', '已完成', 5120
    UNION ALL SELECT 'canceled', '已取消', 6144
) s;

UPDATE issues SET
    board_id = (SELECT b.id FROM project_boards b WHERE b.project_id = issues.project_id),
    list_id = (SELECT l.id FROM board_lists l JOIN project_boards b ON b.id = l.board_id
               WHERE b.project_id = issues.project_id AND l.status = issues.status);

-- +goose Down
DROP TABLE issue_activity;
DROP TABLE issue_members;
DROP INDEX issues_list;
ALTER TABLE issues DROP COLUMN cover_file_id;
ALTER TABLE issues DROP COLUMN archived_at;
ALTER TABLE issues DROP COLUMN list_id;
ALTER TABLE issues DROP COLUMN board_id;
DROP TABLE board_lists;
DROP TABLE project_boards;
