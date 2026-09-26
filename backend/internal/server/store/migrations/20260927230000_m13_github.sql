-- M13 GitHub：关注仓库的缓存。每 5 分钟整体刷新一次。

-- +goose Up
CREATE TABLE github_pulls (
    id           INTEGER  PRIMARY KEY,
    repo         TEXT     NOT NULL, -- owner/name
    number       INTEGER  NOT NULL,
    title        TEXT     NOT NULL,
    author       TEXT     NOT NULL DEFAULT '',
    url          TEXT     NOT NULL,
    head_ref     TEXT     NOT NULL,
    head_sha     TEXT     NOT NULL DEFAULT '',
    base_ref     TEXT     NOT NULL,
    draft        BOOLEAN  NOT NULL DEFAULT 0,
    review_state TEXT     NOT NULL DEFAULT 'none', -- approved, changes_requested, pending, commented, none
    check_state  TEXT     NOT NULL DEFAULT 'none', -- success, failure, pending, none
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL,
    synced_at    DATETIME NOT NULL,
    UNIQUE (repo, number)
);

CREATE TABLE github_runs (
    id             INTEGER  PRIMARY KEY, -- GitHub 的 run id
    repo           TEXT     NOT NULL,
    workflow_id    INTEGER  NOT NULL,
    name           TEXT     NOT NULL,
    branch         TEXT     NOT NULL,
    event          TEXT     NOT NULL DEFAULT '',
    status         TEXT     NOT NULL, -- queued, in_progress, completed ...
    conclusion     TEXT     NOT NULL DEFAULT '', -- success, failure, cancelled ...
    url            TEXT     NOT NULL,
    head_sha       TEXT     NOT NULL DEFAULT '',
    default_branch BOOLEAN  NOT NULL DEFAULT 0,
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL
);
CREATE INDEX github_runs_repo ON github_runs (repo, created_at DESC);

CREATE TABLE github_issues (
    id         INTEGER  PRIMARY KEY,
    repo       TEXT     NOT NULL,
    number     INTEGER  NOT NULL,
    title      TEXT     NOT NULL,
    url        TEXT     NOT NULL,
    author     TEXT     NOT NULL DEFAULT '',
    assignees  TEXT     NOT NULL DEFAULT '[]', -- JSON 数组
    labels     TEXT     NOT NULL DEFAULT '[]', -- JSON 数组
    relation   TEXT     NOT NULL, -- assigned, created, both
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE (repo, number)
);

-- PR 和本地东西的关联。kind：issue（ref 是 XC-12）、coding_task（ref 是任务 id）。
CREATE TABLE github_links (
    id         INTEGER  PRIMARY KEY,
    repo       TEXT     NOT NULL,
    number     INTEGER  NOT NULL,
    kind       TEXT     NOT NULL,
    ref        TEXT     NOT NULL,
    created_at DATETIME NOT NULL,
    UNIQUE (repo, number, kind, ref)
);

-- 最近一次确定的 CI 结果，用来判断“从成功变失败”。
-- key：pr:owner/name#12 或 run:owner/name:<workflow_id>。state：success、failure。
CREATE TABLE github_ci_states (
    key        TEXT     PRIMARY KEY,
    state      TEXT     NOT NULL,
    updated_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE github_ci_states;
DROP TABLE github_links;
DROP TABLE github_issues;
DROP TABLE github_runs;
DROP TABLE github_pulls;
