-- +goose Up
-- B128: 季度目标和每周复盘。
-- goals.status: active、done、dropped。quarter 是 1 到 4，按自然季度。
-- goal_results.source 为 manual 时 manual_value 是当前值，start_value 是起点；
-- 其余来源的值从别的模块算出来，ref 是习惯编号或电脑时间分类。
-- goal_result_logs 记手动关键结果每次改的值，用来算某一周变了多少。
-- goal_reviews 每周一条，week_start 是周一的日期（用户时区）。
-- snapshot 是保存时各目标进度的 JSON，历史不跟着目标后来的修改变。
CREATE TABLE goals (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 title TEXT NOT NULL,
 year INTEGER NOT NULL,
 quarter INTEGER NOT NULL CHECK (quarter BETWEEN 1 AND 4),
 note TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'done', 'dropped')),
 sort_order INTEGER NOT NULL DEFAULT 0,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
);
CREATE INDEX goals_period ON goals(year, quarter);

CREATE TABLE goal_results (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 goal_id INTEGER NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
 name TEXT NOT NULL,
 target REAL NOT NULL,
 unit TEXT NOT NULL DEFAULT '',
 start_value REAL NOT NULL DEFAULT 0,
 source TEXT NOT NULL DEFAULT 'manual',
 ref TEXT NOT NULL DEFAULT '',
 manual_value REAL NOT NULL DEFAULT 0,
 sort_order INTEGER NOT NULL DEFAULT 0,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
);
CREATE INDEX goal_results_goal ON goal_results(goal_id, sort_order);

CREATE TABLE goal_result_logs (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 result_id INTEGER NOT NULL REFERENCES goal_results(id) ON DELETE CASCADE,
 at DATETIME NOT NULL,
 value REAL NOT NULL
);
CREATE INDEX goal_result_logs_result ON goal_result_logs(result_id, at);

CREATE TABLE goal_reviews (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 week_start TEXT NOT NULL UNIQUE,
 went_well TEXT NOT NULL DEFAULT '',
 fell_short TEXT NOT NULL DEFAULT '',
 next_week TEXT NOT NULL DEFAULT '',
 snapshot TEXT NOT NULL DEFAULT '[]',
 progress REAL,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
);

-- +goose Down
SELECT 1;
