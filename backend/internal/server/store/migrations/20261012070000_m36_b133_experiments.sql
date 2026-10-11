-- +goose Up
-- B133: 自我实验。日期是服务端时区的 YYYY-MM-DD 文本。
-- 基线期 [baseline_start, change_start)，改变期 [change_start, end_date]。
-- experiment_metrics.key 是 checkin:<问题编号>、body:<指标>、goal:<种类>:<选项>。
-- label、unit、subjective 是建实验时的快照，来源模块以后改名不影响老实验的显示。
CREATE TABLE experiments (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 title TEXT NOT NULL,
 change_text TEXT NOT NULL DEFAULT '',
 baseline_start TEXT NOT NULL,
 change_start TEXT NOT NULL,
 end_date TEXT NOT NULL,
 note TEXT NOT NULL DEFAULT '',
 conclusion TEXT NOT NULL DEFAULT '',
 stopped INTEGER NOT NULL DEFAULT 0,
 retrospective INTEGER NOT NULL DEFAULT 0,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
);

CREATE TABLE experiment_metrics (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 experiment_id INTEGER NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
 key TEXT NOT NULL,
 label TEXT NOT NULL,
 unit TEXT NOT NULL DEFAULT '',
 subjective INTEGER NOT NULL DEFAULT 0,
 sort_order INTEGER NOT NULL DEFAULT 0,
 UNIQUE (experiment_id, key)
);

-- +goose Down
DROP TABLE experiment_metrics;
DROP TABLE experiments;
