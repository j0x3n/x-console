-- +goose Up
-- B138: 技能练习记录。practiced_on 是 YYYY-MM-DD（服务端时区）。
-- weekly_target 是每周目标分钟数，可以没有。
CREATE TABLE skills (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 name TEXT NOT NULL,
 weekly_target INTEGER CHECK (weekly_target IS NULL OR weekly_target > 0),
 archived INTEGER NOT NULL DEFAULT 0,
 created_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX skills_name ON skills(lower(name));

CREATE TABLE skill_sessions (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 skill_id INTEGER NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
 practiced_on TEXT NOT NULL,
 minutes INTEGER NOT NULL CHECK (minutes BETWEEN 1 AND 1440),
 content TEXT NOT NULL DEFAULT '',
 stuck TEXT NOT NULL DEFAULT '',
 created_at DATETIME NOT NULL
);
CREATE INDEX skill_sessions_skill ON skill_sessions(skill_id, practiced_on);
CREATE INDEX skill_sessions_day ON skill_sessions(practiced_on);

-- +goose Down
DROP TABLE skill_sessions;
DROP TABLE skills;
