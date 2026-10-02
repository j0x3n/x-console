-- +goose Up
CREATE TABLE ai_agent_runs (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 agent_id INTEGER NOT NULL REFERENCES ai_agents(id) ON DELETE CASCADE,
 issue_key TEXT NOT NULL DEFAULT '',
 issue_title TEXT NOT NULL DEFAULT '',
 kind TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'queued',
 task_id INTEGER UNIQUE,
 open_pr INTEGER NOT NULL DEFAULT 0,
 pr_url TEXT NOT NULL DEFAULT '',
 summary TEXT NOT NULL DEFAULT '',
 created_at DATETIME NOT NULL,
 started_at DATETIME,
 finished_at DATETIME
);
CREATE INDEX ai_agent_runs_agent ON ai_agent_runs(agent_id,id);
ALTER TABLE coding_tasks ADD COLUMN auto_open_pr INTEGER NOT NULL DEFAULT 0;
CREATE TABLE ai_agent_run_events (
 run_id INTEGER NOT NULL REFERENCES ai_agent_runs(id) ON DELETE CASCADE,
 seq INTEGER NOT NULL,
 at DATETIME NOT NULL,
 kind TEXT NOT NULL,
 text TEXT NOT NULL DEFAULT '',
 tool TEXT NOT NULL DEFAULT '',
 ok INTEGER,
 PRIMARY KEY(run_id,seq)
);

-- +goose Down
SELECT 1;
