-- B23 订阅分类可以自己加，周期改成“每 N 个单位”。旧列 category、cycle、cycle_days 保留，写入时一起写。

-- +goose Up
CREATE TABLE subscription_categories (
    id         INTEGER PRIMARY KEY,
    name       TEXT     NOT NULL,
    builtin    TEXT,                     -- server、domain、saas、other；用户自己加的为 NULL
    position   INTEGER  NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX subscription_categories_name ON subscription_categories (name);
CREATE UNIQUE INDEX subscription_categories_builtin ON subscription_categories (builtin) WHERE builtin IS NOT NULL;
INSERT INTO subscription_categories (name, builtin, position, created_at) VALUES
    ('服务器', 'server', 0, CURRENT_TIMESTAMP),
    ('域名',   'domain', 1, CURRENT_TIMESTAMP),
    ('软件',   'saas',   2, CURRENT_TIMESTAMP),
    ('其他',   'other',  3, CURRENT_TIMESTAMP);

ALTER TABLE subscriptions ADD COLUMN category_id INTEGER REFERENCES subscription_categories (id);
UPDATE subscriptions SET category_id = (SELECT c.id FROM subscription_categories c WHERE c.builtin = subscriptions.category);

ALTER TABLE subscriptions ADD COLUMN cycle_count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE subscriptions ADD COLUMN cycle_unit  TEXT    NOT NULL DEFAULT 'month';
UPDATE subscriptions SET cycle_count = 1, cycle_unit = 'year' WHERE cycle = 'yearly';
UPDATE subscriptions SET cycle_count = max(cycle_days, 1), cycle_unit = 'day' WHERE cycle = 'custom_days';

-- +goose Down
-- 新增的列和表在回滚时保留，避免丢用户加的分类。
SELECT 1;
