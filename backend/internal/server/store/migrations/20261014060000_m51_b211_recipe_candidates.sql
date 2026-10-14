-- +goose Up
-- B211: AI 联网找来的菜谱候选。draft 是和 RecipeInput 一样的 JSON，sources 是这次搜索用到的来源（标题和地址）。
-- status 是 pending、accepted 或 dismissed。recipe_id 是收进菜单后的菜谱，菜谱删了就清空。
-- recipe_discover_runs 按服务器时区的日期记每天找了几次，失败也算。
CREATE TABLE recipe_candidates (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 draft TEXT NOT NULL,
 reason TEXT NOT NULL DEFAULT '',
 sources TEXT NOT NULL DEFAULT '[]',
 status TEXT NOT NULL DEFAULT 'pending',
 recipe_id INTEGER REFERENCES recipes(id) ON DELETE SET NULL,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
);
CREATE INDEX idx_recipe_candidates_status ON recipe_candidates(status, id DESC);

CREATE TABLE recipe_discover_runs (
 day TEXT PRIMARY KEY,
 runs INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
SELECT 1;
