-- name: GetCard :one
SELECT * FROM cards WHERE id = ? LIMIT 1;

-- name: InsertCard :one
INSERT INTO cards (deck, question, answer, source_title, source_link, due, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateCardContent :one
UPDATE cards SET deck = ?, question = ?, answer = ?, source_title = ?, source_link = ?, suspended = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: UpdateCardSchedule :one
UPDATE cards SET state = ?, stability = ?, difficulty = ?, due = ?, last_review = ?, scheduled_days = ?,
  reps = ?, lapses = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: DeleteCard :execrows
DELETE FROM cards WHERE id = ?;

-- name: InsertCardReview :exec
INSERT INTO card_reviews (card_id, rating, state_before, reviewed_at, scheduled_days, stability, difficulty)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListCards :many
SELECT * FROM cards
WHERE id < CAST(sqlc.arg(before) AS INTEGER)
  AND (instr(lower(question), lower(CAST(sqlc.arg(q) AS TEXT))) > 0 OR instr(lower(answer), lower(CAST(sqlc.arg(q) AS TEXT))) > 0)
  AND (CAST(sqlc.arg(deck) AS TEXT) = '' OR deck = CAST(sqlc.arg(deck) AS TEXT))
  AND (CAST(sqlc.arg(state) AS INTEGER) < 0 OR state = CAST(sqlc.arg(state) AS INTEGER))
  AND (CAST(sqlc.arg(suspended) AS INTEGER) < 0 OR suspended = CAST(sqlc.arg(suspended) AS INTEGER))
ORDER BY id DESC
LIMIT CAST(sqlc.arg(max_rows) AS INTEGER);

-- name: CountCards :one
SELECT COUNT(*) FROM cards
WHERE (instr(lower(question), lower(CAST(sqlc.arg(q) AS TEXT))) > 0 OR instr(lower(answer), lower(CAST(sqlc.arg(q) AS TEXT))) > 0)
  AND (CAST(sqlc.arg(deck) AS TEXT) = '' OR deck = CAST(sqlc.arg(deck) AS TEXT))
  AND (CAST(sqlc.arg(state) AS INTEGER) < 0 OR state = CAST(sqlc.arg(state) AS INTEGER))
  AND (CAST(sqlc.arg(suspended) AS INTEGER) < 0 OR suspended = CAST(sqlc.arg(suspended) AS INTEGER));

-- name: ListDueCards :many
SELECT * FROM cards
WHERE suspended = 0 AND state <> 0 AND due < sqlc.arg(until)
  AND (CAST(sqlc.arg(deck) AS TEXT) = '' OR deck = CAST(sqlc.arg(deck) AS TEXT))
ORDER BY due, id
LIMIT CAST(sqlc.arg(max_rows) AS INTEGER);

-- name: ListNewCards :many
SELECT * FROM cards
WHERE suspended = 0 AND state = 0
  AND (CAST(sqlc.arg(deck) AS TEXT) = '' OR deck = CAST(sqlc.arg(deck) AS TEXT))
ORDER BY id
LIMIT CAST(sqlc.arg(max_rows) AS INTEGER);

-- name: CountDueCards :one
SELECT COUNT(*) FROM cards
WHERE suspended = 0 AND state <> 0 AND due < sqlc.arg(until)
  AND (CAST(sqlc.arg(deck) AS TEXT) = '' OR deck = CAST(sqlc.arg(deck) AS TEXT));

-- name: CountNewCards :one
SELECT COUNT(*) FROM cards
WHERE suspended = 0 AND state = 0
  AND (CAST(sqlc.arg(deck) AS TEXT) = '' OR deck = CAST(sqlc.arg(deck) AS TEXT));

-- name: CountAllCards :one
SELECT COUNT(*) FROM cards
WHERE (CAST(sqlc.arg(deck) AS TEXT) = '' OR deck = CAST(sqlc.arg(deck) AS TEXT));

-- name: CountSuspendedCards :one
SELECT COUNT(*) FROM cards
WHERE suspended = 1 AND (CAST(sqlc.arg(deck) AS TEXT) = '' OR deck = CAST(sqlc.arg(deck) AS TEXT));

-- name: NextDueCard :one
SELECT due FROM cards
WHERE suspended = 0 AND state <> 0 AND due >= sqlc.arg(since)
  AND (CAST(sqlc.arg(deck) AS TEXT) = '' OR deck = CAST(sqlc.arg(deck) AS TEXT))
ORDER BY due
LIMIT 1;

-- name: CountReviewsBetween :one
SELECT COUNT(*) FROM card_reviews
WHERE reviewed_at >= sqlc.arg(from_at) AND reviewed_at < sqlc.arg(until)
  AND ((CAST(sqlc.arg(only_new) AS INTEGER) = 1 AND state_before = 0) OR (CAST(sqlc.arg(only_new) AS INTEGER) = 0 AND state_before <> 0));

-- name: CountRecallReviews :one
SELECT COUNT(*) FROM card_reviews
WHERE reviewed_at >= sqlc.arg(since) AND state_before <> 0;

-- name: CountPassedReviews :one
SELECT COUNT(*) FROM card_reviews
WHERE reviewed_at >= sqlc.arg(since) AND state_before <> 0 AND rating > 1;

-- name: ListDecks :many
SELECT deck AS name, COUNT(*) AS total,
  CAST(COALESCE(SUM(CASE WHEN suspended = 0 AND state <> 0 AND due < sqlc.arg(until) THEN 1 ELSE 0 END), 0) AS INTEGER) AS due,
  CAST(COALESCE(SUM(CASE WHEN suspended = 0 AND state = 0 THEN 1 ELSE 0 END), 0) AS INTEGER) AS new_cards
FROM cards GROUP BY deck ORDER BY deck;
