package monitoring_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
)

func day(s string) openapi_types.Date {
	t, _ := time.Parse("2006-01-02", s)
	return openapi_types.Date{Time: t}
}

func TestRenewalsContract(t *testing.T) {
	env, _ := setup(t)
	for _, item := range []struct {
		name string
		date string
	}{
		{"overdue", "2026-10-01"},
		{"within", "2026-10-07"},
		{"boundary", "2026-10-08"},
		{"archived", "2026-10-06"},
	} {
		var created api.Subscription
		env.MustDo(http.MethodPost, "/subscriptions", api.SubscriptionInput{
			Name: item.name, Amount: 12.5, Currency: ptr("USD"), Cycle: "monthly", NextRenewal: day(item.date),
		}, &created)
		if item.name == "archived" {
			env.MustDo(http.MethodPatch, fmt.Sprintf("/subscriptions/%d", created.Id), api.SubscriptionPatch{Archived: ptr(true)}, &created)
		}
	}
	svc, ok := module.Lookup[contracts.Renewals](env.App.Deps.Registry, contracts.RenewalsKey)
	if !ok {
		t.Fatal("renewals provider missing")
	}
	until := time.Date(2026, 10, 8, 0, 0, 0, 0, env.App.Deps.Config.Location)
	refs, err := svc.Upcoming(context.Background(), until)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 || refs[0].Name != "overdue" || refs[1].Name != "within" ||
		refs[1].Amount != 12.5 || refs[1].Currency != "USD" ||
		refs[1].Date.Format(time.DateOnly) != "2026-10-07" || refs[1].Date.Location() != time.UTC {
		t.Fatalf("upcoming renewals: %+v", refs)
	}
}

func TestSubscriptionReminders(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	sh := env.App.Deps.Config.Location // Asia/Shanghai in tests
	at := func(date string, hour int) time.Time {
		d, _ := time.ParseInLocation("2006-01-02", date, sh)
		return d.Add(time.Duration(hour) * time.Hour)
	}

	expectStatus(t, env, http.MethodPost, "/subscriptions", api.SubscriptionInput{Name: "x", Amount: 1, Cycle: "custom_days", NextRenewal: day("2026-10-11")}, 400, "validation_failed")
	expectStatus(t, env, http.MethodPost, "/subscriptions", api.SubscriptionInput{Name: "x", Amount: -1, Cycle: "monthly", NextRenewal: day("2026-10-11")}, 400, "validation_failed")
	expectStatus(t, env, http.MethodGet, "/subscriptions/77", nil, 404, "not_found")

	var vps api.Subscription
	env.MustDo(http.MethodPost, "/subscriptions", api.SubscriptionInput{Name: "VPS", Amount: 10, Cycle: "monthly", NextRenewal: day("2026-10-11"),
		Category: ptr(api.SubscriptionCategoryServer)}, &vps)
	if vps.Currency != "CNY" || len(vps.RemindDaysBefore) != 3 || vps.RemindDaysBefore[0] != 7 || vps.RemindDaysBefore[1] != 3 || vps.AutoRenew {
		t.Fatalf("created: %+v", vps)
	}

	// Reminders at 7 and 1 days before, each once, and not before 9:00.
	steps := []time.Time{
		at("2026-10-01", 10), // 10 days
		at("2026-10-04", 10), // 7 days: reminder
		at("2026-10-04", 15),
		at("2026-10-05", 10),
		at("2026-10-10", 8), // 1 day, but too early
		at("2026-10-10", 9), // reminder
		at("2026-10-10", 20),
		at("2026-10-11", 10), // renewal day, not auto-renewing
		at("2026-10-12", 10), // overdue
	}
	for _, now := range steps {
		if err := m.ScanSubscriptions(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	sent := notifications(t, env, "subscription.due")
	if len(sent) != 2 || sent[0] != "VPS 还有 7 天续费" || sent[1] != "VPS 还有 1 天续费" {
		t.Fatalf("reminders: %v", sent)
	}
	env.MustDo(http.MethodGet, fmt.Sprintf("/subscriptions/%d", vps.Id), nil, &vps)
	if vps.NextRenewal.String() != "2026-10-11" {
		t.Fatalf("manual renewal moved: %+v", vps)
	}

	// Paying and moving the date starts a new round of reminders.
	env.MustDo(http.MethodPatch, fmt.Sprintf("/subscriptions/%d", vps.Id), api.SubscriptionPatch{NextRenewal: ptr(day("2026-11-11"))}, &vps)
	_ = m.ScanSubscriptions(ctx, at("2026-11-04", 12))
	if n := len(notifications(t, env, "subscription.due")); n != 3 {
		t.Fatalf("after new date: %d", n)
	}

	// Auto-renewing: moved past the renewal date by whole cycles.
	var domain api.Subscription
	env.MustDo(http.MethodPost, "/subscriptions", api.SubscriptionInput{Name: "example.com", Amount: 120, Cycle: "yearly",
		NextRenewal: day("2026-10-02"), AutoRenew: ptr(true), RemindDaysBefore: &[]int{}, Category: ptr(api.SubscriptionCategoryDomain)}, &domain)
	var usd api.Subscription
	env.MustDo(http.MethodPost, "/subscriptions", api.SubscriptionInput{Name: "SaaS", Amount: 5, Currency: ptr("usd"), Cycle: "custom_days",
		CycleDays: ptr(30), NextRenewal: day("2026-09-20"), AutoRenew: ptr(true), RemindDaysBefore: &[]int{3}}, &usd)
	if err := m.ScanSubscriptions(ctx, at("2026-10-03", 10)); err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodGet, fmt.Sprintf("/subscriptions/%d", domain.Id), nil, &domain)
	env.MustDo(http.MethodGet, fmt.Sprintf("/subscriptions/%d", usd.Id), nil, &usd)
	if domain.NextRenewal.String() != "2027-10-02" || usd.NextRenewal.String() != "2026-10-20" || usd.Currency != "USD" {
		t.Fatalf("renewed: %s %s", domain.NextRenewal, usd.NextRenewal)
	}
	var events []api.SubscriptionEvent
	env.MustDo(http.MethodGet, fmt.Sprintf("/subscriptions/%d/events", domain.Id), nil, &events)
	if len(events) != 2 || events[0].Kind != api.Renewed || !strings.Contains(events[0].Detail, "2027-10-02") || events[1].Kind != api.Created {
		t.Fatalf("events: %+v", events)
	}
	env.MustDo(http.MethodGet, fmt.Sprintf("/subscriptions/%d/events", vps.Id), nil, &events)
	kinds := []api.SubscriptionEventKind{}
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	if fmt.Sprint(kinds) != "[reminded updated reminded reminded created]" {
		t.Fatalf("vps events: %v", kinds)
	}

	// Spend: 10 + 120/12 CNY, 5 USD per 30 days.
	var sum api.SubscriptionSummary
	env.MustDo(http.MethodGet, "/subscriptions/summary", nil, &sum)
	if len(sum.Totals) != 2 || sum.Totals[0].Currency != "CNY" || sum.Totals[0].Monthly != 20 || sum.Totals[0].Yearly != 240 ||
		sum.Totals[0].Count != 2 || sum.Totals[1].Currency != "USD" || sum.Totals[1].Monthly != 5.07 {
		t.Fatalf("summary: %+v", sum)
	}
	if len(sum.ByCategory) != 3 {
		t.Fatalf("by category: %+v", sum.ByCategory)
	}
	if sum.Converted != nil || sum.Unconverted != nil || sum.RatesAt != nil {
		t.Fatalf("rates before any fetch: %+v", sum)
	}

	// Archived subscriptions leave the list and the totals.
	env.MustDo(http.MethodPatch, fmt.Sprintf("/subscriptions/%d", usd.Id), api.SubscriptionPatch{Archived: ptr(true)}, &usd)
	if usd.ArchivedAt == nil {
		t.Fatal("not archived")
	}
	var list []api.Subscription
	env.MustDo(http.MethodGet, "/subscriptions", nil, &list)
	if len(list) != 2 || list[0].Name != "VPS" {
		t.Fatalf("list: %+v", list)
	}
	env.MustDo(http.MethodGet, "/subscriptions?archived=true", nil, &list)
	if len(list) != 1 || list[0].Name != "SaaS" {
		t.Fatalf("archived list: %+v", list)
	}
	env.MustDo(http.MethodGet, "/subscriptions/summary", nil, &sum)
	if len(sum.Totals) != 1 {
		t.Fatalf("summary after archive: %+v", sum)
	}

	var out struct {
		Items   []api.Subscription      `json:"items"`
		Summary api.SubscriptionSummary `json:"summary"`
	}
	runAction(t, env, "subscriptions.list", map[string]any{}, &out)
	if len(out.Items) != 2 || len(out.Summary.Totals) != 1 {
		t.Fatalf("subscriptions.list: %+v", out)
	}

	env.MustDo(http.MethodDelete, fmt.Sprintf("/subscriptions/%d", usd.Id), nil, nil)
	expectStatus(t, env, http.MethodGet, fmt.Sprintf("/subscriptions/%d/events", usd.Id), nil, 404, "not_found")
}

func TestSubscriptionAccount(t *testing.T) {
	env, _ := setup(t)
	var created api.Subscription
	env.MustDo(http.MethodPost, "/subscriptions", api.SubscriptionInput{
		Name: "邮箱", Amount: 1, Cycle: "monthly", NextRenewal: day("2026-11-01"), Account: ptr("  a@b.c  "),
	}, &created)
	if created.Account == nil || *created.Account != "a@b.c" {
		t.Fatalf("created account: %+v", created.Account)
	}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/subscriptions/%d", created.Id), api.SubscriptionPatch{Name: ptr("邮箱2")}, &created)
	if created.Account == nil || *created.Account != "a@b.c" || created.Name != "邮箱2" {
		t.Fatalf("account changed with the name: %+v", created)
	}
	empty := ""
	env.MustDo(http.MethodPatch, fmt.Sprintf("/subscriptions/%d", created.Id), api.SubscriptionPatch{Account: &empty}, &created)
	if created.Account != nil && *created.Account != "" {
		t.Fatalf("cleared account: %+v", created.Account)
	}
	long := strings.Repeat("账", 201)
	status, raw := env.Do(http.MethodPost, "/subscriptions", api.SubscriptionInput{
		Name: "太长", Amount: 1, Cycle: "monthly", NextRenewal: day("2026-11-01"), Account: &long,
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(string(raw), "账号最多 200 个字") {
		t.Fatalf("long account: %d %s", status, raw)
	}
}

func TestExchangeRates(t *testing.T) {
	// Restarts reset the job timer: it has to come round well inside a day.
	if monitoring.RatesEvery > time.Hour {
		t.Fatalf("rates job every %s", monitoring.RatesEvery)
	}
	env, m := setup(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, `{"result":"success","time_last_update_unix":1,"rates":{"CNY":7.1,"USD":1,"EUR":0.9}}`)
	}))
	t.Cleanup(srv.Close)
	ctx := context.Background()
	if err := env.App.Deps.Settings.Set(ctx, monitoring.RatesURLKey, srv.URL); err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodPost, "/subscriptions", api.SubscriptionInput{
		Name: "国内", Amount: 100, Currency: ptr("CNY"), Cycle: "monthly", NextRenewal: day("2026-12-01"),
	}, nil)
	env.MustDo(http.MethodPost, "/subscriptions", api.SubscriptionInput{
		Name: "国外", Amount: 10, Currency: ptr("USD"), Cycle: "monthly", NextRenewal: day("2026-12-01"),
	}, nil)
	env.MustDo(http.MethodPost, "/subscriptions", api.SubscriptionInput{
		Name: "泰国", Amount: 30, Currency: ptr("THB"), Cycle: "monthly", NextRenewal: day("2026-12-01"),
	}, nil)

	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if err := m.RefreshRates(ctx, now); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("fetches: %d", hits.Load())
	}
	var sum api.SubscriptionSummary
	env.MustDo(http.MethodGet, "/subscriptions/summary", nil, &sum)
	if sum.Converted == nil || len(*sum.Converted) != 2 || sum.Unconverted == nil || sum.RatesAt == nil {
		t.Fatalf("summary: %+v", sum)
	}
	cny, usd := (*sum.Converted)[0], (*sum.Converted)[1]
	if cny.Currency != "CNY" || cny.Monthly != 171 || cny.Yearly != 2052 || cny.Count != 2 ||
		usd.Currency != "USD" || usd.Monthly != 24.08 || usd.Yearly != 288.96 || usd.Count != 2 ||
		len(*sum.Unconverted) != 1 || (*sum.Unconverted)[0] != "THB" {
		t.Fatalf("converted: %+v unconverted %v", *sum.Converted, *sum.Unconverted)
	}
	if err := m.RefreshRates(ctx, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("refetched inside 20h: %d", hits.Load())
	}
	if err := m.RefreshRates(ctx, now.Add(21*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("fetches after 21h: %d", hits.Load())
	}
}

func TestSubscriptionCategories(t *testing.T) {
	env, _ := setup(t)

	var cats []api.SubscriptionCategoryItem
	env.MustDo(http.MethodGet, "/subscription-categories", nil, &cats)
	if len(cats) != 4 || cats[0].Builtin == nil || *cats[0].Builtin != "server" || cats[3].Builtin == nil || *cats[3].Builtin != "other" {
		t.Fatalf("default categories: %+v", cats)
	}
	other := cats[3]

	var storage api.SubscriptionCategoryItem
	env.MustDo(http.MethodPost, "/subscription-categories", api.SubscriptionCategoryInput{Name: ptr(" 云存储 ")}, &storage)
	if storage.Name != "云存储" || storage.Position != 4 || storage.Builtin != nil {
		t.Fatalf("created: %+v", storage)
	}
	if status, _ := env.Do(http.MethodPost, "/subscription-categories", api.SubscriptionCategoryInput{Name: ptr("云存储")}, nil); status != http.StatusConflict {
		t.Fatalf("duplicate name: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/subscription-categories", api.SubscriptionCategoryInput{Name: ptr("  ")}, nil); status != http.StatusBadRequest {
		t.Fatalf("blank name: %d", status)
	}

	var sub api.Subscription
	env.MustDo(http.MethodPost, "/subscriptions", api.SubscriptionInput{
		Name: "网盘", Amount: 30, Cycle: "monthly", NextRenewal: day("2026-10-20"), CategoryId: &storage.Id,
	}, &sub)
	if sub.CategoryId == nil || *sub.CategoryId != storage.Id || sub.CategoryName == nil || *sub.CategoryName != "云存储" || sub.Category != "other" {
		t.Fatalf("subscription category: %+v", sub)
	}
	var sum api.SubscriptionSummary
	env.MustDo(http.MethodGet, "/subscriptions/summary", nil, &sum)
	if len(sum.ByCategory) != 1 || sum.ByCategory[0].CategoryName == nil || *sum.ByCategory[0].CategoryName != "云存储" {
		t.Fatalf("summary: %+v", sum.ByCategory)
	}
	env.MustDo(http.MethodGet, "/subscription-categories", nil, &cats)
	if cats[4].Count != 1 {
		t.Fatalf("count: %+v", cats)
	}

	var renamed api.SubscriptionCategoryItem
	env.MustDo(http.MethodPatch, fmt.Sprintf("/subscription-categories/%d", storage.Id), api.SubscriptionCategoryInput{Name: ptr("云盘")}, &renamed)
	if renamed.Name != "云盘" || renamed.Count != 1 {
		t.Fatalf("renamed: %+v", renamed)
	}
	if status, _ := env.Do(http.MethodPost, "/subscriptions", api.SubscriptionInput{
		Name: "x", Cycle: "monthly", NextRenewal: day("2026-10-20"), CategoryId: ptr(int64(999)),
	}, nil); status != http.StatusBadRequest {
		t.Fatalf("unknown category: %d", status)
	}

	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/subscription-categories/%d", other.Id), nil, nil); status != http.StatusBadRequest {
		t.Fatalf("delete other: %d", status)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/subscription-categories/%d", storage.Id), nil, nil)
	env.MustDo(http.MethodGet, fmt.Sprintf("/subscriptions/%d", sub.Id), nil, &sub)
	if sub.CategoryId == nil || *sub.CategoryId != other.Id || sub.Category != "other" {
		t.Fatalf("after delete: %+v", sub)
	}
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/subscription-categories/%d", storage.Id), nil, nil); status != http.StatusNotFound {
		t.Fatalf("delete twice: %d", status)
	}
}

func TestSubscriptionCycle(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()

	var sub api.Subscription
	env.MustDo(http.MethodPost, "/subscriptions", api.SubscriptionInput{
		Name: "季付", Amount: 90, Cycle: "monthly", CycleCount: ptr(3), CycleUnit: ptr(api.Month),
		NextRenewal: day("2026-10-01"), AutoRenew: ptr(true),
	}, &sub)
	if sub.CycleCount == nil || *sub.CycleCount != 3 || *sub.CycleUnit != "month" || sub.Cycle != "custom_days" || sub.CycleDays != 90 || sub.MonthlyCost != 30 {
		t.Fatalf("three months: %+v", sub)
	}
	// Auto renew moves the date by whole cycles.
	if err := m.ScanSubscriptions(ctx, time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodGet, fmt.Sprintf("/subscriptions/%d", sub.Id), nil, &sub)
	if sub.NextRenewal.Format("2006-01-02") != "2027-01-01" {
		t.Fatalf("auto renewed to %s", sub.NextRenewal.Format("2006-01-02"))
	}

	for _, bad := range []api.SubscriptionInput{
		{Name: "a", Cycle: "monthly", CycleCount: ptr(0), CycleUnit: ptr(api.Month), NextRenewal: day("2026-10-01")},
		{Name: "a", Cycle: "monthly", CycleCount: ptr(1001), CycleUnit: ptr(api.Day), NextRenewal: day("2026-10-01")},
		{Name: "a", Cycle: "monthly", CycleCount: ptr(2), NextRenewal: day("2026-10-01")},
		{Name: "a", Cycle: "monthly", CycleCount: ptr(2), CycleUnit: ptr(api.SubscriptionCycleUnit("second")), NextRenewal: day("2026-10-01")},
	} {
		if status, raw := env.Do(http.MethodPost, "/subscriptions", bad, nil); status != http.StatusBadRequest {
			t.Fatalf("%+v: %d %s", bad, status, raw)
		}
	}

	// An old client sends only cycle and category. Both old and new fields come back.
	server := api.SubscriptionCategory("server")
	var old api.Subscription
	env.MustDo(http.MethodPost, "/subscriptions", api.SubscriptionInput{
		Name: "旧前端", Amount: 12, Cycle: "custom_days", CycleDays: ptr(14), Category: &server, NextRenewal: day("2026-10-01"),
	}, &old)
	if old.Category != "server" || old.CategoryName == nil || *old.CategoryName != "服务器" || old.Cycle != "custom_days" || old.CycleDays != 14 ||
		*old.CycleCount != 14 || *old.CycleUnit != "day" {
		t.Fatalf("old client: %+v", old)
	}
	var yearly api.Subscription
	env.MustDo(http.MethodPatch, fmt.Sprintf("/subscriptions/%d", old.Id), api.SubscriptionPatch{Cycle: ptr(api.Yearly)}, &yearly)
	if yearly.Cycle != "yearly" || *yearly.CycleCount != 1 || *yearly.CycleUnit != "year" {
		t.Fatalf("patched to yearly: %+v", yearly)
	}
}
