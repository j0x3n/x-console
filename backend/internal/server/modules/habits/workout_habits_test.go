package habits_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
)

func TestWorkoutHabitFollowsWorkoutLog(t *testing.T) {
	env, _ := setup(t)
	var workout api.Habit
	env.MustDo(http.MethodPost, "/habits", map[string]any{
		"name": "力量训练", "kind": "workout", "unit": "组", "dailyTarget": 1,
	}, &workout)
	if workout.Kind == nil || *workout.Kind != "workout" || workout.Unit != "次" {
		t.Fatalf("workout habit: %+v", workout)
	}
	ordinary := createWater(t, env)
	if ordinary.Kind == nil || *ordinary.Kind != "count" {
		t.Fatalf("ordinary kind: %+v", ordinary)
	}
	path := fmt.Sprintf("/habits/%d/checkin", workout.Id)
	if status, _ := env.Do(http.MethodPost, path, map[string]any{"amount": 1}, nil); status != http.StatusBadRequest {
		t.Fatalf("manual workout checkin: %d", status)
	}
	var log api.WorkoutLog
	env.MustDo(http.MethodPost, "/workouts/logs", map[string]any{"durationMinutes": 30}, &log)
	progress := todayOf(t, env, workout.Id)
	if progress.Done != 1 || !progress.Reached || len(progress.Logs) != 1 || progress.Logs[0].Source != "workout" {
		t.Fatalf("after workout: %+v", progress)
	}
	if todayOf(t, env, ordinary.Id).Done != 0 {
		t.Fatal("ordinary habit changed")
	}
	var linkedID int64
	if err := env.App.Deps.DB.QueryRow(`SELECT workout_log_id FROM habit_logs WHERE id = ?`, progress.Logs[0].Id).Scan(&linkedID); err != nil || linkedID != log.Id {
		t.Fatalf("workout link: %d %v", linkedID, err)
	}
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/habits/logs/%d", progress.Logs[0].Id), nil, nil); status != http.StatusBadRequest {
		t.Fatalf("delete linked checkin: %d", status)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/workouts/logs/%d", log.Id), nil, nil)
	if progress := todayOf(t, env, workout.Id); progress.Done != 0 || progress.Reached || len(progress.Logs) != 0 {
		t.Fatalf("after delete: %+v", progress)
	}
	var updated api.Habit
	env.MustDo(http.MethodPatch, fmt.Sprintf("/habits/%d", ordinary.Id), map[string]any{"kind": "workout", "unit": "杯"}, &updated)
	if updated.Kind == nil || *updated.Kind != "workout" || updated.Unit != "次" {
		t.Fatalf("updated: %+v", updated)
	}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/habits/%d", ordinary.Id), map[string]any{"archived": true}, nil)
	env.MustDo(http.MethodPost, "/workouts/logs", map[string]any{"durationMinutes": 45}, &log)
	var count int
	if err := env.App.Deps.DB.QueryRow(`SELECT count(*) FROM habit_logs WHERE habit_id = ?`, ordinary.Id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("archived workout habit changed: %d %v", count, err)
	}
}

func TestWorkoutAndCheckinRollBackTogether(t *testing.T) {
	env, _ := setup(t)
	var workout api.Habit
	env.MustDo(http.MethodPost, "/habits", map[string]any{"name": "训练", "kind": "workout"}, &workout)
	if _, err := env.App.Deps.DB.Exec(`CREATE TRIGGER reject_workout_checkin BEFORE INSERT ON habit_logs
		WHEN NEW.source = 'workout' BEGIN SELECT RAISE(ABORT, 'reject'); END`); err != nil {
		t.Fatal(err)
	}
	if status, _ := env.Do(http.MethodPost, "/workouts/logs", map[string]any{"durationMinutes": 10}, nil); status != http.StatusInternalServerError {
		t.Fatalf("failed checkin status: %d", status)
	}
	var count int
	if err := env.App.Deps.DB.QueryRow(`SELECT count(*) FROM workout_logs`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("workout transaction did not roll back: %d %v", count, err)
	}
}
