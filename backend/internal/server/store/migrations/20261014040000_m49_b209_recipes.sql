-- +goose Up
-- B209: 菜谱和做菜计时。食材、步骤、标签是 JSON 文本列，结构见 api/modules/recipes.yaml。
-- source_note_id 不加外键：笔记删了，菜谱照样留着，只是不能再点回笔记。
-- 计时器存在服务端，到点由后台任务检查。recipe_title 是开始时的菜名，菜谱删了也留着。
-- remaining_seconds 只在 paused 时有意义。done_at 是到点的时间，用来清理没关掉的计时器。
CREATE TABLE recipes (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 title TEXT NOT NULL,
 summary TEXT NOT NULL DEFAULT '',
 servings TEXT NOT NULL DEFAULT '',
 total_minutes INTEGER,
 ingredients TEXT NOT NULL DEFAULT '[]',
 steps TEXT NOT NULL DEFAULT '[]',
 tips TEXT NOT NULL DEFAULT '',
 tags TEXT NOT NULL DEFAULT '[]',
 source_note_id INTEGER,
 source_url TEXT NOT NULL DEFAULT '',
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
);
CREATE INDEX idx_recipes_updated ON recipes(updated_at DESC);

CREATE TABLE cook_timers (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 recipe_id INTEGER REFERENCES recipes(id) ON DELETE SET NULL,
 recipe_title TEXT NOT NULL DEFAULT '',
 step_index INTEGER,
 label TEXT NOT NULL,
 seconds INTEGER NOT NULL,
 state TEXT NOT NULL DEFAULT 'running',
 ends_at DATETIME,
 remaining_seconds INTEGER NOT NULL DEFAULT 0,
 done_at DATETIME,
 created_at DATETIME NOT NULL
);
CREATE INDEX idx_cook_timers_state ON cook_timers(state, ends_at);

-- +goose Down
SELECT 1;
