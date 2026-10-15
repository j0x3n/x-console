-- B208: 网站自动化的浏览器可以经过一台已连接的机器上网。空字符串是不经过。

-- +goose Up
ALTER TABLE watchers ADD COLUMN egress_agent_id TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE watchers DROP COLUMN egress_agent_id;
