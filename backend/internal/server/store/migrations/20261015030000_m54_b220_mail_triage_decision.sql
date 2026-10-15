-- B220: 分拣结果推到今日页。decision 要做决定（批准、选方案、答应或拒绝），done_at 标成已处理的时间。
-- category 有 CHECK 约束，所以不加新类别，加两列。

-- +goose Up
ALTER TABLE mail_triage ADD COLUMN decision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mail_triage ADD COLUMN done_at DATETIME;

-- +goose Down
ALTER TABLE mail_triage DROP COLUMN done_at;
ALTER TABLE mail_triage DROP COLUMN decision;
