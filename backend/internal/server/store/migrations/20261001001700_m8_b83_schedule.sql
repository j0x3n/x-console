-- +goose Up
ALTER TABLE habits ADD COLUMN remind_when TEXT NOT NULL DEFAULT '["window"]';
ALTER TABLE habits ADD COLUMN active_host_ids TEXT NOT NULL DEFAULT '[]';
ALTER TABLE habits ADD COLUMN remind_on_host INTEGER NOT NULL DEFAULT 0;
ALTER TABLE habits ADD COLUMN template TEXT NOT NULL DEFAULT '';
ALTER TABLE habits ADD COLUMN snoozed_until DATETIME;
CREATE TABLE habit_schedule (
    id INTEGER PRIMARY KEY CHECK(id = 1),
    work_days TEXT NOT NULL,
    wake_time TEXT NOT NULL,
    sleep_time TEXT NOT NULL,
    work_start TEXT NOT NULL DEFAULT '',
    work_end TEXT NOT NULL DEFAULT '',
    timezone TEXT NOT NULL,
    idle_minutes INTEGER NOT NULL
);

-- +goose Down
SELECT 1;
