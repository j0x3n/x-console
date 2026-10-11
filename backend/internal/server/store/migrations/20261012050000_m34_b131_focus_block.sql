-- +goose Up
-- B131: what each computer last said about the focus block. One row per
-- computer. The lists and switches are in the settings table, not here.
CREATE TABLE focus_block_agents (
    agent_id TEXT PRIMARY KEY,
    -- blocking: the panel believes the block is on. idle: it is off. error: the last call failed.
    state TEXT NOT NULL DEFAULT 'idle',
    session_id INTEGER NOT NULL DEFAULT 0,
    -- Hash of the lists that were sent, to tell whether a block is out of date.
    config_hash TEXT NOT NULL DEFAULT '',
    hosts_status TEXT NOT NULL DEFAULT '',
    hosts_error TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    sites INTEGER NOT NULL DEFAULT 0,
    killed INTEGER NOT NULL DEFAULT 0,
    -- JSON array of the entries the agent did not use.
    refused TEXT NOT NULL DEFAULT '[]',
    updated_at DATETIME NOT NULL
);

-- +goose Down
SELECT 1;
