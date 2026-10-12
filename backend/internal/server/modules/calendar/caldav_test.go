package calendar_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"

	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// fakeCalDAV is an in-memory CalDAV backend with one principal,
// one event calendar and one task list.
type fakeCalDAV struct {
	mu        sync.Mutex
	objects   map[string][]caldav.CalendarObject // calendar path -> objects
	lastMatch string
	reject    bool
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
	b.mu.Lock()
	defer b.mu.Unlock()
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
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]caldav.CalendarObject(nil), b.objects[strings.TrimSuffix(p, "/")+"/"]...), nil
}

func (b *fakeCalDAV) QueryCalendarObjects(ctx context.Context, p string, _ *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	return b.ListCalendarObjects(ctx, p, nil)
}

func (b *fakeCalDAV) PutCalendarObject(_ context.Context, p string, data *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastMatch = string(opts.IfMatch)
	if b.reject {
		return nil, webdav.NewHTTPError(http.StatusForbidden, nil)
	}
	for key, objects := range b.objects {
		for i, object := range objects {
			if object.Path != p {
				continue
			}
			if opts.IfNoneMatch == "*" || opts.IfMatch != "" && string(opts.IfMatch) != strconv.Quote(object.ETag) {
				return nil, webdav.NewHTTPError(http.StatusPreconditionFailed, nil)
			}
			object.ETag = "updated"
			object.Data = data
			b.objects[key][i] = object
			return &object, nil
		}
	}
	if opts.IfMatch != "" {
		return nil, webdav.NewHTTPError(http.StatusPreconditionFailed, nil)
	}
	object := caldav.CalendarObject{Path: p, ETag: "created", Data: data, ModTime: time.Now()}
	b.objects[workPath] = append(b.objects[workPath], object)
	return &object, nil
}

func (b *fakeCalDAV) DeleteCalendarObject(_ context.Context, p string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, objects := range b.objects {
		for i, object := range objects {
			if object.Path == p {
				b.objects[key] = append(objects[:i], objects[i+1:]...)
				return nil
			}
		}
	}
	return webdav.NewHTTPError(http.StatusNotFound, nil)
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
			{Path: workPath + "planning.ics", ModTime: time.Now(), ETag: "1", Data: mustICal(t, `
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
			{Path: workPath + "offsite.ics", ModTime: time.Now(), ETag: "2", Data: mustICal(t, `
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
			{Path: tasksPath + "todo.ics", ModTime: time.Now(), ETag: "3", Data: mustICal(t, `
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

func TestCalDAVSyncSkipsMissingCalendarData(t *testing.T) {
	env := testutil.New(t)
	raw, err := os.ReadFile("testdata/icloud-calendar-query-404.xml")
	if err != nil {
		t.Fatal(err)
	}
	var mode atomic.Int32
	var writes atomic.Int32
	backend := &fakeCalDAV{objects: map[string][]caldav.CalendarObject{workPath: {}}}
	h := &caldav.Handler{Backend: backend}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "alice" || pass != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != "REPORT" {
			if r.URL.Path == workPath+"meeting one.ics" {
				version := `"icloud-good-1"`
				if n := writes.Load(); n > 0 {
					version = fmt.Sprintf(`"icloud-updated-%d"`, n)
				}
				switch r.Method {
				case http.MethodPut, http.MethodDelete:
					if r.Header.Get("If-Match") != version {
						t.Errorf("If-Match %q，预期 %q", r.Header.Get("If-Match"), version)
						w.WriteHeader(http.StatusPreconditionFailed)
						return
					}
					if r.Method == http.MethodPut {
						// PUT 不返回 ETag，验证后续 GET 回读。
						writes.Add(1)
					}
					w.WriteHeader(http.StatusNoContent)
					return
				case http.MethodGet:
					w.Header().Set("ETag", version)
					w.Header().Set("Content-Type", "text/calendar")
					_, _ = fmt.Fprint(w, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nBEGIN:VEVENT\r\nUID:icloud-good-1\r\nDTSTART:20261020T020000Z\r\nSUMMARY:会议\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
					return
				}
			}
			h.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Depth") != "1" {
			t.Error("REPORT 缺少 Depth: 1")
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		switch mode.Load() {
		case 1:
			_, _ = fmt.Fprint(w, `<D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:response><D:href>/alice/calendars/work/deleted.ics</D:href><D:propstat><D:prop><C:calendar-data/></D:prop><D:status>HTTP/1.1 404 Not Found</D:status></D:propstat></D:response></D:multistatus>`)
		case 2:
			_, _ = fmt.Fprint(w, `<D:multistatus xmlns:D="DAV:"/>`)
		case 3:
			_, _ = fmt.Fprint(w, `<D:multistatus xmlns:D="DAV:"><D:response>`)
		default:
			_, _ = w.Write(raw)
		}
	}))
	t.Cleanup(srv.Close)
	// 先确认样例确实会让原来的解析器整次失败。
	client, err := caldav.NewClient(webdav.HTTPClientWithBasicAuth(http.DefaultClient, "alice", "pw"), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.QueryCalendar(context.Background(), workPath, &caldav.CalendarQuery{
		CompRequest: caldav.CalendarCompRequest{Name: "VCALENDAR", AllProps: true, AllComps: true},
		CompFilter:  caldav.CompFilter{Name: "VCALENDAR", Comps: []caldav.CompFilter{{Name: "VEVENT"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "calendar-data>: 404 Not Found") {
		t.Fatalf("未复现原错误: %v", err)
	}
	cal := createCalendar(t, env, map[string]any{"name": "iCloud", "kind": "caldav", "url": srv.URL + workPath,
		"username": "alice", "password": "pw", "enabled": false})
	cal = syncCalendar(t, env, cal.Id)
	if cal.LastError != "" || cal.EventCount != 2 {
		t.Fatalf("部分失败阻止了同步: %+v", cal)
	}
	var href, etag string
	if err := env.App.Deps.DB.QueryRow("SELECT href,etag FROM calendar_events WHERE calendar_id=? AND uid='icloud-good-1'", cal.Id).Scan(&href, &etag); err != nil {
		t.Fatal(err)
	}
	if href != workPath+"meeting one.ics" || etag != `"icloud-good-1"` {
		t.Fatalf("远端版本或地址丢失: %q %q", href, etag)
	}
	var id int64
	if err := env.App.Deps.DB.QueryRow("SELECT id FROM calendar_events WHERE calendar_id=? AND uid='icloud-good-1'", cal.Id).Scan(&id); err != nil {
		t.Fatal(err)
	}
	var event api.CalendarEvent
	for _, title := range []string{"会议改期", "会议再次改期"} {
		env.MustDo(http.MethodPatch, fmt.Sprintf("/calendar/events/%d", id), map[string]any{"title": title}, &event)
		if event.Title != title {
			t.Fatalf("同步后的日程无法修改: %+v", event)
		}
	}
	for _, value := range []int32{1, 3} {
		mode.Store(value)
		cal = syncCalendar(t, env, cal.Id)
		if cal.LastError == "" || cal.EventCount != 2 {
			t.Fatalf("失败同步未保留旧日程: %+v", cal)
		}
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/calendar/events/%d", id), nil, nil)
	mode.Store(2)
	cal = syncCalendar(t, env, cal.Id)
	if cal.LastError != "" || cal.EventCount != 0 {
		t.Fatalf("空日历未清除旧日程: %+v", cal)
	}
}
