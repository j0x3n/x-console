-- +goose Up
-- B202：目标可以跟着当天实际发出的提醒次数走（护眼、起身）。
ALTER TABLE habits ADD COLUMN target_mode TEXT NOT NULL DEFAULT 'fixed';
CREATE TABLE habit_prompts (
    habit_id INTEGER NOT NULL REFERENCES habits (id) ON DELETE CASCADE,
    day      TEXT    NOT NULL, -- 用户时区的日期 YYYY-MM-DD
    count    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (habit_id, day)
);

-- +goose Down
SELECT 1;
