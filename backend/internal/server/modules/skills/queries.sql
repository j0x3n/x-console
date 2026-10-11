-- name: InsertSkill :one
INSERT INTO skills (name, weekly_target, created_at) VALUES (?, ?, ?) RETURNING *;

-- name: GetSkill :one
SELECT * FROM skills WHERE id = ?;

-- name: GetSkillByName :one
SELECT * FROM skills WHERE lower(name) = lower(?);

-- name: CountSkills :one
SELECT count(*) FROM skills;

-- name: ListSkills :many
SELECT * FROM skills WHERE archived = 0 OR ? = 1 ORDER BY archived, id;

-- name: UpdateSkill :one
UPDATE skills SET name = ?, weekly_target = ?, archived = ? WHERE id = ? RETURNING *;

-- name: DeleteSkill :execrows
DELETE FROM skills WHERE id = ?;

-- name: SkillTotals :many
SELECT skill_id, CAST(COALESCE(SUM(minutes), 0) AS INTEGER) AS total, MAX(practiced_on) AS last_on
FROM skill_sessions GROUP BY skill_id;

-- name: SkillDailyMinutes :many
SELECT skill_id, practiced_on, CAST(SUM(minutes) AS INTEGER) AS minutes
FROM skill_sessions WHERE practiced_on >= ? GROUP BY skill_id, practiced_on;

-- name: InsertSkillSession :one
INSERT INTO skill_sessions (skill_id, practiced_on, minutes, content, stuck, created_at) VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: GetSkillSession :one
SELECT * FROM skill_sessions WHERE id = ?;

-- name: CountSkillSessions :one
SELECT count(*) FROM skill_sessions WHERE skill_id = ?;

-- name: ListSkillSessions :many
SELECT * FROM skill_sessions WHERE skill_id = ? ORDER BY practiced_on DESC, id DESC LIMIT ?;

-- name: ListStuckSkillSessions :many
SELECT * FROM skill_sessions WHERE skill_id = ? AND stuck <> '' ORDER BY practiced_on DESC, id DESC LIMIT ?;

-- name: UpdateSkillSession :one
UPDATE skill_sessions SET practiced_on = ?, minutes = ?, content = ?, stuck = ? WHERE id = ? RETURNING *;

-- name: DeleteSkillSession :execrows
DELETE FROM skill_sessions WHERE id = ?;
