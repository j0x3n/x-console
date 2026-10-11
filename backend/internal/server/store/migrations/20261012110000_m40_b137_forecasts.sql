-- +goose Up
-- B137: 预测和决策记录。kind: prediction、decision。
-- probability 是 1 到 99 的百分比，可以没有。due_date 是 YYYY-MM-DD。
-- outcome: yes 成了、no 没成、void 作废，没填结果时为空。notified 记到期提醒发过了。
CREATE TABLE forecasts (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 kind TEXT NOT NULL CHECK (kind IN ('prediction', 'decision')),
 statement TEXT NOT NULL,
 probability INTEGER CHECK (probability IS NULL OR (probability BETWEEN 1 AND 99)),
 due_date TEXT NOT NULL,
 reason TEXT NOT NULL DEFAULT '',
 outcome TEXT CHECK (outcome IS NULL OR outcome IN ('yes', 'no', 'void')),
 note TEXT NOT NULL DEFAULT '',
 notified INTEGER NOT NULL DEFAULT 0,
 created_at DATETIME NOT NULL,
 resolved_at DATETIME
);
CREATE INDEX forecasts_open ON forecasts(outcome, due_date);

-- +goose Down
DROP TABLE forecasts;
