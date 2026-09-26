-- M9 Home Assistant：收藏的实体。实体状态只在内存里。

-- +goose Up
CREATE TABLE ha_favorites (
    entity_id  TEXT    PRIMARY KEY, -- 例如 light.living_room
    sort_order INTEGER NOT NULL DEFAULT 0,
    alias      TEXT    NOT NULL DEFAULT '' -- 自定义显示名，空表示用 HA 的 friendly_name
);

-- +goose Down
DROP TABLE ha_favorites;
