-- +goose Up
CREATE TABLE ai_agent_decisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL REFERENCES ai_agent_runs(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    options TEXT NOT NULL DEFAULT '[]',
    dangerous INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    answer TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);
ALTER TABLE coding_tasks ADD COLUMN waiting_question TEXT NOT NULL DEFAULT '';

-- +goose Down
SELECT 1;
