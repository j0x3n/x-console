package watchers_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/api"
)

// B236: the check window.

func runCount(f *fixture, id int64) int {
	var n int
	_ = f.env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM watcher_runs WHERE watcher_id = ?`, id).Scan(&n)
	return n
}

func TestCheckWindowSettings(t *testing.T) {
	f := setup(t, false)
	base := func(extra map[string]any) map[string]any {
		b := map[string]any{"name": "x", "url": "https://www.example.gov.cn/a", "goal": "盯"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	for _, bad := range []map[string]any{
		{"activeFrom": "09:00"},
		{"activeTo": "18:00"},
		{"activeFrom": "9:00", "activeTo": "18:00"},
		{"activeFrom": "09:00", "activeTo": "25:00"},
		{"activeFrom": "09:00", "activeTo": "09:00"},
		{"activeDays": []int{0}},
		{"activeDays": []int{8}},
	} {
		if status, _ := f.env.Do(http.MethodPost, "/watchers", base(bad), nil); status != http.StatusBadRequest {
			t.Fatalf("%v: %d", bad, status)
		}
	}

	w := f.create(t, base(map[string]any{"activeFrom": "09:00", "activeTo": "18:00", "activeDays": []int{5, 1, 2, 3, 4}}))
	if w.ActiveFrom == nil || *w.ActiveFrom != "09:00" || *w.ActiveTo != "18:00" || !slices.Equal(*w.ActiveDays, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("created %+v", w)
	}
	f.until(t, w.Id, api.WatcherStatusError)
	watchers.WaitRuns(f.m)

	// A patch keeps the window it does not mention, and empty values clear it.
	path := fmt.Sprintf("/watchers/%d", w.Id)
	var p api.Watcher
	f.env.MustDo(http.MethodPatch, path, map[string]any{"name": "y"}, &p)
	if *p.ActiveFrom != "09:00" || len(*p.ActiveDays) != 5 {
		t.Fatalf("window lost on rename: %+v", p)
	}
	if status, _ := f.env.Do(http.MethodPatch, path, map[string]any{"activeTo": "09:00"}, nil); status != http.StatusBadRequest {
		t.Fatalf("same start and end: %d", status)
	}
	f.env.MustDo(http.MethodPatch, path, map[string]any{"activeFrom": "", "activeTo": "", "activeDays": []int{}}, &p)
	if *p.ActiveFrom != "" || *p.ActiveTo != "" || len(*p.ActiveDays) != 0 {
		t.Fatalf("window not cleared: %+v", p)
	}
	if got := f.get(t, w.Id); got.ActiveDays == nil || len(*got.ActiveDays) != 0 {
		t.Fatalf("detail %+v", got)
	}
}

func TestCheckWindowSchedule(t *testing.T) {
	f := setup(t, false)
	ctx := context.Background()
	sh, _ := time.LoadLocation("Asia/Shanghai")
	// 2026-09-07 is a Monday.
	at := func(day, hour int) time.Time { return time.Date(2026, 9, day, hour, 0, 0, 0, sh) }
	now := at(7, 3)
	watchers.SetNow(f.m, func() time.Time { return now })

	w := f.create(t, map[string]any{"name": "x", "url": "https://www.example.gov.cn/a", "goal": "盯",
		"activeFrom": "09:00", "activeTo": "18:00", "activeDays": []int{1, 2, 3, 4, 5}})
	f.until(t, w.Id, api.WatcherStatusError)
	watchers.WaitRuns(f.m)

	due := func(recipe string) {
		t.Helper()
		_, err := f.env.App.Deps.DB.Exec(`UPDATE watchers SET recipe = ?, next_check_at = ? WHERE id = ?`, recipe, at(1, 0), w.Id)
		if err != nil {
			t.Fatal(err)
		}
	}
	tick := func(when time.Time) {
		t.Helper()
		now = when
		if err := watchers.Tick(f.m, ctx); err != nil {
			t.Fatal(err)
		}
		watchers.WaitRuns(f.m)
	}

	// A task that already knows its steps waits outside the window.
	due(`[]`)
	before := runCount(f, w.Id)
	tick(at(7, 3))   // Monday night
	tick(at(12, 12)) // Saturday noon
	if n := runCount(f, w.Id); n != before {
		t.Fatalf("checked outside the window: %d -> %d", before, n)
	}
	// It runs as soon as the window opens, because the check is still due.
	tick(at(7, 10))
	if n := runCount(f, w.Id); n != before+1 {
		t.Fatalf("not checked inside the window: %d -> %d", before, n)
	}

	// A task that has not learned its steps yet is being set up and ignores the window.
	due(``)
	before = runCount(f, w.Id)
	tick(at(7, 3))
	if n := runCount(f, w.Id); n != before+1 {
		t.Fatalf("setup blocked by the window: %d -> %d", before, n)
	}

	// Clearing the window checks at any time.
	f.env.Elevate()
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/watchers/%d", w.Id), map[string]any{"activeFrom": "", "activeTo": "", "activeDays": []int{}}, nil)
	due(`[]`)
	before = runCount(f, w.Id)
	tick(at(12, 3))
	if n := runCount(f, w.Id); n != before+1 {
		t.Fatalf("not checked without a window: %d -> %d", before, n)
	}
}
