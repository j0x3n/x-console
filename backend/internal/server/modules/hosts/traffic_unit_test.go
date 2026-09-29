package hosts

import (
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestCycleAt(t *testing.T) {
	utc := time.UTC
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, utc) }
	for _, tc := range []struct {
		name         string
		day          time.Time
		startDay     int
		months       int
		wantS, wantE time.Time
	}{
		{"mid cycle", d(2026, 9, 20), 4, 1, d(2026, 9, 4), d(2026, 10, 3)},
		{"on the start day", d(2026, 9, 4), 4, 1, d(2026, 9, 4), d(2026, 10, 3)},
		{"day before the start day", d(2026, 9, 3), 4, 1, d(2026, 8, 4), d(2026, 9, 3)},
		{"31st in february", d(2026, 2, 28), 31, 1, d(2026, 2, 28), d(2026, 3, 30)},
		{"31st before february", d(2026, 2, 27), 31, 1, d(2026, 1, 31), d(2026, 2, 27)},
		{"31st in a leap february", d(2028, 2, 29), 31, 1, d(2028, 2, 29), d(2028, 3, 30)},
		{"first day of the year", d(2026, 1, 1), 1, 1, d(2026, 1, 1), d(2026, 1, 31)},
		{"three months, last quarter", d(2025, 11, 5), 1, 3, d(2025, 10, 1), d(2025, 12, 31)},
		{"three months, inside", d(2026, 2, 10), 1, 3, d(2026, 1, 1), d(2026, 3, 31)},
		{"three months, day before start", d(2026, 1, 9), 10, 3, d(2025, 10, 10), d(2026, 1, 9)},
		{"twelve months", d(2026, 9, 20), 1, 12, d(2026, 1, 1), d(2026, 12, 31)},
		{"six months", d(2026, 9, 20), 15, 6, d(2026, 7, 15), d(2027, 1, 14)},
	} {
		start, end := cycleAt(tc.day, tc.startDay, tc.months)
		if !start.Equal(tc.wantS) || !end.Equal(tc.wantE) {
			t.Errorf("%s: got %s to %s, want %s to %s", tc.name, start.Format(dayLayout), end.Format(dayLayout),
				tc.wantS.Format(dayLayout), tc.wantE.Format(dayLayout))
		}
		if tc.day.Before(start) || tc.day.After(end) {
			t.Errorf("%s: %s is outside its own cycle", tc.name, tc.day.Format(dayLayout))
		}
	}
}

func TestTrafficTracker(t *testing.T) {
	tr := newTrafficTracker()
	loc := time.FixedZone("cst", 8*3600)
	at := time.Date(2026, 9, 15, 15, 59, 0, 0, time.UTC) // 23:59 in loc
	step := func(offset time.Duration, rx, tx uint64) protocol.MetricsSample {
		return protocol.MetricsSample{At: at.Add(offset), NetRxTotal: rx, NetTxTotal: tx}
	}
	total := func() (rx, tx int64) {
		for _, a := range tr.byDay("h") {
			rx, tx = rx+a.rx, tx+a.tx
		}
		return
	}

	tr.observe("h", step(0, 5000, 2000), loc)
	if rx, tx := total(); rx != 0 || tx != 0 {
		t.Fatalf("the first sample counted: %d %d", rx, tx)
	}
	tr.observe("h", step(30*time.Second, 6000, 2500), loc)
	if rx, tx := total(); rx != 1000 || tx != 500 {
		t.Fatalf("first step: %d %d", rx, tx)
	}
	// A counter that went down (reboot) counts as nothing, and counting goes on from the new value.
	tr.observe("h", step(60*time.Second, 100, 50), loc)
	tr.observe("h", step(90*time.Second, 400, 150), loc)
	if rx, tx := total(); rx != 1300 || tx != 600 {
		t.Fatalf("after reset: %d %d", rx, tx)
	}
	// A step faster than 10 Gbit/s is dropped.
	tr.observe("h", step(120*time.Second, 400+maxByteRate*31, 150), loc)
	if rx, _ := total(); rx != 1300 {
		t.Fatalf("anomaly counted: %d", rx)
	}
	// Midnight in loc splits the days.
	days := tr.byDay("h")
	if len(days) != 2 || days["2026-09-15"].rx != 1000 || days["2026-09-16"].rx != 300 {
		t.Fatalf("days: %+v", days)
	}
}

func TestTrafficEstimateFromSpeed(t *testing.T) {
	tr := newTrafficTracker()
	at := time.Date(2026, 9, 15, 4, 0, 0, 0, time.UTC)
	tr.observe("old", protocol.MetricsSample{At: at, NetRxRate: 100, NetTxRate: 10}, time.UTC)
	tr.observe("old", protocol.MetricsSample{At: at.Add(30 * time.Second), NetRxRate: 100, NetTxRate: 10}, time.UTC)
	// A long break is capped at a minute.
	tr.observe("old", protocol.MetricsSample{At: at.Add(30*time.Second + time.Hour), NetRxRate: 100, NetTxRate: 10}, time.UTC)
	got := tr.byDay("old")["2026-09-15"]
	if got.rx != 3000+6000 || got.tx != 300+600 || !got.est {
		t.Fatalf("estimate: %+v", got)
	}
}

func TestTrafficFlushKeepsWhatFailed(t *testing.T) {
	tr := newTrafficTracker()
	tr.pending["h"] = map[trafficCell]*trafficAmount{{hour: "2026-09-15T04", day: "2026-09-15"}: {rx: 10, tx: 1}}
	batch := tr.take()
	if a := tr.byDay("h")["2026-09-15"]; a.rx != 10 {
		t.Fatalf("in flight traffic vanished from the report: %+v", a)
	}
	tr.done(batch, false)
	tr.pending["h"][trafficCell{hour: "2026-09-15T04", day: "2026-09-15"}].rx += 5
	if a := tr.byDay("h")["2026-09-15"]; a.rx != 15 || a.tx != 1 {
		t.Fatalf("after a failed write: %+v", a)
	}
	tr.done(tr.take(), true)
	if a := tr.byDay("h")["2026-09-15"]; a.rx != 0 {
		t.Fatalf("written traffic still waiting: %+v", a)
	}
}
