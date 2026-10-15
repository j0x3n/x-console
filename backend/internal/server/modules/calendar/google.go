package calendar

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// B215：Google 日历。授权照 storage 模块的 Google Drive 做，日历本身走
// Google 的 CalDAV，同步和写回用现有的 CalDAV 代码，只是认证换成 Bearer。

const (
	googleScope        = "https://www.googleapis.com/auth/calendar"
	googleCallbackPath = "/calendar/google/callback"
	googleDAVBase      = "https://apidata.googleusercontent.com"
	googleStateTTL     = 10 * time.Minute

	secClientSecret = "client_secret"
	secToken        = "refresh_token"
)

var (
	errGoogleExpired  = errors.New("Google 授权过期，请到日历管理里重新授权")
	errGoogleNoToken  = errors.New("Google 账号还没授权，请到日历管理里授权")
	errAccountMissing = httpx.NewError(http.StatusNotFound, "not_found", "找不到这个 Google 账号")
)

// googleState is one authorization started and not finished yet.
type googleState struct {
	account  int64
	expires  time.Time
	redirect string
	actor    string
}

type googleToken struct {
	access string
	expiry time.Time
}

// googleAuth holds what the Google part needs besides the module.
type googleAuth struct {
	endpoints files.GoogleEndpoints // zero means the real Google
	dav       string                // CalDAV host, "" means the real one

	mu     sync.Mutex
	states map[string]googleState
	tokens map[int64]googleToken
}

// UseGoogle points the module at fake Google servers. Tests only.
func (m *Module) UseGoogle(e files.GoogleEndpoints, dav string) {
	m.google.endpoints, m.google.dav = e, strings.TrimRight(dav, "/")
}

// PublicPaths implements module.PublicPather.
func (m *Module) PublicPaths() []string { return []string{googleCallbackPath} }

func accountSecretKey(id int64, name string) string {
	return fmt.Sprintf("calendar.account.%d.%s", id, name)
}

func (m *Module) accountSecret(ctx context.Context, id int64, name string) string {
	var v string
	if err := m.d.Settings.Get(ctx, accountSecretKey(id, name), &v); err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("calendar: account secret cannot be read", "id", id, "name", name, "error", err)
	}
	return v
}

func (m *Module) setAccountSecret(ctx context.Context, id int64, name, v string) error {
	if v == "" {
		return m.d.Settings.Delete(ctx, accountSecretKey(id, name))
	}
	return m.d.Settings.SetSecret(ctx, accountSecretKey(id, name), v)
}

func (m *Module) davBase() string {
	if m.google.dav != "" {
		return m.google.dav
	}
	return googleDAVBase
}

// googleCalendarURL is the CalDAV collection of one Google calendar.
func (m *Module) googleCalendarURL(remoteID string) string {
	return m.davBase() + "/caldav/v2/" + url.PathEscape(remoteID) + "/events/"
}

func redirectURI(public string, r *http.Request) string {
	base := strings.TrimRight(public, "/")
	if base == "" && r != nil {
		proto := "http"
		if r.TLS != nil {
			proto = "https"
		}
		if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded == "http" || forwarded == "https" {
			proto = forwarded
		}
		base = proto + "://" + r.Host
	}
	return base + "/api/v1" + googleCallbackPath
}

// ---- access tokens ----

// googleAccess returns a fresh access token of an account, refreshing it
// when needed. A refresh token Google no longer takes marks the account as
// expired and tells the user once a day.
func (m *Module) googleAccess(ctx context.Context, id int64) (string, error) {
	m.google.mu.Lock()
	tok, ok := m.google.tokens[id]
	m.google.mu.Unlock()
	if ok && time.Now().Before(tok.expiry) {
		return tok.access, nil
	}
	acc, err := m.q.GetCalendarAccount(ctx, id)
	if err != nil {
		return "", errAccountMissing
	}
	refresh := m.accountSecret(ctx, id, secToken)
	if refresh == "" {
		return "", errGoogleNoToken
	}
	access, expiry, err := files.GoogleRefresh(ctx, m.google.endpoints, acc.ClientID, m.accountSecret(ctx, id, secClientSecret), refresh)
	if errors.Is(err, files.ErrGDriveAuth) {
		m.markExpired(ctx, acc)
		return "", errGoogleExpired
	}
	if err != nil {
		return "", err
	}
	m.google.mu.Lock()
	m.google.tokens[id] = googleToken{access: access, expiry: expiry}
	m.google.mu.Unlock()
	return access, nil
}

func (m *Module) forgetToken(id int64) {
	m.google.mu.Lock()
	delete(m.google.tokens, id)
	m.google.mu.Unlock()
}

func (m *Module) markExpired(ctx context.Context, acc db.CalendarAccount) {
	now := time.Now().UTC()
	notified := acc.AuthNotifiedAt
	tell := notified == nil || now.Sub(*notified) >= 24*time.Hour
	if tell {
		notified = &now
	}
	if _, err := m.q.SetCalendarAccountAuthError(ctx, db.SetCalendarAccountAuthErrorParams{
		AuthError: errGoogleExpired.Error(), AuthNotifiedAt: notified, UpdatedAt: now, ID: acc.ID,
	}); err != nil {
		m.d.Log.Warn("calendar: mark google account expired", "id", acc.ID, "err", err)
	}
	if !tell || m.d.Notify == nil {
		return
	}
	name := acc.Email
	if name == "" {
		name = "Google 账号"
	}
	if _, err := m.d.Notify.Send(ctx, notify.Notification{
		Kind: "calendar.google_expired", Title: "Google 日历授权过期",
		Body: name + " 的授权失效了，日程停止同步。到日历管理里点“重新授权”。",
		Link: "/calendar/calendars", Source: "calendar", Scope: "account:" + strconv.FormatInt(acc.ID, 10),
	}); err != nil {
		m.d.Log.Warn("calendar: notify google expired", "err", err)
	}
}

// bearerClient signs CalDAV requests with the account's access token.
type bearerClient struct {
	m       *Module
	account int64
}

func (b bearerClient) Do(r *http.Request) (*http.Response, error) {
	tok, err := b.m.googleAccess(r.Context(), b.account)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", "Bearer "+tok)
	resp, err := b.m.http.Do(r)
	if err == nil && resp.StatusCode == http.StatusForbidden {
		// 最常见的原因是没在 Google Cloud 里启用 CalDAV API
		resp.Body.Close()
		return nil, errors.New("Google 拒绝访问（403）。确认 Google Cloud 项目里启用了 CalDAV API")
	}
	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		b.m.forgetToken(b.account)
	}
	return resp, err
}

// ---- Calendar API: the list of calendars ----

type googleCalendar struct {
	ID              string `json:"id"`
	Summary         string `json:"summary"`
	SummaryOverride string `json:"summaryOverride"`
	BackgroundColor string `json:"backgroundColor"`
	AccessRole      string `json:"accessRole"`
	Primary         bool   `json:"primary"`
}

func (c googleCalendar) name() string {
	if c.SummaryOverride != "" {
		return c.SummaryOverride
	}
	if c.Summary != "" {
		return c.Summary
	}
	return c.ID
}

func (c googleCalendar) readOnly() bool { return c.AccessRole != "owner" && c.AccessRole != "writer" }

func (m *Module) googleCalendars(ctx context.Context, id int64) ([]googleCalendar, error) {
	e := m.google.endpoints.WithDefaults()
	var out []googleCalendar
	page := ""
	for range 20 {
		tok, err := m.googleAccess(ctx, id)
		if err != nil {
			return nil, err
		}
		q := url.Values{"maxResults": {"250"}}
		if page != "" {
			q.Set("pageToken", page)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.API+"/calendar/v3/users/me/calendarList?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := e.Client.Do(req)
		if err != nil {
			return nil, cleanErr(err)
		}
		var body struct {
			Items         []googleCalendar `json:"items"`
			NextPageToken string           `json:"nextPageToken"`
			Error         struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body)
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusForbidden:
			return nil, httpx.NewError(http.StatusBadGateway, "google_failed", "Google 拒绝访问。确认 Google Cloud 项目里启用了 Google Calendar API 和 CalDAV API")
		case resp.StatusCode == http.StatusUnauthorized:
			m.forgetToken(id)
			return nil, httpx.NewError(http.StatusBadGateway, "google_failed", "Google 不认这个授权，请重新授权")
		case resp.StatusCode/100 != 2:
			msg := body.Error.Message
			if msg == "" {
				msg = "HTTP " + strconv.Itoa(resp.StatusCode)
			}
			return nil, httpx.NewError(http.StatusBadGateway, "google_failed", "读取 Google 日历列表失败："+msg)
		case err != nil:
			return nil, httpx.NewError(http.StatusBadGateway, "google_failed", "Google 返回的日历列表读不懂")
		}
		out = append(out, body.Items...)
		if page = body.NextPageToken; page == "" {
			return out, nil
		}
	}
	return out, nil
}

// ---- conversion ----

func (m *Module) accountAPI(ctx context.Context, a db.CalendarAccount) api.CalendarAccount {
	cals, _ := m.q.ListAccountCalendars(ctx, &a.ID)
	return api.CalendarAccount{
		Id: a.ID, Provider: api.CalendarAccountProvider(a.Provider), Email: a.Email, ClientId: a.ClientID,
		SecretSet:  m.accountSecret(ctx, a.ID, secClientSecret) != "",
		Authorized: m.accountSecret(ctx, a.ID, secToken) != "", AuthError: a.AuthError,
		CalendarCount: len(cals), CreatedAt: a.CreatedAt,
	}
}

func (m *Module) loadAccount(ctx context.Context, id int64) (db.CalendarAccount, error) {
	a, err := m.q.GetCalendarAccount(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return a, errAccountMissing
	}
	return a, err
}

// ---- handlers ----

// ListCalendarAccounts is GET /calendar/accounts.
func (m *Module) ListCalendarAccounts(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListCalendarAccounts(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.CalendarAccount, 0, len(rows))
	for _, a := range rows {
		out = append(out, m.accountAPI(r.Context(), a))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"accounts": out, "redirectUri": redirectURI(m.d.Config.PublicURL, r)})
}

// CreateCalendarAccount is POST /calendar/accounts.
func (m *Module) CreateCalendarAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.CalendarAccountInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	clientID, secret := strings.TrimSpace(deref(in.ClientId)), strings.TrimSpace(deref(in.ClientSecret))
	if clientID == "" || secret == "" {
		httpx.Fail(w, r, httpx.Invalid("客户端 ID 和密钥都要填"))
		return
	}
	now := time.Now().UTC()
	row, err := m.q.CreateCalendarAccount(ctx, db.CreateCalendarAccountParams{ClientID: clientID, CreatedAt: now, UpdatedAt: now})
	if err == nil {
		err = m.setAccountSecret(ctx, row.ID, secClientSecret, secret)
	}
	m.d.Audit.Record(ctx, "calendar.account.create", clientID, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, m.accountAPI(ctx, row))
}

// UpdateCalendarAccount is PATCH /calendar/accounts/{id}. A new client ID
// drops the authorization: the old refresh token belongs to the old client.
func (m *Module) UpdateCalendarAccount(w http.ResponseWriter, r *http.Request, id api.AccountId) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.CalendarAccountInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.loadAccount(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.ClientId != nil {
		clientID := strings.TrimSpace(*in.ClientId)
		if clientID == "" {
			httpx.Fail(w, r, httpx.Invalid("客户端 ID 不能为空"))
			return
		}
		if clientID != row.ClientID {
			if row, err = m.q.UpdateCalendarAccountClient(ctx, db.UpdateCalendarAccountClientParams{ClientID: clientID, UpdatedAt: time.Now().UTC(), ID: id}); err == nil {
				err = m.setAccountSecret(ctx, id, secToken, "")
				m.forgetToken(id)
			}
		}
	}
	if secret := strings.TrimSpace(deref(in.ClientSecret)); err == nil && secret != "" {
		err = m.setAccountSecret(ctx, id, secClientSecret, secret)
		m.forgetToken(id)
	}
	m.d.Audit.Record(ctx, "calendar.account.update", row.Email, map[string]any{"id": id}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m.accountAPI(ctx, row))
}

// DeleteCalendarAccount is DELETE /calendar/accounts/{id}.
func (m *Module) DeleteCalendarAccount(w http.ResponseWriter, r *http.Request, id api.AccountId) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.loadAccount(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if token := m.accountSecret(ctx, id, secToken); token != "" {
		if err := files.GoogleRevoke(ctx, m.google.endpoints, token); err != nil {
			// 撤销失败不拦着删除，用户可以在 Google 账号里自己移除
			m.d.Log.Warn("calendar: revoke google token", "id", id, "err", err)
		}
	}
	m.syncMu.Lock()
	_, err = m.q.DeleteCalendarAccount(ctx, id)
	m.syncMu.Unlock()
	if err == nil {
		_ = m.setAccountSecret(ctx, id, secToken, "")
		_ = m.setAccountSecret(ctx, id, secClientSecret, "")
		m.forgetToken(id)
	}
	m.d.Audit.Record(ctx, "calendar.account.delete", row.Email, map[string]any{"id": id}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("calendar.deleted", map[string]any{"accountId": id})
	httpx.NoContent(w)
}

// StartCalendarAccountAuth is GET /calendar/accounts/{id}/auth.
func (m *Module) StartCalendarAccountAuth(w http.ResponseWriter, r *http.Request, id api.AccountId) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.loadAccount(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if row.ClientID == "" || m.accountSecret(ctx, id, secClientSecret) == "" {
		httpx.Fail(w, r, httpx.Invalid("先填好客户端 ID 和密钥并保存，再授权"))
		return
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	state := hex.EncodeToString(raw)
	redirect := redirectURI(m.d.Config.PublicURL, r)
	now := time.Now()
	m.google.mu.Lock()
	for k, v := range m.google.states {
		if now.After(v.expires) {
			delete(m.google.states, k)
		}
	}
	m.google.states[state] = googleState{account: id, expires: now.Add(googleStateTTL), redirect: redirect, actor: audit.Actor(ctx)}
	m.google.mu.Unlock()
	u := files.GoogleAuthURLFor(m.google.endpoints, row.ClientID, redirect, state, googleScope)
	httpx.JSON(w, http.StatusOK, map[string]string{"url": u, "redirectUri": redirect})
}

func (m *Module) takeState(state string) (googleState, bool) {
	m.google.mu.Lock()
	defer m.google.mu.Unlock()
	st, ok := m.google.states[state]
	delete(m.google.states, state)
	if !ok || time.Now().After(st.expires) {
		return googleState{}, false
	}
	return st, true
}

// CalendarGoogleCallback is GET /calendar/google/callback.
func (m *Module) CalendarGoogleCallback(w http.ResponseWriter, r *http.Request, params api.CalendarGoogleCallbackParams) {
	target := m.finishGoogleAuth(r.Context(), deref(params.State), deref(params.Code), deref(params.Error))
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

func (m *Module) finishGoogleAuth(ctx context.Context, state, code, googleError string) string {
	back := func(account int64, err error) string {
		if err == nil {
			return "/calendar/calendars?" + url.Values{"google": {"ok"}, "account": {strconv.FormatInt(account, 10)}}.Encode()
		}
		return "/calendar/calendars?" + url.Values{"google": {"error"}, "message": {err.Error()}}.Encode()
	}
	st, ok := m.takeState(state)
	if !ok {
		return back(0, errors.New("授权链接已失效，请回到日历管理重新点授权"))
	}
	ctx = audit.WithActor(ctx, st.actor)
	switch {
	case googleError == "access_denied":
		return back(0, errors.New("你在 Google 页面取消了授权"))
	case googleError != "":
		return back(0, errors.New("Google 授权失败："+googleError))
	case code == "":
		return back(0, errors.New("Google 没有返回授权码"))
	}
	email, err := m.authorize(ctx, st.account, code, st.redirect)
	m.d.Audit.Record(ctx, "calendar.account.authorize", email, map[string]any{"id": st.account}, err)
	if err == nil {
		m.d.Bus.Publish("calendar.updated", map[string]any{"accountId": st.account})
		for _, c := range m.accountCalendars(ctx, st.account) {
			if c.Enabled == 1 {
				m.syncLater(c.ID)
			}
		}
	}
	return back(st.account, err)
}

// authorize trades the code for a refresh token and reads the account's
// address from its primary calendar.
func (m *Module) authorize(ctx context.Context, id int64, code, redirect string) (string, error) {
	row, err := m.q.GetCalendarAccount(ctx, id)
	if err != nil {
		return "", errors.New("这个 Google 账号已经删了")
	}
	secret := m.accountSecret(ctx, id, secClientSecret)
	grant, err := files.GoogleExchange(ctx, m.google.endpoints, row.ClientID, secret, code, redirect)
	if err != nil {
		return "", err
	}
	if err := m.setAccountSecret(ctx, id, secToken, grant.RefreshToken); err != nil {
		return "", err
	}
	m.forgetToken(id)
	email := row.Email
	if cals, err := m.googleCalendars(ctx, id); err == nil {
		for _, c := range cals {
			if c.Primary {
				email = c.ID
			}
		}
	}
	_, err = m.q.SetCalendarAccountAuthorized(ctx, db.SetCalendarAccountAuthorizedParams{Email: email, UpdatedAt: time.Now().UTC(), ID: id})
	return email, err
}

func (m *Module) accountCalendars(ctx context.Context, id int64) []db.Calendar {
	cals, _ := m.q.ListAccountCalendars(ctx, &id)
	return cals
}

// ListRemoteCalendars is GET /calendar/accounts/{id}/remote-calendars.
func (m *Module) ListRemoteCalendars(w http.ResponseWriter, r *http.Request, id api.AccountId) {
	ctx := r.Context()
	if _, err := m.loadAccount(ctx, id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cals, err := m.googleCalendars(ctx, id)
	if err != nil {
		httpx.Fail(w, r, googleFail(err))
		return
	}
	added := map[string]bool{}
	for _, c := range m.accountCalendars(ctx, id) {
		added[c.RemoteID] = true
	}
	out := make([]api.RemoteCalendar, 0, len(cals))
	for _, c := range cals {
		out = append(out, api.RemoteCalendar{Id: c.ID, Name: c.name(), Color: strings.ToLower(c.BackgroundColor),
			ReadOnly: c.readOnly(), Primary: c.Primary, Added: added[c.ID]})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"calendars": out})
}

// googleFail turns token errors into answers the page can show.
func googleFail(err error) error {
	if errors.Is(err, errGoogleExpired) || errors.Is(err, errGoogleNoToken) {
		return httpx.NewError(http.StatusPreconditionFailed, "integration_not_configured", err.Error())
	}
	return err
}

// AddRemoteCalendars is POST /calendar/accounts/{id}/remote-calendars.
func (m *Module) AddRemoteCalendars(w http.ResponseWriter, r *http.Request, id api.AccountId) {
	ctx := r.Context()
	var in api.AddRemoteCalendarsJSONRequestBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(in.Ids) == 0 || len(in.Ids) > 50 {
		httpx.Fail(w, r, httpx.Invalid("一次选 1 到 50 个日历"))
		return
	}
	acc, err := m.loadAccount(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cals, err := m.googleCalendars(ctx, id)
	if err != nil {
		httpx.Fail(w, r, googleFail(err))
		return
	}
	byID := map[string]googleCalendar{}
	for _, c := range cals {
		byID[c.ID] = c
	}
	added := map[string]bool{}
	for _, c := range m.accountCalendars(ctx, id) {
		added[c.RemoteID] = true
	}
	out := []api.Calendar{}
	for _, rid := range in.Ids {
		c, ok := byID[rid]
		if !ok {
			httpx.Fail(w, r, httpx.Invalid("这个 Google 账号下没有日历 "+rid))
			return
		}
		if added[rid] {
			continue
		}
		added[rid] = true
		name := []rune(c.name())
		if len(name) > 100 {
			name = name[:100]
		}
		color, _ := cleanColor(c.BackgroundColor)
		row, err := m.q.CreateAccountCalendar(ctx, db.CreateAccountCalendarParams{
			Name: string(name), Url: m.googleCalendarURL(rid), Color: color, CreatedAt: time.Now().UTC(),
			AccountID: &id, RemoteID: rid, ReadOnly: boolInt(c.readOnly()),
		})
		m.d.Audit.Record(ctx, "calendar.create", string(name), map[string]any{"kind": "google", "account": acc.Email}, err)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		cal := m.toAPI(ctx, row)
		m.d.Bus.Publish("calendar.created", cal)
		m.syncLater(row.ID)
		out = append(out, cal)
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"calendars": out})
}
