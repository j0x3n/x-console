-- name: ListFavorites :many
SELECT * FROM ha_favorites ORDER BY sort_order, entity_id;

-- name: DeleteAllFavorites :exec
DELETE FROM ha_favorites;

-- name: InsertFavorite :exec
INSERT INTO ha_favorites (entity_id, sort_order, alias) VALUES (?, ?, ?);
