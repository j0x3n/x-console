package calendar_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-webdav/caldav"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar"
	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// fakeGoogle answers the OAuth token, calendarList and CalDAV requests the
// way Google does, for one client and one account.
type fakeGoogle struct {
	*httptest.Server
	dav *fakeCalDAV

	mu      sync.Mutex
	expired bool
	revoked []string
	scope   string
}

const (
	googleClient  = "client.apps.googleusercontent.com"
	googleSecret  = "client-secret"
	googleCode    = "code"
	googleRefresh = "refresh-token"
	googleEmail   = "me@gmail.com"
	holidayID     = "zh.china#holiday@group.v.calendar.google.com"
)

var googlePrimaryPath = "/caldav/v2/" + url.PathEscape(googleEmail) + "/events/"

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	g := &fakeGoogle{dav: &fakeCalDAV{objects: map[string][]caldav.CalendarObject{
		googlePrimaryPath: {{Path: googlePrimaryPath + "standup.ics", ModTime: time.Now(), ETag: "7", Data: mustICal(t, `
BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Google Inc//Google Calendar 70.9054//EN
BEGIN:VEVENT
UID:standup@google.com
DTSTAMP:20261001T000000Z
DTSTART:20261020T020000Z
DTEND:20261020T030000Z
SUMMARY:谷歌上的会
END:VEVENT
END:VCALENDAR`)}},
	}}}
	h := &caldav.Handler{Backend: g.dav}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			g.token(w, r)
			return
		case "/revoke":
			r.ParseForm()
			g.mu.Lock()
			g.revoked = append(g.revoked, r.PostForm.Get("token"))
			g.mu.Unlock()
			return
		}
		if r.Header.Get("Authorization") != "Bearer access-"+googleRefresh {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/calendar/v3/users/me/calendarList" {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"items":[
				{"id":"`+googleEmail+`","summary":"`+googleEmail+`","backgroundColor":"#9FC6E7","accessRole":"owner","primary":true},
				{"id":"`+holidayID+`","summary":"中国的节假日","backgroundColor":"#16a765","accessRole":"reader"}]}`)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(g.Close)
	return g
}

func (g *fakeGoogle) token(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	f := r.PostForm
	w.Header().Set("Content-Type", "application/json")
	if f.Get("client_id") != googleClient || f.Get("client_secret") != googleSecret {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"invalid_client"}`)
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	switch {
	case f.Get("grant_type") == "authorization_code" && f.Get("code") == googleCode:
		g.expired = false
		json.NewEncoder(w).Encode(map[string]any{"access_token": "access-" + googleRefresh, "refresh_token": googleRefresh, "expires_in": 3600, "scope": g.scope})
	case f.Get("grant_type") == "refresh_token" && !g.expired && f.Get("refresh_token") == googleRefresh:
		json.NewEncoder(w).Encode(map[string]any{"access_token": "access-" + googleRefresh, "expires_in": 3600})
	default:
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"invalid_grant"}`)
	}
}

func (g *fakeGoogle) endpoints() files.GoogleEndpoints {
	return files.GoogleEndpoints{Auth: g.URL + "/auth", Token: g.URL + "/token", Revoke: g.URL + "/revoke", API: g.URL}
}

var noFollow = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func googleCallback(t *testing.T, env *testutil.Env, q url.Values) url.Values {
	t.Helper()
	resp, err := noFollow.Get(env.URL("/calendar/google/callback?" + q.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	u, err := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusFound || err != nil || u.Path != "/calendar/calendars" {
		t.Fatalf("callback: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	return u.Query()
}

type accountList struct {
	Accounts    []api.CalendarAccount
	RedirectUri string
}

func TestGoogleCalendarAccount(t *testing.T) {
	env := testutil.New(t)
	c, _ := module.Lookup[contracts.Calendar](env.App.Deps.Registry, contracts.CalendarKey)
	m := c.(*calendar.Module)
	g := newFakeGoogle(t)
	m.UseGoogle(g.endpoints(), g.URL)

	body := map[string]any{"clientId": googleClient, "clientSecret": googleSecret}
	if code, _ := env.Do(http.MethodPost, "/calendar/accounts", body, nil); code != http.StatusForbidden {
		t.Fatalf("without elevation: %d", code)
	}
	env.Elevate()
	if code, _ := env.Do(http.MethodPost, "/calendar/accounts", map[string]any{"clientId": googleClient}, nil); code != http.StatusBadRequest {
		t.Fatalf("without a secret: %d", code)
	}
	var acc api.CalendarAccount
	env.MustDo(http.MethodPost, "/calendar/accounts", body, &acc)
	if acc.Authorized || !acc.SecretSet || acc.ClientId != googleClient {
		t.Fatalf("created: %+v", acc)
	}
	var list accountList
	env.MustDo(http.MethodGet, "/calendar/accounts", nil, &list)
	if len(list.Accounts) != 1 || !strings.HasSuffix(list.RedirectUri, "/api/v1/calendar/google/callback") {
		t.Fatalf("list: %+v", list)
	}
	_, raw := env.Do(http.MethodGet, "/calendar/accounts", nil, nil)
	if strings.Contains(string(raw), googleSecret) {
		t.Fatalf("secret leaked: %s", raw)
	}

	var start struct{ Url, RedirectUri string }
	env.MustDo(http.MethodGet, fmt.Sprintf("/calendar/accounts/%d/auth", acc.Id), nil, &start)
	authURL, _ := url.Parse(start.Url)
	q := authURL.Query()
	if q.Get("scope") != "https://www.googleapis.com/auth/calendar" || q.Get("redirect_uri") != start.RedirectUri || q.Get("access_type") != "offline" {
		t.Fatalf("auth url: %s", start.Url)
	}
	if got := googleCallback(t, env, url.Values{"state": {"forged"}, "code": {googleCode}}); got.Get("google") != "error" {
		t.Fatalf("forged state: %v", got)
	}
	if got := googleCallback(t, env, url.Values{"state": {q.Get("state")}, "code": {googleCode}}); got.Get("google") != "ok" || got.Get("account") != fmt.Sprint(acc.Id) {
		t.Fatalf("callback: %v", got)
	}
	if got := googleCallback(t, env, url.Values{"state": {q.Get("state")}, "code": {googleCode}}); got.Get("google") != "error" {
		t.Fatalf("state used twice: %v", got)
	}
	env.MustDo(http.MethodGet, "/calendar/accounts", nil, &list)
	if a := list.Accounts[0]; !a.Authorized || a.Email != googleEmail || a.AuthError != "" {
		t.Fatalf("authorized: %+v", a)
	}

	var remote struct{ Calendars []api.RemoteCalendar }
	env.MustDo(http.MethodGet, fmt.Sprintf("/calendar/accounts/%d/remote-calendars", acc.Id), nil, &remote)
	if len(remote.Calendars) != 2 || !remote.Calendars[0].Primary || remote.Calendars[0].ReadOnly || !remote.Calendars[1].ReadOnly || remote.Calendars[0].Added {
		t.Fatalf("remote calendars: %+v", remote.Calendars)
	}
	var added struct{ Calendars []api.Calendar }
	env.MustDo(http.MethodPost, fmt.Sprintf("/calendar/accounts/%d/remote-calendars", acc.Id),
		map[string]any{"ids": []string{googleEmail, holidayID, googleEmail}}, &added)
	if len(added.Calendars) != 2 {
		t.Fatalf("added: %+v", added.Calendars)
	}
	primary, holiday := added.Calendars[0], added.Calendars[1]
	if *primary.Writable == false || *holiday.Writable || primary.AccountId == nil || *primary.AccountId != acc.Id ||
		primary.Color != "#9fc6e7" || primary.Url != g.URL+googlePrimaryPath {
		t.Fatalf("calendars: %+v %+v", primary, holiday)
	}
	if code, _ := env.Do(http.MethodPost, fmt.Sprintf("/calendar/accounts/%d/remote-calendars", acc.Id), map[string]any{"ids": []string{"nope"}}, nil); code != http.StatusBadRequest {
		t.Fatalf("unknown calendar: %d", code)
	}
	if code, _ := env.Do(http.MethodPatch, fmt.Sprintf("/calendars/%d", primary.Id), map[string]any{"url": "https://evil.example/"}, nil); code != http.StatusBadRequest {
		t.Fatalf("changed the address of a Google calendar: %d", code)
	}

	primary = syncCalendar(t, env, primary.Id)
	if primary.LastError != "" || primary.EventCount != 1 {
		t.Fatalf("synced: %+v", primary)
	}
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, shanghai)
	events := listEvents(t, env, day, day.AddDate(0, 0, 1))
	if len(events) != 1 || events[0].Title != "谷歌上的会" || !*events[0].Writable {
		t.Fatalf("events: %+v", events)
	}

	// 改、新建、删除都写回 Google，带 If-Match
	var ev api.CalendarEvent
	env.MustDo(http.MethodPatch, fmt.Sprintf("/calendar/events/%d", events[0].EventId), map[string]any{"title": "改过的会"}, &ev)
	g.dav.mu.Lock()
	match := g.dav.lastMatch
	title, _ := g.dav.objects[googlePrimaryPath][0].Data.Events()[0].Props.Text("SUMMARY")
	g.dav.mu.Unlock()
	if match != `"7"` || title != "改过的会" {
		t.Fatalf("update writeback: %q %q", match, title)
	}
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, shanghai)
	env.MustDo(http.MethodPost, "/calendar/events", map[string]any{"calendarId": primary.Id, "title": "新会", "allDay": false,
		"start": at, "end": at.Add(time.Hour)}, &ev)
	g.dav.mu.Lock()
	n := len(g.dav.objects[googlePrimaryPath])
	g.dav.mu.Unlock()
	if n != 2 {
		t.Fatalf("create writeback: %d objects", n)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/calendar/events/%d", ev.EventId), nil, nil)
	if code, _ := env.Do(http.MethodPost, "/calendar/events", map[string]any{"calendarId": holiday.Id, "title": "x", "allDay": true,
		"startDate": "2026-10-20", "endDate": "2026-10-21"}, nil); code != http.StatusBadRequest {
		t.Fatalf("wrote to a read only calendar: %d", code)
	}

	// 授权过期：同步报错，账号上有提示，只通知一次
	g.mu.Lock()
	g.expired = true
	g.mu.Unlock()
	forgetTokens(t, env, acc.Id)
	primary = syncCalendar(t, env, primary.Id)
	_ = syncCalendar(t, env, holiday.Id)
	if !strings.Contains(primary.LastError, "Google 授权过期") || primary.EventCount != 1 {
		t.Fatalf("expired sync: %+v", primary)
	}
	env.MustDo(http.MethodGet, "/calendar/accounts", nil, &list)
	if list.Accounts[0].AuthError == "" {
		t.Fatalf("auth error missing: %+v", list.Accounts[0])
	}
	var notes int
	if err := env.App.Deps.DB.QueryRow(`SELECT count(*) FROM notifications WHERE kind = 'calendar.google_expired'`).Scan(&notes); err != nil || notes != 1 {
		t.Fatalf("notifications: %d %v", notes, err)
	}

	// 删账号：撤销令牌，删掉它的日历和日程
	env.MustDo(http.MethodDelete, fmt.Sprintf("/calendar/accounts/%d", acc.Id), nil, nil)
	g.mu.Lock()
	revoked := g.revoked
	g.mu.Unlock()
	if len(revoked) != 1 || revoked[0] != googleRefresh {
		t.Fatalf("revoked: %v", revoked)
	}
	var cals []api.Calendar
	env.MustDo(http.MethodGet, "/calendars", nil, &cals)
	if len(cals) != 0 || len(listEvents(t, env, day, day.AddDate(0, 0, 2))) != 0 {
		t.Fatalf("left after delete: %+v", cals)
	}
}

// forgetTokens drops the cached access token by changing the secret to the
// same value, which the module treats as a new client secret.
func forgetTokens(t *testing.T, env *testutil.Env, id int64) {
	t.Helper()
	env.MustDo(http.MethodPatch, fmt.Sprintf("/calendar/accounts/%d", id), map[string]any{"clientSecret": googleSecret}, nil)
}
