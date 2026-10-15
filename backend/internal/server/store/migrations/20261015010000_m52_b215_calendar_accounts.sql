-- +goose Up
-- B215：Google 日历账号。client secret 和 refresh token 加密存在设置里
-- （calendar.account.<id>.client_secret、calendar.account.<id>.refresh_token）。
-- auth_error 是授权过期时的提示，重新授权后清空；auth_notified_at 用来一天只通知一次。
CREATE TABLE calendar_accounts (
    id               INTEGER PRIMARY KEY,
    provider         TEXT     NOT NULL DEFAULT 'google',
    email            TEXT     NOT NULL DEFAULT '',
    client_id        TEXT     NOT NULL DEFAULT '',
    auth_error       TEXT     NOT NULL DEFAULT '',
    auth_notified_at DATETIME,
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL
);

-- 账号下的日历仍是 kind = caldav，用 account_id 找到账号。remote_id 是 Google 的日历 ID。
-- read_only 是只读的日历（节假日、别人只读共享的）。
ALTER TABLE calendars ADD COLUMN account_id INTEGER REFERENCES calendar_accounts (id) ON DELETE CASCADE;
ALTER TABLE calendars ADD COLUMN remote_id TEXT NOT NULL DEFAULT '';
ALTER TABLE calendars ADD COLUMN read_only INTEGER NOT NULL DEFAULT 0;
CREATE INDEX calendars_account ON calendars (account_id);

-- +goose Down
SELECT 1;
