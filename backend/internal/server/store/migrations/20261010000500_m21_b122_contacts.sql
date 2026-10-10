-- +goose Up
-- B122: 联系人和重要日期。日期都是文本：last_contact_on 是 YYYY-MM-DD，空字符串表示没有。
-- events 是重要日期的 JSON 数组，每个有 id、kind、label、date（MM-DD 或 YYYY-MM-DD）。
-- notified_json 是已提醒的记录，每项 "<日期 id>:<年>:<天数>"。
-- lost_for 是上次“太久没联系”提醒对应的基准日期，基准变了就当作新的一轮。
CREATE TABLE contacts (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 name TEXT NOT NULL,
 group_kind TEXT NOT NULL DEFAULT 'other',
 events TEXT NOT NULL DEFAULT '[]',
 last_contact_on TEXT NOT NULL DEFAULT '',
 contact_every_days INTEGER NOT NULL DEFAULT 0,
 remind_days TEXT NOT NULL DEFAULT '7,1',
 notes TEXT NOT NULL DEFAULT '',
 notified_json TEXT NOT NULL DEFAULT '[]',
 lost_for TEXT NOT NULL DEFAULT '',
 archived_at DATETIME,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
);

-- +goose Down
SELECT 1;
