-- B225: 验证码是标记，不是新类别。营销邮件不推送。

-- +goose Up
ALTER TABLE mail_triage ADD COLUMN code INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE mail_triage DROP COLUMN code;
