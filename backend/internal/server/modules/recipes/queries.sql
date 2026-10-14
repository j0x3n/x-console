-- name: ListRecipes :many
SELECT * FROM recipes ORDER BY updated_at DESC, id DESC;

-- name: GetRecipe :one
SELECT * FROM recipes WHERE id = ?;

-- name: InsertRecipe :one
INSERT INTO recipes (title, summary, servings, total_minutes, ingredients, steps, tips, tags, source_note_id, source_url, nutrition, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateRecipe :one
UPDATE recipes SET title = ?1, summary = ?2, servings = ?3, total_minutes = ?4, ingredients = ?5, steps = ?6,
  tips = ?7, tags = ?8, source_note_id = ?9, source_url = ?10, nutrition = ?11, updated_at = ?12
WHERE id = ?13 RETURNING *;

-- name: UpdateRecipeNutrition :exec
UPDATE recipes SET nutrition = ?1 WHERE id = ?2;

-- name: ListRecipesWithoutNutrition :many
SELECT * FROM recipes WHERE nutrition = '[]' ORDER BY updated_at DESC, id DESC LIMIT ?;

-- name: CountRecipesWithoutNutrition :one
SELECT COUNT(*) FROM recipes WHERE nutrition = '[]';

-- name: DeleteRecipe :execrows
DELETE FROM recipes WHERE id = ?;

-- name: ListCookTimers :many
SELECT * FROM cook_timers ORDER BY id;

-- name: GetCookTimer :one
SELECT * FROM cook_timers WHERE id = ?;

-- name: CountCookTimers :one
SELECT COUNT(*) FROM cook_timers;

-- name: InsertCookTimer :one
INSERT INTO cook_timers (recipe_id, recipe_title, step_index, label, seconds, state, ends_at, remaining_seconds, created_at)
VALUES (?, ?, ?, ?, ?, 'running', ?, 0, ?)
RETURNING *;

-- name: UpdateCookTimerState :one
UPDATE cook_timers SET state = ?1, ends_at = ?2, remaining_seconds = ?3, done_at = ?4 WHERE id = ?5 RETURNING *;

-- name: DeleteCookTimer :execrows
DELETE FROM cook_timers WHERE id = ?;

-- name: ListDueCookTimers :many
SELECT * FROM cook_timers WHERE state = 'running' AND ends_at <= ? ORDER BY ends_at, id;

-- name: FinishCookTimer :execrows
UPDATE cook_timers SET state = 'done', done_at = ?1, remaining_seconds = 0
WHERE id = ?2 AND state = 'running' AND ends_at <= ?1;

-- name: ListStaleDoneCookTimers :many
SELECT id FROM cook_timers WHERE state = 'done' AND done_at <= ?;

-- name: RecipeMealStats :many
SELECT recipe_id AS recipe_id,
  CAST(SUM(CASE WHEN cooked THEN 1 ELSE 0 END) AS INTEGER) AS cooked_count,
  CAST(COALESCE(MAX(CASE WHEN cooked THEN date END), '') AS TEXT) AS last_cooked_on,
  CAST(COALESCE(SUM(rating), 0) AS INTEGER) AS rating_sum,
  COUNT(rating) AS rating_count
FROM meals WHERE recipe_id IS NOT NULL GROUP BY recipe_id;

-- name: RecipeLastEaten :many
SELECT recipe_id AS recipe_id, CAST(MAX(date) AS TEXT) AS last_eaten_on
FROM meals WHERE recipe_id IS NOT NULL GROUP BY recipe_id;

-- name: ListMeals :many
SELECT * FROM meals WHERE date >= ?1 AND date <= ?2
ORDER BY date DESC,
  CASE slot WHEN 'breakfast' THEN 0 WHEN 'lunch' THEN 1 WHEN 'dinner' THEN 2 ELSE 3 END, id;

-- name: GetMeal :one
SELECT * FROM meals WHERE id = ?;

-- name: InsertMeal :one
INSERT INTO meals (date, slot, recipe_id, title, cooked, rating, note, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateMeal :one
UPDATE meals SET date = ?1, slot = ?2, recipe_id = ?3, title = ?4, cooked = ?5, rating = ?6, note = ?7
WHERE id = ?8 RETURNING *;

-- name: DeleteMeal :execrows
DELETE FROM meals WHERE id = ?;

-- name: ListRecipeCandidatesByStatus :many
SELECT * FROM recipe_candidates WHERE status = ? ORDER BY id DESC;

-- name: GetRecipeCandidate :one
SELECT * FROM recipe_candidates WHERE id = ?;

-- name: InsertRecipeCandidate :one
INSERT INTO recipe_candidates (draft, reason, sources, status, created_at, updated_at)
VALUES (?, ?, ?, 'pending', ?, ?)
RETURNING *;

-- name: UpdateRecipeCandidateStatus :one
UPDATE recipe_candidates SET status = ?1, recipe_id = ?2, updated_at = ?3
WHERE id = ?4 AND status = 'pending' RETURNING *;

-- name: DeleteOldRecipeCandidates :execrows
DELETE FROM recipe_candidates WHERE status != 'pending' AND updated_at < ?;

-- name: GetRecipeDiscoverRuns :one
SELECT runs FROM recipe_discover_runs WHERE day = ?;

-- name: DeleteOldRecipeDiscoverRuns :exec
DELETE FROM recipe_discover_runs WHERE day < ?;

-- name: LikedRecipeIDs :many
SELECT DISTINCT CAST(recipe_id AS INTEGER) AS recipe_id FROM meals
WHERE cooked = 1 AND rating >= 4 AND recipe_id IS NOT NULL;
