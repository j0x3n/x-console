package reminders_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type fakeReminderSource struct {
	items []contracts.ExternalReminder
	err   error
}

func (f fakeReminderSource) Upcoming(context.Context, time.Time, time.Time) ([]contracts.ExternalReminder, error) {
	return f.items, f.err
}

func TestExternalRemindersRangesAndSources(t *testing.T) {
	env := testutil.New(t)
	loc := env.App.Deps.Scheduler.Location()
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	module.Provide[contracts.ReminderSource](env.App.Deps.Registry, contracts.ReminderSourcePrefix+"good", fakeReminderSource{items: []contracts.ExternalReminder{
		{ID: "old", Source: "custom", SourceLabel: "自定义", Title: "昨天", At: today.Add(-time.Hour)},
		{ID: "far", Source: "issue", Title: "太远", At: today.AddDate(0, 0, 32)},
	}})
	module.Provide[contracts.ReminderSource](env.App.Deps.Registry, contracts.ReminderSourcePrefix+"bad", fakeReminderSource{err: errors.New("broken")})
	var todayItems struct {
		Items []api.ExternalReminder `json:"items"`
	}
	env.MustDo("GET", "/reminders/external?range=today", nil, &todayItems)
	if len(todayItems.Items) != 1 || todayItems.Items[0].Id != "old" || todayItems.Items[0].Source != "other" {
		t.Fatalf("today: %+v", todayItems.Items)
	}
	var project struct {
		Id int64 `json:"id"`
	}
	env.MustDo("POST", "/projects", map[string]any{"key": "EXT", "name": "汇总"}, &project)
	when := today.AddDate(0, 0, 1).Add(12 * time.Hour).UTC().Format(time.RFC3339)
	var issue struct {
		Key string `json:"key"`
	}
	env.MustDo("POST", fmt.Sprintf("/projects/%d/issues", project.Id), map[string]any{"title": "明天截止", "dueAt": when}, &issue)
	renewal := today.AddDate(0, 0, 3).Format(time.DateOnly)
	env.MustDo("POST", "/subscriptions", map[string]any{"name": "云服务", "amount": 20, "cycle": "monthly", "nextRenewal": renewal}, nil)
	var upcoming struct {
		Items []api.ExternalReminder `json:"items"`
	}
	env.MustDo("GET", "/reminders/external?range=upcoming", nil, &upcoming)
	if len(upcoming.Items) != 2 || upcoming.Items[0].Source != "issue" || upcoming.Items[1].Source != "subscription" {
		t.Fatalf("upcoming: %+v", upcoming.Items)
	}
	if upcoming.Items[0].Link != "/projects/EXT/1" || upcoming.Items[1].Link != "/monitoring/subscriptions" {
		t.Fatalf("links: %+v", upcoming.Items)
	}
	env.MustDo("PATCH", "/issues/"+issue.Key, map[string]any{"status": "done"}, nil)
	env.MustDo("GET", "/reminders/external?range=upcoming", nil, &upcoming)
	if len(upcoming.Items) != 1 || upcoming.Items[0].Source != "subscription" {
		t.Fatalf("completed issue: %+v", upcoming.Items)
	}
}

func TestExternalMonitorExpiry(t *testing.T) {
	env := testutil.New(t)
	loc := env.App.Deps.Scheduler.Location()
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	at := today.AddDate(0, 0, 2).Add(10 * time.Hour)
	for _, kind := range []string{"tls", "domain"} {
		_, err := env.App.Deps.DB.Exec(`INSERT INTO monitors(kind,name,target,interval_seconds,expires_at,created_at) VALUES(?,?,?,?,?,?)`, kind, kind, "example.com", 86400, at, time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
	var list struct {
		Items []api.ExternalReminder `json:"items"`
	}
	env.MustDo("GET", "/reminders/external?range=upcoming", nil, &list)
	if len(list.Items) != 2 || list.Items[0].Source != "certificate" || list.Items[1].Source != "domain" {
		t.Fatalf("monitor expiry: %+v", list.Items)
	}
	for _, item := range list.Items {
		if item.Link != "/monitoring/certs" {
			t.Fatalf("link: %+v", item)
		}
	}
}
