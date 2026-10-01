package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func f32(v float32) *float32 { return &v }

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// B42：命中缓存按缓存价算；没有缓存价时按原价算并标成估算。
func TestUsageCost(t *testing.T) {
	cost, est := ai.UsageCostForTest(1_000_000, 800_000, 100_000, 1_000_000, f32(2), f32(8), f32(0.5), f32(2.5))
	// 未命中 100k×2 + 命中 800k×0.5 + 写入 100k×2.5 + 输出 1M×8
	if cost == nil || !near(*cost, 0.2+0.4+0.25+8) || est {
		t.Fatalf("with cache prices: %v %v", cost, est)
	}
	cost, est = ai.UsageCostForTest(1_000_000, 800_000, 0, 0, f32(2), f32(8), nil, nil)
	if cost == nil || !near(*cost, 2) || !est {
		t.Fatalf("without cache price: %v %v", cost, est)
	}
	if cost, _ := ai.UsageCostForTest(10, 0, 0, 10, nil, f32(8), nil, nil); cost != nil {
		t.Fatalf("no input price: %v", *cost)
	}
}

func insertUsage(t *testing.T, env *testutil.Env, model, source, status string, input, cached, output int64, cost any, at time.Time) {
	t.Helper()
	_, err := env.App.Deps.DB.Exec(`INSERT INTO ai_usage(provider_id,provider_name,model,purpose,input_tokens,cached_input_tokens,output_tokens,
 duration_ms,cost,source,status,created_at) VALUES(1,'Test',?,'agent',?,?,?,100,?,?,?,?)`, model, input, cached, output, cost, source, status, at.UTC())
	if err != nil {
		t.Fatal(err)
	}
}

func TestUsageSummaryRecordsAndCSV(t *testing.T) {
	env := testutil.New(t)
	loc := env.App.Deps.Config.Location
	day := func(d int) time.Time { return time.Date(2026, 3, d, 12, 0, 0, 0, loc) }
	insertUsage(t, env, "m1", "notes", "ok", 100, 80, 10, 0.5, day(1))
	insertUsage(t, env, "m1", "assistant", "ok", 100, 20, 10, 0.25, day(3))
	insertUsage(t, env, "m2", "assistant", "error", 0, 0, 0, nil, day(3))
	insertUsage(t, env, "m2", "assistant", "ok", 50, 0, 5, nil, day(4)) // outside the range
	insertUsage(t, env, "m1", "notes", "ok", 1000, 0, 10, 1.0, day(1).AddDate(0, 0, -3))

	var byDay api.AiUsageSummary
	env.MustDo("GET", "/ai/usage/summary?from=2026-03-01&to=2026-03-03&groupBy=day", nil, &byDay)
	if len(byDay.Groups) != 3 || byDay.Groups[1].Key != "2026-03-02" || byDay.Groups[1].Totals.Calls != 0 {
		t.Fatalf("days: %+v", byDay.Groups)
	}
	total := byDay.Total
	if total.Calls != 3 || total.Errors != 1 || total.InputTokens != 200 || total.CachedInputTokens != 100 ||
		total.CacheHitRate == nil || *total.CacheHitRate != 0.5 || total.Cost == nil || *total.Cost != 0.75 {
		t.Fatalf("total: %+v", total)
	}
	if byDay.Previous == nil || byDay.Previous.Calls != 1 || byDay.Previous.InputTokens != 1000 {
		t.Fatalf("previous: %+v", byDay.Previous)
	}

	var bySource api.AiUsageSummary
	env.MustDo("GET", "/ai/usage/summary?from=2026-03-01&to=2026-03-03&groupBy=source", nil, &bySource)
	if len(bySource.Groups) != 2 || bySource.Groups[0].Key != "notes" || bySource.Groups[1].Totals.Calls != 2 {
		t.Fatalf("by source: %+v", bySource.Groups)
	}

	var page struct {
		Items      []api.AiUsageRecord
		NextCursor *string
	}
	env.MustDo("GET", "/ai/usage/records?from=2026-03-01&to=2026-03-03&limit=2", nil, &page)
	if len(page.Items) != 2 || page.NextCursor == nil || page.Items[0].Id < page.Items[1].Id {
		t.Fatalf("page 1: %+v", page)
	}
	var page2 struct {
		Items      []api.AiUsageRecord
		NextCursor *string
	}
	env.MustDo("GET", "/ai/usage/records?from=2026-03-01&to=2026-03-03&limit=2&cursor="+*page.NextCursor, nil, &page2)
	if len(page2.Items) != 1 || page2.NextCursor != nil {
		t.Fatalf("page 2: %+v", page2)
	}
	var errs struct{ Items []api.AiUsageRecord }
	env.MustDo("GET", "/ai/usage/records?from=2026-03-01&to=2026-03-03&status=error", nil, &errs)
	if len(errs.Items) != 1 || errs.Items[0].Model != "m2" {
		t.Fatalf("errors: %+v", errs)
	}

	resp, err := env.Client.Get(env.URL("/ai/usage/records.csv?from=2026-03-01&to=2026-03-03&source=notes"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	lines := strings.Split(strings.TrimSpace(strings.TrimPrefix(string(raw), "\ufeff")), "\n")
	if resp.StatusCode != 200 || len(lines) != 2 || !strings.HasPrefix(lines[0], "时间,") || !strings.Contains(lines[1], ",m1,") {
		t.Fatalf("csv %d: %q", resp.StatusCode, raw)
	}

	if status, _ := env.Do("GET", "/ai/usage/summary?from=2026-03-03&to=2026-03-01&groupBy=day", nil, nil); status != 400 {
		t.Fatalf("reversed range: %d", status)
	}
	if status, _ := env.Do("GET", "/ai/usage/summary?from=2025-01-01&to=2026-03-01&groupBy=day", nil, nil); status != 400 {
		t.Fatalf("too long: %d", status)
	}
}

// B42：跨年的时间范围、按天分组时服务器时区的边界。
func TestUsageSummaryAcrossYear(t *testing.T) {
	env := testutil.New(t)
	loc := env.App.Deps.Config.Location
	insertUsage(t, env, "m", "", "ok", 10, 0, 1, nil, time.Date(2025, 12, 31, 23, 30, 0, 0, loc))
	insertUsage(t, env, "m", "", "ok", 20, 0, 1, nil, time.Date(2026, 1, 1, 0, 30, 0, 0, loc))
	var s api.AiUsageSummary
	env.MustDo("GET", "/ai/usage/summary?from=2025-12-31&to=2026-01-01&groupBy=day", nil, &s)
	if len(s.Groups) != 2 || s.Groups[0].Totals.InputTokens != 10 || s.Groups[1].Totals.InputTokens != 20 {
		t.Fatalf("groups: %+v", s.Groups)
	}
}

// B42：Agent 任务等外部用量通过 contracts.AIUsageRecorder 记进来，带上来源。
func TestExternalUsageRecorder(t *testing.T) {
	env := testutil.New(t)
	rec, ok := module.Lookup[contracts.AIUsageRecorder](env.App.Deps.Registry, contracts.AIUsageKey)
	if !ok {
		t.Fatal("recorder not provided")
	}
	cost := 0.12
	rec.RecordExternalUsage(context.Background(), "Claude Code", "claude-x", "coding", "7", 1000, 900, 50, 30, time.Second, &cost)
	today := time.Now().In(env.App.Deps.Config.Location).Format(time.DateOnly)
	var page struct{ Items []api.AiUsageRecord }
	env.MustDo("GET", fmt.Sprintf("/ai/usage/records?from=%s&to=%s&source=coding", today, today), nil, &page)
	if len(page.Items) != 1 {
		t.Fatalf("records: %+v", page)
	}
	r := page.Items[0]
	if r.Ref != "7" || r.CachedInputTokens != 900 || r.Cost == nil || math.Abs(float64(*r.Cost)-0.12) > 1e-6 {
		b, _ := json.Marshal(r)
		t.Fatalf("record: %s", b)
	}
	_ = http.StatusOK
}
