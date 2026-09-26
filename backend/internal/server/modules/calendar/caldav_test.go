package calendar_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"

	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// fakeCalDAV is an in-memory, read-only CalDAV backend with one principal,
// one event calendar and one task list.
type fakeCalDAV struct {
	objects map[string][]caldav.CalendarObject // calendar path -> objects
}

const (
	principalPath = "/alice/"
	homePath      = "/alice/calendars/"
	workPath      = "/alice/calendars/work/"
	tasksPath     = "/alice/calendars/tasks/"
)

func (b *fakeCalDAV) CurrentUserPrincipal(context.Context) (string, error) { return principalPath, nil }
func (b *fakeCalDAV) CalendarHomeSetPath(context.Context) (string, error)  { return homePath, nil }
func (b *fakeCalDAV) CreateCalendar(context.Context, *caldav.Calendar) error {
	return webdav.NewHTTPError(http.StatusForbidden, nil)
}

func (b *fakeCalDAV) ListCalendars(context.Context) ([]caldav.Calendar, error) {
	return []caldav.Calendar{
		{Path: workPath, Name: "Work", SupportedComponentSet: []string{"VEVENT"}},
		{Path: tasksPath, Name: "Tasks", SupportedComponentSet: []string{"VTODO"}},
	}, nil
}

func (b *fakeCalDAV) GetCalendar(ctx context.Context, p string) (*caldav.Calendar, error) {
	cals, _ := b.ListCalendars(ctx)
	for _, c := range cals {
		if strings.TrimSuffix(c.Path, "/") == strings.TrimSuffix(p, "/") {
			return &c, nil
		}
	}
	return nil, webdav.NewHTTPError(http.StatusNotFound, nil)
}

func (b *fakeCalDAV) GetCalendarObject(_ context.Context, p string, _ *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	for _, objs := range b.objects {
		for _, o := range objs {
			if o.Path == p {
				return &o, nil
			}
		}
	}
	return nil, webdav.NewHTTPError(http.StatusNotFound, nil)
}

func (b *fakeCalDAV) ListCalendarObjects(_ context.Context, p string, _ *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	return b.objects[strings.TrimSuffix(p, "/")+"/"], nil
}

func (b *fakeCalDAV) QueryCalendarObjects(ctx context.Context, p string, _ *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	return b.ListCalendarObjects(ctx, p, nil)
}

func (b *fakeCalDAV) PutCalendarObject(context.Context, string, *ical.Calendar, *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	return nil, webdav.NewHTTPError(http.StatusForbidden, nil)
}

func (b *fakeCalDAV) DeleteCalendarObject(context.Context, string) error {
	return webdav.NewHTTPError(http.StatusForbidden, nil)
}

func mustICal(t *testing.T, s string) *ical.Calendar {
	t.Helper()
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", "\r\n") + "\r\n"
	cal, err := ical.NewDecoder(strings.NewReader(s)).Decode()
	if err != nil {
		t.Fatal(err)
	}
	return cal
}

func newCalDAVServer(t *testing.T) *httptest.Server {
	t.Helper()
	backend := &fakeCalDAV{objects: map[string][]caldav.CalendarObject{
		workPath: {
			{Path: workPath + "planning.ics", ModTime: time.Now(), ETag: `"1"`, Data: mustICal(t, `
BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//fake//EN
BEGIN:VEVENT
UID:planning@fake
DTSTAMP:20261001T000000Z
DTSTART;TZID=Europe/Berlin:20261020T100000
DTEND;TZID=Europe/Berlin:20261020T110000
RRULE:FREQ=WEEKLY;COUNT=3
SUMMARY:Planning
END:VEVENT
BEGIN:VEVENT
UID:planning@fake
DTSTAMP:20261001T000000Z
RECURRENCE-ID;TZID=Europe/Berlin:20261027T100000
DTSTART;TZID=Europe/Berlin:20261027T140000
DTEND;TZID=Europe/Berlin:20261027T150000
SUMMARY:Planning (late)
END:VEVENT
END:VCALENDAR`)},
			{Path: workPath + "offsite.ics", ModTime: time.Now(), ETag: `"2"`, Data: mustICal(t, `
BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//fake//EN
BEGIN:VEVENT
UID:offsite@fake
DTSTAMP:20261001T000000Z
DTSTART;VALUE=DATE:20261022
SUMMARY:Offsite
END:VEVENT
END:VCALENDAR`)},
		},
		tasksPath: {
			{Path: tasksPath + "todo.ics", ModTime: time.Now(), ETag: `"3"`, Data: mustICal(t, `
BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//fake//EN
BEGIN:VEVENT
UID:should-not-load@fake
DTSTAMP:20261001T000000Z
DTSTART:20261021T000000Z
SUMMARY:Hidden
END:VEVENT
END:VCALENDAR`)},
		},
	}}
	h := &caldav.Handler{Backend: backend}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "alice" || pass != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCalDAVSync(t *testing.T) {
	env := testutil.New(t)
	srv := newCalDAVServer(t)
	from := time.Date(2026, 10, 19, 0, 0, 0, 0, shanghai)
	want := strings.Join([]string{
		"Planning 10-20T08:00-09:00",        // 10:00 Berlin, summer time
		"Offsite 2026-10-22..2026-10-23",    // all-day without DTEND
		"Planning (late) 10-27T13:00-14:00", // override, 14:00 Berlin after DST ended
		"Planning 11-03T09:00-10:00",        // 10:00 Berlin, winter time
	}, "\n")

	for _, u := range []string{srv.URL + workPath, srv.URL + "/"} {
		cal := createCalendar(t, env, map[string]any{"name": "Fastmail", "kind": "caldav", "url": u, "username": "alice", "password": "pw", "enabled": false})
		env.MustDo(http.MethodPatch, fmt.Sprintf("/calendars/%d", cal.Id), map[string]any{"enabled": true}, nil)
		cal = syncCalendar(t, env, cal.Id)
		if cal.LastError != "" || cal.EventCount != 3 {
			t.Fatalf("%s: %+v", u, cal)
		}
		got := strings.Join(describe(listEvents(t, env, from, from.AddDate(0, 0, 21))), "\n")
		if got != want {
			t.Fatalf("%s:\n%s\nwant:\n%s", u, got, want)
		}
		env.MustDo(http.MethodDelete, fmt.Sprintf("/calendars/%d", cal.Id), nil, nil)
	}

	cal := createCalendar(t, env, map[string]any{"name": "Bad", "kind": "caldav", "url": srv.URL + workPath, "username": "alice", "password": "wrong", "enabled": false})
	cal = syncCalendar(t, env, cal.Id)
	if cal.LastError == "" || strings.Contains(cal.LastError, srv.URL) {
		t.Fatalf("wrong password: %+v", cal)
	}
}
