-- M4 编码任务：登记的仓库、任务和执行器输出。
-- 仓库在代理那台机器上，路径是代理机器上的绝对路径。

-- +goose Up
CREATE TABLE coding_repos (
    id             INTEGER PRIMARY KEY,
    agent_id       TEXT     NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    path           TEXT     NOT NULL,
    name           TEXT     NOT NULL,
    default_branch TEXT     NOT NULL DEFAULT '',
    remote_url     TEXT     NOT NULL DEFAULT '',
    github_repo    TEXT     NOT NULL DEFAULT '', -- owner/name，从远端地址解析
    created_at     DATETIME NOT NULL,
    UNIQUE (agent_id, path)
);

CREATE TABLE coding_tasks (
    id              INTEGER PRIMARY KEY,
    repo_id         INTEGER  NOT NULL REFERENCES coding_repos (id) ON DELETE CASCADE,
    issue_key       TEXT     NOT NULL DEFAULT '',
    executor        TEXT     NOT NULL CHECK (executor IN ('claude', 'codex')),
    prompt          TEXT     NOT NULL,
    base_branch     TEXT     NOT NULL DEFAULT '',
    branch          TEXT     NOT NULL DEFAULT '',
    base_commit     TEXT     NOT NULL DEFAULT '', -- 分支起点，代理建 worktree 时上报
    status          TEXT     NOT NULL DEFAULT 'queued' CHECK (status IN
                        ('queued', 'running', 'review', 'failed', 'canceled', 'committed', 'pushed', 'pr_opened', 'discarded')),
    exit_code       INTEGER,
    error           TEXT     NOT NULL DEFAULT '',
    commit_sha      TEXT     NOT NULL DEFAULT '',
    pr_url          TEXT     NOT NULL DEFAULT '',
    changed_files   TEXT     NOT NULL DEFAULT '[]', -- JSON [{path,status,additions,deletions,binary}]，执行器退出时记录
    timeout_minutes INTEGER  NOT NULL DEFAULT 60,
    created_at      DATETIME NOT NULL,
    started_at      DATETIME,
    finished_at     DATETIME,
    updated_at      DATETIME NOT NULL
);
CREATE INDEX coding_tasks_status ON coding_tasks (status, id);
CREATE INDEX coding_tasks_repo ON coding_tasks (repo_id, id);
CREATE INDEX coding_tasks_issue ON coding_tasks (issue_key) WHERE issue_key != '';

CREATE TABLE coding_task_events (
    id      INTEGER PRIMARY KEY,
    task_id INTEGER  NOT NULL REFERENCES coding_tasks (id) ON DELETE CASCADE,
    seq     INTEGER  NOT NULL,
    at      DATETIME NOT NULL,
    kind    TEXT     NOT NULL,
    text    TEXT     NOT NULL DEFAULT '',
    data    TEXT     NOT NULL DEFAULT '', -- JSON 对象，没有时为空串
    UNIQUE (task_id, seq)
);

-- +goose Down
DROP TABLE coding_task_events;
DROP TABLE coding_tasks;
DROP TABLE coding_repos;
