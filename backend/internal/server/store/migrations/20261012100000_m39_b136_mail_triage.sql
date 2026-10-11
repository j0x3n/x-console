-- +goose Up
-- B136: 邮件 AI 分拣。category: action 要处理，notice 通知，promo 营销。
-- dates 是 JSON 数组，元素 {date, text}。source: snippet 只看了开头摘要，body 看了正文。
-- 哪些邮箱开了自动分拣放在设置键 mail.triage 里。
CREATE TABLE mail_triage (
 message_id INTEGER PRIMARY KEY REFERENCES mail_messages(id) ON DELETE CASCADE,
 category TEXT NOT NULL CHECK (category IN ('action', 'notice', 'promo')),
 summary TEXT NOT NULL DEFAULT '',
 dates TEXT NOT NULL DEFAULT '[]',
 source TEXT NOT NULL DEFAULT 'snippet',
 triaged_at DATETIME NOT NULL
);
CREATE INDEX mail_triage_category ON mail_triage(category, message_id);

-- +goose Down
DROP TABLE mail_triage;
