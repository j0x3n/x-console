package calendar_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-webdav/caldav"

	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestLocalEventCRUD(t *testing.T) {
	env := testutil.New(t)
	cal := createCalendar(t, env, map[string]any{"name": "私人日历", "kind": "local", "url": ""})
	if cal.Writable == nil || !*cal.Writable || cal.LastSyncedAt != nil {
		t.Fatalf("local calendar: %+v", cal)
	}
	start := time.Date(2026, 10, 20, 10, 0, 0, 0, shanghai)
	end := start.Add(time.Hour)
	var event api.CalendarEvent
	env.MustDo(http.MethodPost, "/calendar/events", map[string]any{
		"calendarId": cal.Id, "title": "开会", "allDay": false, "start": start, "end": end,
	}, &event)
	if event.Title != "开会" || !event.Start.Equal(start) || event.Writable == nil || !*event.Writable {
		t.Fatalf("created: %+v", event)
	}
	path := fmt.Sprintf("/calendar/events/%d", event.EventId)
	start = start.Add(2 * time.Hour)
	end = end.Add(2 * time.Hour)
	env.MustDo(http.MethodPatch, path, map[string]any{"start": start, "end": end}, &event)
	if !event.Start.Equal(start) || len(listEvents(t, env, start.Add(-time.Hour), end.Add(time.Hour))) != 1 {
		t.Fatalf("moved: %+v", event)
	}
	env.MustDo(http.MethodPatch, path, map[string]any{
		"allDay": true, "startDate": "2026-10-21", "endDate": "2026-10-23",
	}, &event)
	if !event.AllDay || *event.StartDate != "2026-10-21" || *event.EndDate != "2026-10-23" {
		t.Fatalf("all day: %+v", event)
	}
	list := listEvents(t, env, start.AddDate(0, 0, 1), end.AddDate(0, 0, 3))
	if len(list) != 1 || list[0].Title != "开会" {
		t.Fatalf("listed: %+v", list)
	}
	env.MustDo(http.MethodDelete, path, nil, nil)
	if got := listEvents(t, env, start.AddDate(0, 0, 1), end.AddDate(0, 0, 3)); len(got) != 0 {
		t.Fatalf("after delete: %+v", got)
	}
}

func TestCalDAVWritebackAndConflict(t *testing.T) {
	env := testutil.New(t)
	backend := &fakeCalDAV{objects: map[string][]caldav.CalendarObject{workPath: {}}}
	h := &caldav.Handler{Backend: backend}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "alice" || pass != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodDelete {
			backend.mu.Lock()
			for _, obj := range backend.objects[workPath] {
				if obj.Path == r.URL.Path && r.Header.Get("If-Match") != obj.ETag {
					backend.mu.Unlock()
					w.WriteHeader(http.StatusPreconditionFailed)
					return
				}
			}
			backend.mu.Unlock()
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	cal := createCalendar(t, env, map[string]any{"name": "远端", "kind": "caldav", "url": srv.URL + workPath,
		"username": "alice", "password": "pw", "enabled": false})
	backend.mu.Lock()
	backend.reject = true
	backend.mu.Unlock()
	status, _ := env.Do(http.MethodPost, "/calendar/events", map[string]any{
		"calendarId": cal.Id, "title": "不能保存", "allDay": true,
		"startDate": "2026-10-20", "endDate": "2026-10-21",
	}, nil)
	if status != http.StatusBadGateway {
		t.Fatalf("failed PUT: %d", status)
	}
	var count int
	if err := env.App.Deps.DB.QueryRow(`SELECT count(*) FROM calendar_events WHERE calendar_id = ?`, cal.Id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed PUT saved event: %d %v", count, err)
	}
	backend.mu.Lock()
	backend.reject = false
	backend.mu.Unlock()
	var event api.CalendarEvent
	start := time.Date(2026, 10, 20, 10, 0, 0, 0, shanghai)
	env.MustDo(http.MethodPost, "/calendar/events", map[string]any{
		"calendarId": cal.Id, "title": "远端会议", "allDay": false,
		"start": start, "end": start.Add(time.Hour),
	}, &event)
	backend.mu.Lock()
	if len(backend.objects[workPath]) != 1 {
		t.Fatalf("remote objects: %+v", backend.objects)
	}
	remote := backend.objects[workPath][0]
	backend.mu.Unlock()
	if len(remote.Data.Events()) != 1 {
		t.Fatal("remote VEVENT missing")
	}
	title, _ := remote.Data.Events()[0].Props.Text("SUMMARY")
	if title != "远端会议" {
		t.Fatalf("remote title: %s", title)
	}
	path := fmt.Sprintf("/calendar/events/%d", event.EventId)
	env.MustDo(http.MethodPatch, path, map[string]any{"title": "远端会议改期"}, &event)
	backend.mu.Lock()
	match := backend.lastMatch
	backend.objects[workPath][0].ETag = `"elsewhere"`
	backend.mu.Unlock()
	if match != `"created"` {
		t.Fatalf("If-Match: %q", match)
	}
	status, body := env.Do(http.MethodPatch, path, map[string]any{"title": "覆盖远端"}, nil)
	if status != http.StatusConflict || !strings.Contains(string(body), "日程在别处改过了") {
		t.Fatalf("conflict: %d %s", status, body)
	}
	var savedTitle string
	if err := env.App.Deps.DB.QueryRow(`SELECT title FROM calendar_events WHERE id = ?`, event.EventId).Scan(&savedTitle); err != nil || savedTitle != "远端会议改期" {
		t.Fatalf("conflict changed local event: %q %v", savedTitle, err)
	}
	backend.mu.Lock()
	backend.objects[workPath][0].ETag = `"updated"`
	backend.mu.Unlock()
	env.MustDo(http.MethodDelete, path, nil, nil)
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.objects[workPath]) != 0 {
		t.Fatalf("delete writeback: %+v", backend.objects)
	}
}

func TestReadOnlyAndRecurringEventsRejectWrites(t *testing.T) {
	env := testutil.New(t)
	ics := newICSServer(t)
	cal := createCalendar(t, env, map[string]any{"name": "订阅", "kind": "ics", "url": ics.URL,
		"username": "jo", "password": "s3cret", "enabled": false})
	status, _ := env.Do(http.MethodPost, "/calendar/events", map[string]any{
		"calendarId": cal.Id, "title": "禁写", "allDay": true,
		"startDate": "2026-10-20", "endDate": "2026-10-21",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("ICS create: %d", status)
	}
	cal = syncCalendar(t, env, cal.Id)
	list := listEvents(t, env, time.Date(2026, 10, 1, 0, 0, 0, 0, shanghai), time.Date(2026, 10, 31, 0, 0, 0, 0, shanghai))
	if len(list) != 0 {
		t.Fatalf("disabled ICS: %+v", list)
	}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/calendars/%d", cal.Id), map[string]any{"enabled": true}, nil)
	list = listEvents(t, env, time.Date(2026, 10, 1, 0, 0, 0, 0, shanghai), time.Date(2026, 10, 31, 0, 0, 0, 0, shanghai))
	if len(list) == 0 || list[0].Writable == nil || *list[0].Writable {
		t.Fatalf("ICS writable: %+v", list)
	}
	status, _ = env.Do(http.MethodPatch, fmt.Sprintf("/calendar/events/%d", list[0].EventId), map[string]any{"title": "禁改"}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("ICS update: %d", status)
	}
	remote := newCalDAVServer(t)
	cal = createCalendar(t, env, map[string]any{"name": "重复", "kind": "caldav", "url": remote.URL + workPath,
		"username": "alice", "password": "pw", "enabled": false})
	_ = syncCalendar(t, env, cal.Id)
	env.MustDo(http.MethodPatch, fmt.Sprintf("/calendars/%d", cal.Id), map[string]any{"enabled": true}, nil)
	list = listEvents(t, env, time.Date(2026, 10, 19, 0, 0, 0, 0, shanghai), time.Date(2026, 10, 29, 0, 0, 0, 0, shanghai))
	for _, e := range list {
		if !strings.HasPrefix(e.Title, "Planning") {
			continue
		}
		if e.Writable == nil || *e.Writable {
			t.Fatalf("recurring event should be read only: %+v", e)
		}
		status, _ = env.Do(http.MethodDelete, fmt.Sprintf("/calendar/events/%d", e.EventId), nil, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("recurring delete: %d", status)
		}
		return
	}
	t.Fatal("recurring event missing")
}
