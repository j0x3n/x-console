-- +goose Up
ALTER TABLE habits ADD COLUMN kind TEXT NOT NULL DEFAULT 'count' CHECK (kind IN ('count', 'workout'));
ALTER TABLE habit_logs ADD COLUMN workout_log_id INTEGER REFERENCES workout_logs (id) ON DELETE CASCADE;
CREATE INDEX habit_logs_workout_log ON habit_logs (workout_log_id);

-- +goose Down
-- These columns are kept on rollback to preserve existing habit and workout data.
SELECT 1;
