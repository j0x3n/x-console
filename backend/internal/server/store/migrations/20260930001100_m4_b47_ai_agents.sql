-- B47 Agent 管理：AI Agent、Git 连接；仓库按远端登记、构建步骤；任务记下 Agent 和构建结果；评论记下作者。
-- 表名不用 agents，那是已配对的代理程序。

-- +goose Up
CREATE TABLE git_connections (
    id                 INTEGER PRIMARY KEY,
    kind               TEXT     NOT NULL CHECK (kind IN ('github', 'forgejo')), -- Gitea 和 Forgejo 接口一样，都算 forgejo
    name               TEXT     NOT NULL,
    base_url           TEXT     NOT NULL,                -- GitHub 是 API 地址，Forgejo 是站点地址
    username           TEXT     NOT NULL DEFAULT '',     -- 保存时检查令牌得到的用户名
    token_enc          TEXT     NOT NULL DEFAULT '',     -- secrets.Box 加密；用 GitHub 模块的令牌时为空
    use_github_module  INTEGER  NOT NULL DEFAULT 0,
    webhook_secret_enc TEXT     NOT NULL,                -- 回调签名密钥，secrets.Box 加密
    created_at         DATETIME NOT NULL,
    last_checked_at    DATETIME,
    last_error         TEXT     NOT NULL DEFAULT ''
);

CREATE TABLE ai_agents (
    id                 INTEGER PRIMARY KEY,
    name               TEXT     NOT NULL,
    avatar             TEXT     NOT NULL DEFAULT '',     -- 一个 emoji
    color              TEXT     NOT NULL DEFAULT '',
    kind               TEXT     NOT NULL CHECK (kind IN ('claude_code', 'codex', 'builtin')),
    model              TEXT     NOT NULL DEFAULT '',     -- builtin 是 <供应商 id>:<模型 id>；CLI 是传给 --model 的值
    instructions       TEXT     NOT NULL DEFAULT '',
    runner_agent_id    TEXT     REFERENCES agents (id) ON DELETE SET NULL,
    access             TEXT     NOT NULL DEFAULT 'write' CHECK (access IN ('read', 'write', 'write_delete')),
    cli_permission     TEXT     NOT NULL DEFAULT 'workspace' CHECK (cli_permission IN ('workspace', 'full')),
    repo_ids           TEXT     NOT NULL DEFAULT '[]',   -- JSON 数组，允许操作的 coding_repos.id
    max_parallel       INTEGER  NOT NULL DEFAULT 1,
    monthly_budget_usd REAL,
    auto_build         INTEGER  NOT NULL DEFAULT 1,
    build_retries      INTEGER  NOT NULL DEFAULT 2,
    enabled            INTEGER  NOT NULL DEFAULT 1,
    created_at         DATETIME NOT NULL,
    updated_at         DATETIME NOT NULL
);

-- connection_id 和 ai_agent_id 不加外键：带外键的列以后不能 DROP COLUMN。删除连接或 Agent 时由代码把它们置空。
ALTER TABLE coding_repos ADD COLUMN connection_id INTEGER;
ALTER TABLE coding_repos ADD COLUMN owner TEXT NOT NULL DEFAULT '';
ALTER TABLE coding_repos ADD COLUMN repo TEXT NOT NULL DEFAULT '';
ALTER TABLE coding_repos ADD COLUMN clone_url TEXT NOT NULL DEFAULT '';
-- JSON {"linux": [step], "windows": [step]}，step = {name, command, timeoutSeconds, artifacts}
ALTER TABLE coding_repos ADD COLUMN build_config TEXT NOT NULL DEFAULT '';

ALTER TABLE coding_tasks ADD COLUMN ai_agent_id INTEGER;
ALTER TABLE coding_tasks ADD COLUMN model TEXT NOT NULL DEFAULT '';
ALTER TABLE coding_tasks ADD COLUMN permission TEXT NOT NULL DEFAULT '';            -- '' 或 workspace 按执行器默认；full 不限制
ALTER TABLE coding_tasks ADD COLUMN build_status TEXT NOT NULL DEFAULT '';          -- ''、running、passed、failed
ALTER TABLE coding_tasks ADD COLUMN build_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE coding_tasks ADD COLUMN artifacts TEXT NOT NULL DEFAULT '[]';          -- JSON [{name, fileId, size}]
CREATE INDEX coding_tasks_ai_agent ON coding_tasks (ai_agent_id, status) WHERE ai_agent_id IS NOT NULL;

ALTER TABLE issue_comments ADD COLUMN author TEXT NOT NULL DEFAULT '';              -- '' 是用户自己，agent:<id> 是 Agent

-- +goose Down
ALTER TABLE issue_comments DROP COLUMN author;
DROP INDEX coding_tasks_ai_agent;
ALTER TABLE coding_tasks DROP COLUMN artifacts;
ALTER TABLE coding_tasks DROP COLUMN build_attempts;
ALTER TABLE coding_tasks DROP COLUMN build_status;
ALTER TABLE coding_tasks DROP COLUMN permission;
ALTER TABLE coding_tasks DROP COLUMN model;
ALTER TABLE coding_tasks DROP COLUMN ai_agent_id;
ALTER TABLE coding_repos DROP COLUMN build_config;
ALTER TABLE coding_repos DROP COLUMN clone_url;
ALTER TABLE coding_repos DROP COLUMN repo;
ALTER TABLE coding_repos DROP COLUMN owner;
ALTER TABLE coding_repos DROP COLUMN connection_id;
DROP TABLE ai_agents;
DROP TABLE git_connections;
