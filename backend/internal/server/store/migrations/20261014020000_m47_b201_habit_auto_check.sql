-- +goose Up
-- B201：按“我的状态”的入睡、起床时间自动打卡（早睡、早起）。
ALTER TABLE habits ADD COLUMN auto_check TEXT NOT NULL DEFAULT '';  -- '', bedtime, wake
ALTER TABLE habits ADD COLUMN auto_before TEXT NOT NULL DEFAULT ''; -- HH:MM

-- +goose Down
SELECT 1;
