-- +goose Up
-- B241: 资讯页面的设置，单行（id 固定为 1）。数据不存在面板里，每次经过代理去虚拟机上的小接口读。
-- agent_id 是选的机器，不加外键：机器吊销后设置留着，页面显示机器不在线。
-- token_enc 是小接口令牌，用主密钥加密，只写不读。
CREATE TABLE feeds_settings (
 id INTEGER PRIMARY KEY CHECK (id = 1),
 agent_id TEXT NOT NULL DEFAULT '',
 base_url TEXT NOT NULL DEFAULT '',
 token_enc TEXT NOT NULL DEFAULT '',
 updated_at DATETIME NOT NULL
);

-- +goose Down
SELECT 1;
