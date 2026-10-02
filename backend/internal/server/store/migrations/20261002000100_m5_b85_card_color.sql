-- +goose Up
-- B85: 卡片颜色（Trello 的封面色），空表示不设
ALTER TABLE issues ADD COLUMN color TEXT NOT NULL DEFAULT '';

-- +goose Down
SELECT 1;
