-- B127 通知合并推送：暂存队列。存的是通知的副本，通知中心里的记录不受影响。
-- 时间一律由 Go 写入 UTC。

-- +goose Up
CREATE TABLE notify_digest_queue (
    id              INTEGER  PRIMARY KEY,
    notification_id INTEGER  NOT NULL DEFAULT 0, -- 通知中心里的记录，仅供参考
    created_at      DATETIME NOT NULL,           -- 通知产生的时间
    kind            TEXT     NOT NULL,
    title           TEXT     NOT NULL,
    body            TEXT     NOT NULL DEFAULT '',
    link            TEXT     NOT NULL DEFAULT '',
    priority        TEXT     NOT NULL DEFAULT 'normal',
    source          TEXT     NOT NULL DEFAULT '',
    category        TEXT     NOT NULL DEFAULT 'other'
);

-- +goose Down
DROP TABLE notify_digest_queue;
