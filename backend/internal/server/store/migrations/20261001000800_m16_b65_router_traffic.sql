-- +goose Up
-- B65：WAN 口每分钟的流量，留 30 天。rx、tx 是这一分钟收发的字节数。
CREATE TABLE router_traffic (
    at      INTEGER PRIMARY KEY, -- Unix 秒
    seconds INTEGER NOT NULL,    -- 和上一次采样隔了多少秒
    rx      INTEGER NOT NULL,
    tx      INTEGER NOT NULL
);

-- +goose Down
DROP TABLE router_traffic;
