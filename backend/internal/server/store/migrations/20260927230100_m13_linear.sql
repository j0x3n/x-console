-- M13 Linear：团队和本地项目的对应，以及每个 Issue 上次同步时两边的更新时间。

-- +goose Up
CREATE TABLE linear_teams (
    team_id    TEXT     PRIMARY KEY, -- Linear 团队 id
    team_key   TEXT     NOT NULL,
    team_name  TEXT     NOT NULL,
    project_id INTEGER  NOT NULL UNIQUE, -- 本地项目 id
    cursor     DATETIME, -- 已经拉过的最新 updatedAt，空表示还没拉过
    created_at DATETIME NOT NULL
);

CREATE TABLE linear_issues (
    linear_id         TEXT     PRIMARY KEY, -- Linear issue id，等于本地 issues.external_id
    issue_key         TEXT     NOT NULL, -- 本地 key，例如 XC-12
    identifier        TEXT     NOT NULL DEFAULT '', -- Linear 的编号，例如 ENG-7
    remote_updated_at DATETIME NOT NULL, -- 上次同步时 Linear 的 updatedAt
    local_updated_at  DATETIME NOT NULL, -- 上次同步时本地的 updated_at
    synced_at         DATETIME NOT NULL
);
CREATE INDEX linear_issues_key ON linear_issues (issue_key);

-- +goose Down
DROP TABLE linear_issues;
DROP TABLE linear_teams;
