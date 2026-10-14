-- +goose Up
-- B210: 营养分类和饮食记录。nutrition 是 JSON 文本列，值见 api/modules/recipes.yaml 的 RecipeNutrition。
-- meals 一行是一顿里的一样东西。date 是服务器时区的 YYYY-MM-DD。
-- recipe_id 在菜谱删除后清空，记录和名称留着。rating 是 1 到 5，可以空。
ALTER TABLE recipes ADD COLUMN nutrition TEXT NOT NULL DEFAULT '[]';

CREATE TABLE meals (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 date TEXT NOT NULL,
 slot TEXT NOT NULL,
 recipe_id INTEGER REFERENCES recipes(id) ON DELETE SET NULL,
 title TEXT NOT NULL,
 cooked BOOLEAN NOT NULL DEFAULT 0,
 rating INTEGER,
 note TEXT NOT NULL DEFAULT '',
 created_at DATETIME NOT NULL
);
CREATE INDEX idx_meals_date ON meals(date);
CREATE INDEX idx_meals_recipe ON meals(recipe_id);

-- +goose Down
SELECT 1;
