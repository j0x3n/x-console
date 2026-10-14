-- +goose Up
-- B206: AI 盯网页。kind 现在只有 web，B207 加 x。状态、模式不加 CHECK，以后加取值不用重建表。
-- *_sealed 列用 Secrets 加密：登录资料、出口代理、浏览器登录状态（Cookie 和本地存储）。
-- recipe 是 AI 记下的步骤（JSON），空字符串表示还没摸索过。
-- pending 是正在等用户处理的事（JSON），空字符串表示没有。
CREATE TABLE watchers (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 kind TEXT NOT NULL DEFAULT 'web',
 name TEXT NOT NULL,
 url TEXT NOT NULL,
 goal TEXT NOT NULL,
 allowed_hosts TEXT NOT NULL DEFAULT '[]',
 interval_minutes INTEGER NOT NULL DEFAULT 60,
 keepalive_minutes INTEGER NOT NULL DEFAULT 0,
 mode TEXT NOT NULL DEFAULT 'replay',
 enabled INTEGER NOT NULL DEFAULT 1,
 status TEXT NOT NULL DEFAULT 'new',
 status_detail TEXT NOT NULL DEFAULT '',
 current_value TEXT NOT NULL DEFAULT '',
 recipe TEXT NOT NULL DEFAULT '',
 pending TEXT NOT NULL DEFAULT '',
 fail_count INTEGER NOT NULL DEFAULT 0,
 login_sealed TEXT NOT NULL DEFAULT '',
 proxy_sealed TEXT NOT NULL DEFAULT '',
 state_sealed TEXT NOT NULL DEFAULT '',
 asks_day TEXT NOT NULL DEFAULT '',
 asks_count INTEGER NOT NULL DEFAULT 0,
 last_checked_at DATETIME,
 last_change_at DATETIME,
 last_keepalive_at DATETIME,
 next_check_at DATETIME,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
);
CREATE INDEX watchers_due ON watchers(enabled, next_check_at);

-- 一次检查、摸索、手动登录或保活。steps 是 JSON 数组 [{at,text,shotId}]。
CREATE TABLE watcher_runs (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 watcher_id INTEGER NOT NULL REFERENCES watchers(id) ON DELETE CASCADE,
 kind TEXT NOT NULL,
 outcome TEXT NOT NULL DEFAULT 'running',
 detail TEXT NOT NULL DEFAULT '',
 steps TEXT NOT NULL DEFAULT '[]',
 tokens INTEGER NOT NULL DEFAULT 0,
 started_at DATETIME NOT NULL,
 finished_at DATETIME
);
CREATE INDEX watcher_runs_watcher ON watcher_runs(watcher_id, id);

CREATE TABLE watcher_shots (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 watcher_id INTEGER NOT NULL REFERENCES watchers(id) ON DELETE CASCADE,
 data BLOB NOT NULL,
 created_at DATETIME NOT NULL
);
CREATE INDEX watcher_shots_watcher ON watcher_shots(watcher_id, created_at);

CREATE TABLE watcher_changes (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 watcher_id INTEGER NOT NULL REFERENCES watchers(id) ON DELETE CASCADE,
 old_value TEXT NOT NULL,
 new_value TEXT NOT NULL,
 summary TEXT NOT NULL DEFAULT '',
 important INTEGER NOT NULL DEFAULT 1,
 shot_id INTEGER REFERENCES watcher_shots(id) ON DELETE SET NULL,
 created_at DATETIME NOT NULL
);
CREATE INDEX watcher_changes_watcher ON watcher_changes(watcher_id, id);

-- +goose Down
DROP TABLE watcher_changes;
DROP TABLE watcher_shots;
DROP TABLE watcher_runs;
DROP TABLE watchers;
