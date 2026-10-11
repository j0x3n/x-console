-- +goose Up
-- B135: AI 排今天的计划。date 是服务端时区的 YYYY-MM-DD。
-- status: draft、confirmed。source: ai、rule、manual。
-- dayplan_items.start_time、end_time 是 HH:MM，没排上时间为空字符串。
-- notified 记这件事的开始提醒发过了。kind: issue、reminder、carry、manual。
CREATE TABLE dayplans (
 date TEXT PRIMARY KEY,
 status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'confirmed')),
 source TEXT NOT NULL DEFAULT 'rule' CHECK (source IN ('ai', 'rule', 'manual')),
 generated_at DATETIME NOT NULL,
 confirmed_at DATETIME,
 review_sent INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE dayplan_items (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 plan_date TEXT NOT NULL REFERENCES dayplans(date) ON DELETE CASCADE,
 position INTEGER NOT NULL,
 title TEXT NOT NULL,
 kind TEXT NOT NULL DEFAULT 'manual',
 ref TEXT NOT NULL DEFAULT '',
 start_time TEXT NOT NULL DEFAULT '',
 end_time TEXT NOT NULL DEFAULT '',
 reason TEXT NOT NULL DEFAULT '',
 done INTEGER NOT NULL DEFAULT 0,
 notified INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX dayplan_items_plan ON dayplan_items(plan_date, position);

-- +goose Down
DROP TABLE dayplan_items;
DROP TABLE dayplans;
