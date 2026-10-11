package codingroi

import (
	"context"
	"time"
)

// Hooks for the integration tests in package codingroi_test, which cannot
// live in this package because testutil imports app, which imports codingroi.

func SetNow(m *Module, now func() time.Time) { m.now = now }

func SyncPulls(m *Module, ctx context.Context) error { return m.syncPulls(ctx) }

func RecordOutcome(m *Module, ctx context.Context, taskID int64, outcome string, at time.Time) error {
	return m.recordOutcome(ctx, taskID, outcome, at, "test", "")
}

// AllocateReviewForTest runs allocateReview. A window is {start, end}; a minute
// is {Unix minute, 1 when it is a coding minute}.
func AllocateReviewForTest(wins map[int64][][2]time.Time, mins [][2]int64) map[int64][2]float64 {
	w := map[int64][]window{}
	for id, list := range wins {
		for _, x := range list {
			w[id] = append(w[id], window{Start: x[0], End: x[1]})
		}
	}
	var m []screenMinute
	for _, x := range mins {
		m = append(m, screenMinute{Minute: x[0], Coding: x[1] == 1})
	}
	out := map[int64][2]float64{}
	for id, r := range allocateReview(w, m) {
		has := 0.0
		if r.HasData {
			has = 1
		}
		out[id] = [2]float64{r.Minutes, has}
	}
	return out
}

// CapForTest returns the end of a window after the cap and whether it was cut.
func CapForTest(start, end time.Time) (time.Time, bool) {
	w, cut := capWindow(window{Start: start, End: end})
	return w.End, cut
}
