package monitoring_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
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
	if vps.Currency != "CNY" || len(vps.RemindDaysBefore) != 2 || vps.RemindDaysBefore[0] != 7 || vps.AutoRenew {
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
