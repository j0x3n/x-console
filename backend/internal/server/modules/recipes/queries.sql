-- name: ListRecipes :many
SELECT * FROM recipes ORDER BY updated_at DESC, id DESC;

-- name: GetRecipe :one
SELECT * FROM recipes WHERE id = ?;

-- name: InsertRecipe :one
INSERT INTO recipes (title, summary, servings, total_minutes, ingredients, steps, tips, tags, source_note_id, source_url, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateRecipe :one
UPDATE recipes SET title = ?1, summary = ?2, servings = ?3, total_minutes = ?4, ingredients = ?5, steps = ?6,
  tips = ?7, tags = ?8, source_note_id = ?9, source_url = ?10, updated_at = ?11
WHERE id = ?12 RETURNING *;

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
