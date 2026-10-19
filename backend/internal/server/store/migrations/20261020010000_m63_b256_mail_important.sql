-- B256: AI 分拣多判断一项“重要”。只对通知有意义，用来在设置打开后只推重要的通知。

-- +goose Up
ALTER TABLE mail_triage ADD COLUMN important INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE mail_triage DROP COLUMN important;
