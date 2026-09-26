-- M5 项目：项目、Issue、标签、里程碑、关联、评论。
-- 时间列由 Go 写入 UTC。due_date 是本地日期，存成 YYYY-MM-DD 文本，方便按日期比较。

-- +goose Up
CREATE TABLE projects (
    id          INTEGER PRIMARY KEY,
    key         TEXT     NOT NULL UNIQUE, -- 2 到 5 个大写字母，例如 XC
    name        TEXT     NOT NULL,
    description TEXT     NOT NULL DEFAULT '',
    color       TEXT     NOT NULL DEFAULT '',
    icon        TEXT     NOT NULL DEFAULT '',
    archived_at DATETIME,
    next_number INTEGER  NOT NULL DEFAULT 1, -- 下一个 Issue 编号，只增不减
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL
);

CREATE TABLE milestones (
    id         INTEGER PRIMARY KEY,
    project_id INTEGER  NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name       TEXT     NOT NULL,
    due_date   TEXT, -- YYYY-MM-DD
    created_at DATETIME NOT NULL
);
CREATE INDEX milestones_project ON milestones (project_id);

CREATE TABLE issues (
    id              INTEGER PRIMARY KEY,
    project_id      INTEGER  NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    number          INTEGER  NOT NULL,
    title           TEXT     NOT NULL,
    description     TEXT     NOT NULL DEFAULT '', -- Markdown
    status          TEXT     NOT NULL DEFAULT 'todo'
        CHECK (status IN ('backlog', 'todo', 'in_progress', 'in_review', 'done', 'canceled')),
    priority        INTEGER  NOT NULL DEFAULT 0 CHECK (priority BETWEEN 0 AND 4), -- 0 无 1 紧急 2 高 3 中 4 低
    due_date        TEXT, -- YYYY-MM-DD
    milestone_id    INTEGER  REFERENCES milestones (id) ON DELETE SET NULL,
    sort_order      REAL     NOT NULL DEFAULT 0, -- 看板同一列内的顺序，小的在上
    external_source TEXT     NOT NULL DEFAULT '', -- 例如 linear
    external_id     TEXT     NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    completed_at    DATETIME,
    UNIQUE (project_id, number)
);
CREATE INDEX issues_board ON issues (project_id, status, sort_order);
CREATE INDEX issues_updated ON issues (updated_at);
CREATE INDEX issues_due ON issues (due_date);
CREATE UNIQUE INDEX issues_external ON issues (external_source, external_id) WHERE external_id <> '';

CREATE TABLE labels (
    id         INTEGER PRIMARY KEY,
    project_id INTEGER REFERENCES projects (id) ON DELETE CASCADE, -- 空表示全局标签
    name       TEXT    NOT NULL,
    color      TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX labels_project ON labels (project_id);

CREATE TABLE issue_labels (
    issue_id INTEGER NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    label_id INTEGER NOT NULL REFERENCES labels (id) ON DELETE CASCADE,
    PRIMARY KEY (issue_id, label_id)
);
CREATE INDEX issue_labels_label ON issue_labels (label_id);

CREATE TABLE issue_links (
    id         INTEGER PRIMARY KEY,
    issue_id   INTEGER  NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    kind       TEXT     NOT NULL, -- pull_request / coding_task / note / url
    title      TEXT     NOT NULL DEFAULT '',
    url        TEXT     NOT NULL,
    ref        TEXT     NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);
CREATE INDEX issue_links_issue ON issue_links (issue_id);

CREATE TABLE issue_comments (
    id         INTEGER PRIMARY KEY,
    issue_id   INTEGER  NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    body       TEXT     NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX issue_comments_issue ON issue_comments (issue_id);

-- +goose Down
DROP TABLE issue_comments;
DROP TABLE issue_links;
DROP TABLE issue_labels;
DROP TABLE labels;
DROP TABLE issues;
DROP TABLE milestones;
DROP TABLE projects;
