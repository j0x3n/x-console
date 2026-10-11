-- +goose Up
-- B162: 我的状态。me_states 是状态段，ended_at 为空的是现在这一段，存 180 天。
-- source 是 pc、phone、ha、manual、schedule。时间一律存 UTC。
CREATE TABLE me_states (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 state TEXT NOT NULL,
 source TEXT NOT NULL,
 guessed INTEGER NOT NULL DEFAULT 0,
 manual INTEGER NOT NULL DEFAULT 0,
 started_at DATETIME NOT NULL,
 ended_at DATETIME
);
CREATE INDEX me_states_started ON me_states(started_at);

-- me_reports 是手机和 Home Assistant 的上报，存 30 天，用来去重和查问题。
-- event 是 sleep.start、sleep.end、home.arrive、home.leave。
-- source 是 phone、ha、pc（电脑在用时结束睡觉）、manual（手动改状态时结束睡觉）。
CREATE TABLE me_reports (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 event TEXT NOT NULL,
 source TEXT NOT NULL,
 at DATETIME NOT NULL,
 created_at DATETIME NOT NULL
);
CREATE INDEX me_reports_at ON me_reports(at);

-- +goose Down
SELECT 1;
