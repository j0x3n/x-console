package habits_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
)

func TestPersonalActions(t *testing.T) {
	env, _ := setup(t)
	run := func(name, input string) any {
		t.Helper()
		out, err := env.App.Deps.Actions.Run(context.Background(), name, json.RawMessage(input))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	catalog := run("habits.library", `{}`).(map[string]any)
	if len(catalog["exercises"].([]map[string]string)) != 66 {
		t.Fatal("missing exercise actions")
	}
	p := run("habits.personal_activate", `{"ids":["words"]}`).(api.PersonalProfile)
	run("habits.personal_check", fmt.Sprintf(`{"date":%q,"id":"words","done":true}`, p.Start))
	d := run("habits.personal_day", fmt.Sprintf(`{"date":%q}`, p.Start)).(api.PersonalDay)
	if !d.Checks["words"] {
		t.Fatal("action checkin missing")
	}
	run("habits.personal_profile", `{}`)
	run("habits.library", fmt.Sprintf(`{"exerciseId":%q}`, func() string {
		var l api.PersonalLibrary
		env.MustDo("GET", "/habits/library", nil, &l)
		return l.Exercises[0].Id
	}()))
	if _, err := env.App.Deps.Actions.Run(context.Background(), "habits.library", json.RawMessage(`{"exerciseId":"unknown"}`)); err == nil {
		t.Fatal("unknown exercise accepted")
	}
}

func TestPersonalLibraryAndNativeCheckins(t *testing.T) {
	env, _ := setup(t)
	var library api.PersonalLibrary
	env.MustDo(http.MethodGet, "/habits/library", nil, &library)
	if len(library.Exercises) != 66 || len(library.Sessions) != 9 || len(library.RunLevels) != 9 || len(library.Habits) != 16 || len(library.Articles) != 26 || library.Original.Attachment == "" {
		t.Fatalf("incomplete library: exercises=%d articles=%d", len(library.Exercises), len(library.Articles))
	}
	var p api.PersonalProfile
	activate := map[string]any{"ids": []string{"words", "protein"}}
	env.MustDo(http.MethodPost, "/habits/library/activate", activate, &p)
	env.MustDo(http.MethodPost, "/habits/library/activate", activate, &p)
	var count int
	env.App.Deps.DB.QueryRow("SELECT count(*) FROM habits").Scan(&count)
	if count != 2 {
		t.Fatalf("duplicate habits: %d", count)
	}
	path := "/habits/personal/days/" + p.Start
	var d api.PersonalDay
	body := map[string]any{"id": "words", "done": true}
	env.MustDo(http.MethodPost, path+"/check", body, &d)
	env.MustDo(http.MethodPost, path+"/check", body, &d)
	if !d.Checks["words"] || todayOf(t, env, p.HabitIds["words"]).Done != 1 {
		t.Fatalf("native checkin not idempotent: %+v", d)
	}
	env.MustDo(http.MethodPost, fmt.Sprintf("/habits/%d/checkin", p.HabitIds["words"]), map[string]any{}, nil)
	d = api.PersonalDay{}
	env.MustDo(http.MethodPost, path+"/check", map[string]any{"id": "words", "done": false}, &d)
	if d.Checks["words"] || todayOf(t, env, p.HabitIds["words"]).Done != 1 {
		t.Fatal("undo deleted a manual checkin")
	}
	env.MustDo(http.MethodPost, path+"/check", body, &d)
	var logID int64
	env.App.Deps.DB.QueryRow("SELECT id FROM habit_logs WHERE note LIKE '个人计划:%'").Scan(&logID)
	env.MustDo(http.MethodDelete, fmt.Sprintf("/habits/logs/%d", logID), nil, nil)
	d = api.PersonalDay{}
	env.MustDo(http.MethodGet, path, nil, &d)
	if d.Checks["words"] {
		t.Fatal("deleted native log still shown as checked")
	}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/habits/%d", p.HabitIds["words"]), map[string]any{"archived": true}, nil)
	if status, _ := env.Do(http.MethodPost, path+"/check", body, nil); status != http.StatusConflict {
		t.Fatalf("archived checkin: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/habits/library/activate", map[string]any{"ids": []string{"unknown"}}, nil); status != 400 {
		t.Fatalf("unknown template: %d", status)
	}
}

func TestPersonalProfileDaysAndBackup(t *testing.T) {
	env, _ := setup(t)
	profile := map[string]any{"start": "2026-09-01", "wake": "11:00", "sleep": "03:00", "phase": 2, "baseline": 82, "stepGoal": 6000, "runLevel": 2}
	backup := map[string]any{"version": 1, "exportedAt": "2026-09-13T01:00:00Z", "profile": profile, "checks": map[string]bool{"2026-09-12:words": true, "2026-09-12:posture-hang": true, "2026-09-12:exercise-A1-0": true}, "sets": map[string]bool{"2026-09-12:A1:0:0": true}, "logs": map[string]any{"2026-09-12": map[string]any{"weight": "81.5", "sleep": "7.5", "english": "confirm the deadline"}}}
	var p api.PersonalProfile
	env.MustDo(http.MethodPost, "/habits/personal/backup", backup, &p)
	env.MustDo(http.MethodPost, "/habits/personal/backup", backup, &p)
	if p.Phase != 2 || len(p.HabitIds) != 2 {
		t.Fatalf("profile not imported: %+v", p)
	}
	var d api.PersonalDay
	env.MustDo(http.MethodGet, "/habits/personal/days/2026-09-12", nil, &d)
	if d.Weight != "81.5" || d.English != "confirm the deadline" || !d.Checks["words"] || !d.Sets["A1:0:0"] || !d.Sets["A1:0:2"] {
		t.Fatalf("legacy data lost: %+v", d)
	}
	var n int
	env.App.Deps.DB.QueryRow("SELECT count(*) FROM habit_logs").Scan(&n)
	if n != 2 {
		t.Fatalf("duplicate imported logs: %d", n)
	}
	env.MustDo(http.MethodPatch, "/habits/personal/days/2026-09-12", map[string]any{"food": "鱼肉和蔬菜", "sets": map[string]bool{"B1:1:0": true}}, &d)
	if d.Weight != "81.5" || !d.Sets["A1:0:0"] || !d.Sets["B1:1:0"] {
		t.Fatal("partial edit overwrote other fields")
	}
	env.MustDo(http.MethodPatch, "/habits/personal/days/2026-09-12", map[string]any{"weight": ""}, &d)
	if d.Weight != "" || d.Food != "鱼肉和蔬菜" {
		t.Fatal("clear metric did not work")
	}
	var exported api.PersonalBackup
	env.MustDo(http.MethodGet, "/habits/personal/backup", nil, &exported)
	env.MustDo(http.MethodPost, "/habits/personal/backup", exported, &p)
	env.MustDo(http.MethodGet, "/habits/personal/days/2026-09-12", nil, &d)
	if d.Food != "鱼肉和蔬菜" || !d.Checks["posture-hang"] {
		t.Fatal("round trip lost data")
	}
	backup["logs"] = map[string]any{"2026-09-12": map[string]any{"weight": "90"}, "2026-02-30": map[string]any{"weight": "82"}}
	if status, _ := env.Do(http.MethodPost, "/habits/personal/backup", backup, nil); status != 400 {
		t.Fatalf("invalid date: %d", status)
	}
	env.MustDo(http.MethodGet, "/habits/personal/days/2026-09-12", nil, &d)
	if d.Weight != "" {
		t.Fatal("invalid import partly changed records")
	}
	for _, fields := range []map[string]any{{"weight": "NaN"}, {"steps": "1.5"}, {"sets": map[string]bool{"A1:99:0": true}}, {"sleep": "25"}} {
		if status, _ := env.Do(http.MethodPatch, "/habits/personal/days/2026-09-12", fields, nil); status != 400 {
			t.Fatalf("bad fields accepted: %+v status=%d", fields, status)
		}
	}
	if status, _ := env.Do(http.MethodGet, "/habits/personal/days/invalid", nil, nil); status != 400 {
		t.Fatalf("invalid date read: %d", status)
	}
	profile["wake"] = "25:00"
	if status, _ := env.Do(http.MethodPut, "/habits/personal/profile", profile, nil); status != 400 {
		t.Fatalf("bad wake time: %d", status)
	}
}

func TestPersonalWorkoutIsOneNativeRecord(t *testing.T) {
	env, _ := setup(t)
	var h api.Habit
	env.MustDo(http.MethodPost, "/habits", map[string]any{"name": "训练", "kind": "workout"}, &h)
	var p api.PersonalProfile
	env.MustDo(http.MethodGet, "/habits/personal/profile", nil, &p)
	path := "/habits/personal/days/" + p.Start + "/workout"
	body := map[string]any{"durationMinutes": 35, "note": "训练笔记", "items": []map[string]any{{"name": "高脚杯深蹲", "exerciseId": "squat", "sets": 2, "prescription": "8–12 次"}}}
	var d api.PersonalDay
	env.MustDo(http.MethodPost, path, body, &d)
	first := *d.WorkoutLogId
	body["durationMinutes"] = 45
	env.MustDo(http.MethodPost, path, body, &d)
	if *d.WorkoutLogId != first || todayOf(t, env, h.Id).Done != 1 {
		t.Fatal("saved workout twice caused duplicate checkins")
	}
	var logs []api.WorkoutLog
	env.MustDo(http.MethodGet, "/workouts/logs", nil, &logs)
	if len(logs) != 1 || logs[0].DurationMinutes != 45 || *logs[0].Items[0].Prescription != "8–12 次" {
		t.Fatalf("workout snapshot lost: %+v", logs)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/workouts/logs/%d", first), nil, nil)
	d = api.PersonalDay{}
	env.MustDo(http.MethodGet, "/habits/personal/days/"+p.Start, nil, &d)
	if d.WorkoutLogId != nil {
		t.Fatal("deleted workout still linked")
	}
	env.MustDo(http.MethodPost, path, body, &d)
	if todayOf(t, env, h.Id).Done != 1 {
		t.Fatal("recreated workout not checked in")
	}
}

func TestPersonalAtomicActivationAndConcurrentChecks(t *testing.T) {
	env, _ := setup(t)
	_, err := env.App.Deps.DB.Exec("CREATE TRIGGER reject_personal_profile BEFORE INSERT ON settings WHEN NEW.key = 'habits.personal_profile' BEGIN SELECT RAISE(ABORT, 'reject'); END")
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := env.Do(http.MethodPost, "/habits/library/activate", map[string]any{"ids": []string{"review"}}, nil); status != 500 {
		t.Fatalf("trigger status: %d", status)
	}
	var n int
	env.App.Deps.DB.QueryRow("SELECT count(*) FROM habits").Scan(&n)
	if n != 0 {
		t.Fatal("failed activation left a habit")
	}
	env.App.Deps.DB.Exec("DROP TRIGGER reject_personal_profile")
	var p api.PersonalProfile
	env.MustDo(http.MethodPost, "/habits/library/activate", map[string]any{"ids": []string{"review"}}, &p)
	var wg sync.WaitGroup
	statuses := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, _ := env.Do(http.MethodPost, "/habits/personal/days/"+p.Start+"/check", map[string]any{"id": "review", "done": true}, nil)
			statuses <- status
		}()
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != 200 {
			t.Errorf("concurrent check: %d", status)
		}
	}
	env.App.Deps.DB.QueryRow("SELECT count(*) FROM habit_logs").Scan(&n)
	if n != 1 {
		t.Fatalf("concurrent checks duplicated: %d", n)
	}
	// SQLite may reuse an ID after deletion. The old template must not attach to a new habit.
	env.MustDo(http.MethodDelete, fmt.Sprintf("/habits/%d", p.HabitIds["review"]), nil, nil)
	env.MustDo(http.MethodPost, "/habits", map[string]any{"name": "别的习惯"}, nil)
	p = api.PersonalProfile{}
	env.MustDo(http.MethodGet, "/habits/personal/profile", nil, &p)
	if _, ok := p.HabitIds["review"]; ok {
		t.Fatal("deleted template rebound to reused ID")
	}
}
