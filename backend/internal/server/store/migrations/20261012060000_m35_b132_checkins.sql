-- +goose Up
-- B132: 每天状态打卡。
-- checkin_questions.times 是 JSON 数组，元素 HH:MM。
-- checkin_answers.slot 是被回答的时间点 HH:MM，随手记一次用 manual。
-- checkin_asks 记每个时间点已经问过，保证一天只问一次。
CREATE TABLE checkin_questions (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 label TEXT NOT NULL,
 low_label TEXT NOT NULL DEFAULT '',
 high_label TEXT NOT NULL DEFAULT '',
 times TEXT NOT NULL DEFAULT '[]',
 enabled INTEGER NOT NULL DEFAULT 1,
 sort_order INTEGER NOT NULL DEFAULT 0,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
);

CREATE TABLE checkin_answers (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 question_id INTEGER NOT NULL REFERENCES checkin_questions(id) ON DELETE CASCADE,
 value INTEGER NOT NULL CHECK (value BETWEEN 1 AND 5),
 date TEXT NOT NULL,
 slot TEXT NOT NULL,
 answered_at DATETIME NOT NULL,
 source TEXT NOT NULL DEFAULT 'web',
 UNIQUE (question_id, date, slot)
);
CREATE INDEX checkin_answers_date ON checkin_answers(date);

CREATE TABLE checkin_asks (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 question_id INTEGER NOT NULL REFERENCES checkin_questions(id) ON DELETE CASCADE,
 date TEXT NOT NULL,
 slot TEXT NOT NULL,
 asked_at DATETIME NOT NULL,
 UNIQUE (question_id, date, slot)
);

INSERT INTO checkin_questions (label, low_label, high_label, times, enabled, sort_order, created_at, updated_at) VALUES
 ('现在精力怎么样', '很低', '很好', '["10:00","15:00"]', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
 ('现在压力大不大', '很轻松', '很大', '["20:00"]', 1, 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

-- +goose Down
DROP TABLE checkin_asks;
DROP TABLE checkin_answers;
DROP TABLE checkin_questions;
