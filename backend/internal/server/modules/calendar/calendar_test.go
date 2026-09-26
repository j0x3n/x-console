package calendar_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

var shanghai, _ = time.LoadLocation("Asia/Shanghai")

// icsServer serves testdata/sample.ics behind basic auth. Setting fail makes
// it answer 500.
type icsServer struct {
	*httptest.Server
	fail atomic.Bool
	hits atomic.Int32
}

func newICSServer(t *testing.T) *icsServer {
	t.Helper()
	data, err := os.ReadFile("testdata/sample.ics")
	if err != nil {
		t.Fatal(err)
	}
	s := &icsServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		if user, pass, ok := r.BasicAuth(); !ok || user != "jo" || pass != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if s.fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write(data)
	}))
	t.Cleanup(s.Close)
	return s
}

func createCalendar(t *testing.T, env *testutil.Env, body map[string]any) api.Calendar {
	t.Helper()
	var c api.Calendar
	env.MustDo(http.MethodPost, "/calendars", body, &c)
	return c
}

func syncCalendar(t *testing.T, env *testutil.Env, id int64) api.Calendar {
	t.Helper()
	var c api.Calendar
	env.MustDo(http.MethodPost, fmt.Sprintf("/calendars/%d/sync", id), nil, &c)
	return c
}

func listEvents(t *testing.T, env *testutil.Env, from, to time.Time) []api.CalendarEvent {
	t.Helper()
	q := url.Values{"from": {from.Format(time.RFC3339)}, "to": {to.Format(time.RFC3339)}}
	var out []api.CalendarEvent
	env.MustDo(http.MethodGet, "/calendar/events?"+q.Encode(), nil, &out)
	return out
}

func describe(events []api.CalendarEvent) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		if e.AllDay {
			out = append(out, fmt.Sprintf("%s %s..%s", e.Title, *e.StartDate, *e.EndDate))
			continue
		}
		out = append(out, fmt.Sprintf("%s %s-%s", e.Title, e.Start.UTC().Format("01-02T15:04"), e.End.UTC().Format("15:04")))
	}
	return out
}

func TestICSSyncExpandsRecurringEventsInTimeZones(t *testing.T) {
	env := testutil.New(t)
	srv := newICSServer(t)

	cal := createCalendar(t, env, map[string]any{
		"name": "Work", "kind": "ics", "url": srv.URL + "/cal.ics", "username": "jo", "password": "s3cret", "color": "#3366FF",
	})
	if !cal.HasPassword || cal.Color != "#3366ff" || !cal.Enabled {
		t.Fatalf("created: %+v", cal)
	}
	cal = syncCalendar(t, env, cal.Id)
	if cal.LastError != "" || cal.LastSyncedAt == nil {
		t.Fatalf("sync: %+v", cal)
	}
	// 9 VEVENTs; the cancelled single event is dropped and the cancelled
	// occurrence becomes an EXDATE of its master.
	if cal.EventCount != 8 {
		t.Fatalf("event count = %d", cal.EventCount)
	}

	from := time.Date(2026, 10, 19, 0, 0, 0, 0, shanghai)
	got := describe(listEvents(t, env, from, from.AddDate(0, 0, 21)))
	want := []string{
		"Standup 10-19T13:00-13:30",         // 09:00 New York, summer time
		"Standup 10-21T13:00-13:30",         //
		"Deploy 10-22T02:00-03:00",          // UTC
		"Lunch 10-23T04:00-05:00",           // floating, read in the user's zone
		"Birthday 2026-10-24..2026-10-25",   // yearly all-day
		"Standup (moved) 10-26T15:00-15:30", // override; Wednesday 10-28 is excluded
		"Review 10-27T07:00-08:30",          // Windows zone name, DURATION
		"Trip 2026-10-30..2026-11-01",       // two-day all-day event
		"Pill 11-01T00:00-00:00",            // daily, COUNT=3
		"Pill 11-02T00:00-00:00",            //
		"Standup 11-02T14:00-14:30",         // 09:00 New York after DST ended
		"Pill 11-03T00:00-00:00",            // 11-04 standup was cancelled
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("events:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// A week view starting mid-event still shows the running all-day trip.
	week := listEvents(t, env, time.Date(2026, 10, 31, 0, 0, 0, 0, shanghai), time.Date(2026, 11, 1, 0, 0, 0, 0, shanghai))
	if d := describe(week); len(d) != 1 || d[0] != "Trip 2026-10-30..2026-11-01" {
		t.Fatalf("day view: %v", d)
	}
	for _, e := range listEvents(t, env, from, from.AddDate(0, 0, 7)) {
		if e.Title == "Deploy" {
			if e.Location != "Room 1, Floor 2" || e.Description != "First line\nSecond line" || e.Calendar != "Work" || e.Color != "#3366ff" {
				t.Fatalf("deploy: %+v", e)
			}
		}
		if e.Title == "Standup" && !e.Recurring {
			t.Fatalf("standup not marked recurring")
		}
	}

	// contracts.Calendar gives the same occurrences.
	svc, ok := module.Lookup[contracts.Calendar](env.App.Deps.Registry, contracts.CalendarKey)
	if !ok {
		t.Fatal("contracts.Calendar not provided")
	}
	evs, err := svc.Events(context.Background(), from, from.AddDate(0, 0, 21))
	if err != nil || len(evs) != len(want) || evs[0].Title != "Standup" || evs[0].Calendar != "Work" {
		t.Fatalf("contract events: %v %+v", err, evs)
	}

	// The action defaults to today and accepts a range.
	raw, _ := json.Marshal(map[string]any{"from": from.Format(time.RFC3339), "to": from.AddDate(0, 0, 4).Format(time.RFC3339)})
	res, err := env.App.Deps.Actions.Run(context.Background(), "calendar.events", raw)
	if err != nil {
		t.Fatal(err)
	}
	if list := res.([]api.CalendarEvent); len(list) != 3 {
		t.Fatalf("action events: %d", len(list))
	}
	if _, err := env.App.Deps.Actions.Run(context.Background(), "calendar.events", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.App.Deps.Actions.Run(context.Background(), "calendar.events", json.RawMessage(`{"x":1}`)); err == nil {
		t.Fatal("unknown field accepted")
	}

	// Disabled calendars are hidden.
	env.MustDo(http.MethodPatch, fmt.Sprintf("/calendars/%d", cal.Id), map[string]any{"enabled": false}, nil)
	if got := listEvents(t, env, from, from.AddDate(0, 0, 21)); len(got) != 0 {
		t.Fatalf("disabled calendar still listed: %d", len(got))
	}
}

func TestSyncFailureKeepsEventsAndPasswordStaysSecret(t *testing.T) {
	env := testutil.New(t)
	srv := newICSServer(t)
	cal := createCalendar(t, env, map[string]any{"name": "Work", "kind": "ics", "url": srv.URL, "username": "jo", "password": "s3cret", "enabled": false})
	cal = syncCalendar(t, env, cal.Id)
	if cal.EventCount != 8 {
		t.Fatalf("count %d (%s)", cal.EventCount, cal.LastError)
	}

	srv.fail.Store(true)
	cal = syncCalendar(t, env, cal.Id)
	if cal.LastError == "" || strings.Contains(cal.LastError, srv.URL) || cal.EventCount != 8 {
		t.Fatalf("failed sync: %+v", cal)
	}

	// Wrong password: 401 is reported, URL stays out of the message.
	srv.fail.Store(false)
	env.MustDo(http.MethodPatch, fmt.Sprintf("/calendars/%d", cal.Id), map[string]any{"password": "nope"}, nil)
	cal = syncCalendar(t, env, cal.Id)
	if !strings.Contains(cal.LastError, "401") {
		t.Fatalf("wrong password: %q", cal.LastError)
	}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/calendars/%d", cal.Id), map[string]any{"password": "s3cret"}, nil)
	if cal = syncCalendar(t, env, cal.Id); cal.LastError != "" {
		t.Fatalf("fixed password: %q", cal.LastError)
	}

	var secret string
	if err := env.App.Deps.DB.QueryRow(`SELECT secret FROM calendars WHERE id = ?`, cal.Id).Scan(&secret); err != nil {
		t.Fatal(err)
	}
	if secret == "" || strings.Contains(secret, "s3cret") {
		t.Fatalf("password stored in plain text: %q", secret)
	}
	_, raw := env.Do(http.MethodGet, fmt.Sprintf("/calendars/%d", cal.Id), nil, nil)
	if strings.Contains(string(raw), "s3cret") || strings.Contains(string(raw), secret) {
		t.Fatalf("password leaked: %s", raw)
	}
}

func TestCalendarValidationAndNotFound(t *testing.T) {
	env := testutil.New(t)
	cases := []map[string]any{
		{"name": "", "kind": "ics", "url": "https://example.com/a.ics"},
		{"name": "x", "kind": "outlook", "url": "https://example.com/a.ics"},
		{"name": "x", "kind": "ics", "url": "ftp://example.com/a.ics"},
		{"name": "x", "kind": "caldav", "url": "webcal://example.com/a.ics"},
		{"name": "x", "kind": "ics", "url": "https://example.com/a.ics", "color": "blue"},
	}
	for _, c := range cases {
		if status, raw := env.Do(http.MethodPost, "/calendars", c, nil); status != http.StatusBadRequest {
			t.Fatalf("%v: %d %s", c, status, raw)
		}
	}
	for _, p := range []string{"/calendars/999"} {
		if status, _ := env.Do(http.MethodGet, p, nil, nil); status != http.StatusNotFound {
			t.Fatalf("GET %s: %d", p, status)
		}
		if status, _ := env.Do(http.MethodPatch, p, map[string]any{"name": "x"}, nil); status != http.StatusNotFound {
			t.Fatalf("PATCH %s: %d", p, status)
		}
		if status, _ := env.Do(http.MethodDelete, p, nil, nil); status != http.StatusNotFound {
			t.Fatalf("DELETE %s: %d", p, status)
		}
	}
	if status, _ := env.Do(http.MethodPost, "/calendars/999/sync", nil, nil); status != http.StatusNotFound {
		t.Fatalf("sync missing: %d", status)
	}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, shanghai)
	for _, r := range [][2]time.Time{{from, from}, {from, from.AddDate(0, 0, 101)}} {
		q := url.Values{"from": {r[0].Format(time.RFC3339)}, "to": {r[1].Format(time.RFC3339)}}
		if status, _ := env.Do(http.MethodGet, "/calendar/events?"+q.Encode(), nil, nil); status != http.StatusBadRequest {
			t.Fatalf("range %v: %d", r, status)
		}
	}
	if status, _ := env.Do(http.MethodGet, "/calendar/events?from=nope&to=nope", nil, nil); status != http.StatusBadRequest {
		t.Fatalf("bad time: %d", status)
	}

	// Create, then delete.
	srv := newICSServer(t)
	cal := createCalendar(t, env, map[string]any{"name": "Work", "kind": "ics", "url": "webcal://" + strings.TrimPrefix(srv.URL, "http://"), "enabled": false})
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/calendars/%d", cal.Id), nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	var list []api.Calendar
	env.MustDo(http.MethodGet, "/calendars", nil, &list)
	if len(list) != 0 {
		t.Fatalf("list after delete: %d", len(list))
	}
}

func TestCreateSyncsInBackground(t *testing.T) {
	env := testutil.New(t)
	srv := newICSServer(t)
	var mu sync.Mutex
	synced := false
	ch, cancel := env.App.Deps.Bus.Subscribe("calendar.synced", 4)
	defer cancel()
	go func() {
		for range ch {
			mu.Lock()
			synced = true
			mu.Unlock()
		}
	}()
	cal := createCalendar(t, env, map[string]any{"name": "Work", "kind": "ics", "url": srv.URL, "username": "jo", "password": "s3cret"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		done := synced
		mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no background sync")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var got api.Calendar
	env.MustDo(http.MethodGet, fmt.Sprintf("/calendars/%d", cal.Id), nil, &got)
	if got.EventCount != 8 || got.LastError != "" {
		t.Fatalf("after background sync: %+v", got)
	}
}
