package repo

import (
	"testing"
	"time"
)

func TestRetentionAcrossISOYearAndSparseDays(t *testing.T) {
	loc := time.UTC
	now := time.Date(2027, 1, 4, 12, 0, 0, 0, loc)
	at := func(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, loc) }
	all := []Snapshot{{ID: "this-week", CreatedAt: at(2027, 1, 4, 9)}, {ID: "previous-week", CreatedAt: at(2026, 12, 31, 9)}, {ID: "old-week", CreatedAt: at(2026, 12, 20, 9)}, {ID: "older", CreatedAt: at(2026, 11, 20, 9)}}
	keep := Keep(all, Retention{Last: 1, Weekly: 2}, now, loc)
	if !keep["this-week"] || !keep["previous-week"] || keep["old-week"] || len(keep) != 2 {
		t.Fatal(keep)
	}
	keep = Keep(all, Retention{Last: 1, Daily: 2}, now, loc)
	if len(keep) != 1 {
		t.Fatal("sparse days filled from old snapshots", keep)
	}
}

func TestRetentionMonthEndLeapAndFuture(t *testing.T) {
	now := time.Date(2028, 3, 31, 12, 0, 0, 0, time.UTC)
	all := []Snapshot{{ID: "march", CreatedAt: now.Add(-time.Hour)}, {ID: "february", CreatedAt: time.Date(2028, 2, 29, 12, 0, 0, 0, time.UTC)}, {ID: "january", CreatedAt: time.Date(2028, 1, 31, 12, 0, 0, 0, time.UTC)}, {ID: "future", CreatedAt: now.Add(time.Hour)}}
	keep := Keep(all, Retention{Last: 1, Monthly: 2}, now, time.UTC)
	if len(keep) != 3 || !keep["march"] || !keep["february"] || !keep["future"] || keep["january"] {
		t.Fatal(keep)
	}
}
