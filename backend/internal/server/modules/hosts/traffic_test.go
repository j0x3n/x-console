package hosts_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

type notificationList struct {
	Items []struct {
		Kind  string `json:"kind"`
		Title string `json:"title"`
		Body  string `json:"body"`
	} `json:"items"`
}

func TestHostTraffic(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	// 2026-09-15 12:00 in Shanghai.
	now := time.Date(2026, 9, 15, 4, 0, 0, 0, time.UTC)
	m.SetNow(now)
	id, _, _ := startAgent(t, env, "tokyo-1", "server", serverCaps, nil)

	feed := func(ago time.Duration, rx, tx uint64) {
		m.RecordSample(id, protocol.MetricsSample{At: now.Add(-ago), NetRxTotal: rx, NetTxTotal: tx})
	}
	// The default plan: from the 1st, one month, no limit.
	var got api.HostTraffic
	env.MustDo(http.MethodGet, "/hosts/"+id+"/traffic", nil, &got)
	if got.CycleStart.Format("2006-01-02") != "2026-09-01" || got.CycleEnd.Format("2006-01-02") != "2026-09-30" ||
		got.Plan.LimitBytes != 0 || len(got.Days) != 15 || got.UsedBytes != 0 {
		t.Fatalf("default: %+v", got)
	}

	feed(3*time.Minute, 10_000, 4_000)
	feed(2*time.Minute, 1_010_000, 504_000)
	feed(time.Minute, 2_010_000, 1_004_000)
	// Counted from memory before it is written.
	env.MustDo(http.MethodGet, "/hosts/"+id+"/traffic", nil, &got)
	if got.Rx != 2_000_000 || got.Tx != 1_000_000 || got.UsedBytes != 3_000_000 || got.Days[14].Rx != 2_000_000 {
		t.Fatalf("from memory: %+v", got)
	}
	if err := m.FlushTraffic(ctx); err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodGet, "/hosts/"+id+"/traffic", nil, &got)
	if got.Rx != 2_000_000 || got.Tx != 1_000_000 {
		t.Fatalf("after flush: %+v", got)
	}
	if got.ProjectedBytes == nil || *got.ProjectedBytes != 3_000_000*30/15 {
		t.Fatalf("projected: %v", got.ProjectedBytes)
	}
	if rows, _ := m.Queries().ListTrafficDays(ctx, db.ListTrafficDaysParams{HostID: id, FromDay: "2026-01-01", ToDay: "2026-12-31"}); len(rows) != 1 || rows[0].Rx != 2_000_000 {
		t.Fatalf("day rows: %+v", rows)
	}

	// Plan: from the 10th, out only, 8 MB limit, alert at 50%.
	expectStatus(t, env, http.MethodPut, "/hosts/"+id+"/traffic/plan", map[string]any{"startDay": 0, "periodMonths": 1, "limitBytes": 1, "countMode": "both", "alertPercent": 80}, http.StatusBadRequest, "validation_failed")
	expectStatus(t, env, http.MethodPut, "/hosts/"+id+"/traffic/plan", map[string]any{"startDay": 1, "periodMonths": 2, "limitBytes": 1, "countMode": "both", "alertPercent": 80}, http.StatusBadRequest, "validation_failed")
	expectStatus(t, env, http.MethodPut, "/hosts/nope/traffic/plan", map[string]any{"startDay": 1, "periodMonths": 1, "limitBytes": 1, "countMode": "both", "alertPercent": 80}, http.StatusNotFound, "not_found")
	plan := map[string]any{"startDay": 10, "periodMonths": 1, "limitBytes": 8_000_000, "countMode": "out", "alertPercent": 50}
	env.MustDo(http.MethodPut, "/hosts/"+id+"/traffic/plan", plan, &got)
	if got.Plan.CountMode != api.Out || got.UsedBytes != 1_000_000 || got.CycleStart.Format("2006-01-02") != "2026-09-10" ||
		got.CycleEnd.Format("2006-01-02") != "2026-10-09" {
		t.Fatalf("after plan: %+v", got)
	}
	// The previous cycle has no data but the right dates.
	got = api.HostTraffic{}
	env.MustDo(http.MethodGet, "/hosts/"+id+"/traffic?cycle=previous", nil, &got)
	if got.CycleStart.Format("2006-01-02") != "2026-08-10" || got.CycleEnd.Format("2006-01-02") != "2026-09-09" ||
		got.UsedBytes != 0 || got.ProjectedBytes != nil || len(got.Days) != 31 {
		t.Fatalf("previous: %+v", got)
	}

	// The list shows a usage bar for hosts with a limit.
	var list []api.HostListItem
	env.MustDo(http.MethodGet, "/hosts", nil, &list)
	if len(list) != 1 || list[0].Traffic == nil || list[0].Traffic.UsedBytes != 1_000_000 || list[0].Traffic.LimitBytes != 8_000_000 {
		t.Fatalf("list traffic: %+v", list)
	}

	notices := func() []string {
		var n notificationList
		env.MustDo(http.MethodGet, "/notifications", nil, &n)
		var out []string
		for _, it := range n.Items {
			if it.Kind == "host.traffic_alert" {
				out = append(out, it.Title)
			}
		}
		return out
	}
	if err := m.CheckTraffic(ctx); err != nil {
		t.Fatal(err)
	}
	if len(notices()) != 0 {
		t.Fatalf("alert below the line: %v", notices())
	}
	// 5 MB more going out: 6 MB of 8 MB is over the 50% line.
	feed(30*time.Second, 2_010_000, 6_004_000)
	feed(0, 2_010_000, 6_004_000)
	if err := m.FlushTraffic(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // the second run must not send again
		if err := m.CheckTraffic(ctx); err != nil {
			t.Fatal(err)
		}
	}
	got1 := notices()
	if len(got1) != 1 || !strings.Contains(got1[0], "50%") {
		t.Fatalf("first alert: %v", got1)
	}
	feed(-time.Second, 2_010_000, 10_004_000)
	if err := m.FlushTraffic(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := m.CheckTraffic(ctx); err != nil {
			t.Fatal(err)
		}
	}
	got2 := notices()
	if len(got2) != 2 || !strings.Contains(got2[0], "用完") {
		t.Fatalf("second alert: %v", got2)
	}

	// Changing the plan without a limit leaves the alerts alone; removing the host removes its numbers.
	plan["limitBytes"] = 0
	env.MustDo(http.MethodPut, "/hosts/"+id+"/traffic/plan", plan, &got)
	list = nil
	env.MustDo(http.MethodGet, "/hosts", nil, &list)
	if list[0].Traffic != nil {
		t.Fatalf("traffic bar without a limit: %+v", list[0].Traffic)
	}
}
