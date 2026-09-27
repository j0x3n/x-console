package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/db"
)

// ---- conversion and validation ----

func (m *Module) toAPI(ctx context.Context, c db.Calendar) api.Calendar {
	count, _ := m.q.CountCalendarEvents(ctx, c.ID)
	return api.Calendar{
		Id: c.ID, Name: c.Name, Kind: api.CalendarKind(c.Kind), Url: c.Url, Username: c.Username,
		HasPassword: c.Secret != "", Color: c.Color, Enabled: c.Enabled == 1, LastSyncedAt: c.LastSyncedAt,
		LastError: c.LastError, EventCount: int(count), CreatedAt: c.CreatedAt,
	}
}

func occurrenceToAPI(o occurrence) api.CalendarEvent {
	ev := api.CalendarEvent{
		Id: eventKey(o), EventId: o.row.ID, CalendarId: o.row.CalendarID, Calendar: o.row.CalendarName,
		Color: o.row.CalendarColor, Title: o.row.Title, Start: o.start, End: o.end, AllDay: o.row.AllDay == 1,
		Location: o.row.Location, Description: o.row.Description, Recurring: o.recurring,
	}
	if ev.AllDay {
		s, e := o.start.Format(time.DateOnly), o.end.Format(time.DateOnly)
		ev.StartDate, ev.EndDate = &s, &e
	}
	return ev
}

func eventKey(o occurrence) string {
	return strings.Join([]string{itoa(o.row.ID), itoa(o.start.Unix())}, "-")
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func cleanName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", httpx.Invalid("名称不能为空")
	}
	if len([]rune(s)) > 100 {
		return "", httpx.Invalid("名称太长了")
	}
	return s, nil
}

func cleanColor(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s != "" && !colorPattern.MatchString(s) {
		return "", httpx.Invalid("颜色要写成 #rrggbb")
	}
	return strings.ToLower(s), nil
}

func cleanURL(kind, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if raw == "" || err != nil || u.Host == "" {
		return "", httpx.Invalid("地址格式不对")
	}
	switch u.Scheme {
	case "http", "https":
	case "webcal", "webcals":
		if kind != kindICS {
			return "", httpx.Invalid("CalDAV 地址要以 http(s):// 开头")
		}
	default:
		return "", httpx.Invalid("地址要以 http(s):// 或 webcal:// 开头")
	}
	return raw, nil
}

func (m *Module) seal(password string) (string, error) {
	if password == "" {
		return "", nil
	}
	return m.d.Secrets.Seal(password)
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// ---- business methods ----

func (m *Module) create(ctx context.Context, in api.CalendarInput) (db.Calendar, error) {
	if in.Kind == "local" {
		return db.Calendar{}, httpx.NewError(http.StatusNotImplemented, "not_ready", "本地日历还没上线")
	}
	if in.Kind != kindICS && in.Kind != kindCalDAV {
		return db.Calendar{}, httpx.Invalid("类型只能是 ics 或 caldav")
	}
	name, err := cleanName(in.Name)
	if err != nil {
		return db.Calendar{}, err
	}
	u, err := cleanURL(string(in.Kind), in.Url)
	if err != nil {
		return db.Calendar{}, err
	}
	color, err := cleanColor(deref(in.Color))
	if err != nil {
		return db.Calendar{}, err
	}
	secret, err := m.seal(deref(in.Password))
	if err != nil {
		return db.Calendar{}, err
	}
	enabled := in.Enabled == nil || *in.Enabled
	row, err := m.q.CreateCalendar(ctx, db.CreateCalendarParams{
		Name: name, Kind: string(in.Kind), Url: u, Username: strings.TrimSpace(deref(in.Username)), Secret: secret,
		Color: color, Enabled: boolInt(enabled), CreatedAt: time.Now().UTC(),
	})
	m.d.Audit.Record(ctx, "calendar.create", name, map[string]any{"kind": in.Kind}, err)
	return row, err
}

// update applies a patch. resync reports whether the source changed.
func (m *Module) update(ctx context.Context, id int64, p api.CalendarPatch) (row db.Calendar, resync bool, err error) {
	cur, err := m.q.GetCalendar(ctx, id)
	if err != nil {
		return cur, false, notFound(err)
	}
	next := db.UpdateCalendarParams{
		ID: id, Name: cur.Name, Url: cur.Url, Username: cur.Username, Secret: cur.Secret, Color: cur.Color, Enabled: cur.Enabled,
	}
	if p.Name != nil {
		if next.Name, err = cleanName(*p.Name); err != nil {
			return cur, false, err
		}
	}
	if p.Url != nil {
		if next.Url, err = cleanURL(cur.Kind, *p.Url); err != nil {
			return cur, false, err
		}
	}
	if p.Username != nil {
		next.Username = strings.TrimSpace(*p.Username)
	}
	if p.Password != nil {
		if next.Secret, err = m.seal(*p.Password); err != nil {
			return cur, false, err
		}
	}
	if p.Color != nil {
		if next.Color, err = cleanColor(*p.Color); err != nil {
			return cur, false, err
		}
	}
	if p.Enabled != nil {
		next.Enabled = boolInt(*p.Enabled)
	}
	row, err = m.q.UpdateCalendar(ctx, next)
	m.d.Audit.Record(ctx, "calendar.update", row.Name, map[string]any{"id": id}, err)
	if err != nil {
		return row, false, err
	}
	resync = next.Enabled == 1 && (next.Url != cur.Url || next.Username != cur.Username || p.Password != nil || cur.Enabled == 0)
	return row, resync, nil
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// ---- HTTP handlers ----

func (m *Module) ListCalendars(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListCalendars(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.Calendar, 0, len(rows))
	for _, c := range rows {
		out = append(out, m.toAPI(r.Context(), c))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateCalendar(w http.ResponseWriter, r *http.Request) {
	var body api.CreateCalendarJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.create(r.Context(), body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := m.toAPI(r.Context(), row)
	m.d.Bus.Publish("calendar.created", out)
	if row.Enabled == 1 {
		m.syncLater(row.ID)
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) GetCalendar(w http.ResponseWriter, r *http.Request, id api.CalendarId) {
	row, err := m.q.GetCalendar(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, notFound(err))
		return
	}
	httpx.JSON(w, http.StatusOK, m.toAPI(r.Context(), row))
}

func (m *Module) UpdateCalendar(w http.ResponseWriter, r *http.Request, id api.CalendarId) {
	var body api.UpdateCalendarJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, resync, err := m.update(r.Context(), id, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := m.toAPI(r.Context(), row)
	m.d.Bus.Publish("calendar.updated", out)
	if resync {
		m.syncLater(row.ID)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) DeleteCalendar(w http.ResponseWriter, r *http.Request, id api.CalendarId) {
	n, err := m.q.DeleteCalendar(r.Context(), id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(r.Context(), "calendar.delete", itoa(id), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("calendar.deleted", map[string]any{"id": id})
	httpx.NoContent(w)
}

func (m *Module) SyncCalendar(w http.ResponseWriter, r *http.Request, id api.CalendarId) {
	row, err := m.syncOne(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m.toAPI(r.Context(), row))
}

func (m *Module) ListCalendarEvents(w http.ResponseWriter, r *http.Request, params api.ListCalendarEventsParams) {
	out, err := m.events(r.Context(), params.From, params.To)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) events(ctx context.Context, from, to time.Time) ([]api.CalendarEvent, error) {
	occ, err := m.occurrences(ctx, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]api.CalendarEvent, 0, len(occ))
	for _, o := range occ {
		out = append(out, occurrenceToAPI(o))
	}
	return out, nil
}

// ---- actions ----

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "calendar.events",
		Title: "查看日程",
		Description: "List calendar events between `from` and `to` (RFC 3339). Both default to today in the user's time zone (" +
			m.d.Config.Location.String() + "). Repeating events are expanded. The range is at most 100 days.",
		Input:  actions.Schema(`{"type":"object","properties":{"from":{"type":"string","format":"date-time"},"to":{"type":"string","format":"date-time"}},"additionalProperties":false}`),
		Effect: actions.Read,
		Run:    m.actionEvents,
	})
}

func (m *Module) actionEvents(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		From *time.Time `json:"from"`
		To   *time.Time `json:"to"`
	}
	if len(raw) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			return nil, httpx.Invalid("参数格式不正确: " + err.Error())
		}
	}
	now := time.Now().In(m.d.Config.Location)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, m.d.Config.Location)
	to := from.AddDate(0, 0, 1)
	if in.From != nil {
		from = *in.From
	}
	if in.To != nil {
		to = *in.To
	} else if in.From != nil {
		to = from.AddDate(0, 0, 1)
	}
	return m.events(ctx, from, to)
}

// 新建和修改事件还没做完（本地日历、CalDAV 写回），先回 501，前端显示“还没上线”。
var errEventsNotReady = httpx.NewError(http.StatusNotImplemented, "not_ready", "新建和修改日程还没上线")

func (m *Module) CreateCalendarEvent(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, errEventsNotReady)
}

func (m *Module) UpdateCalendarEvent(w http.ResponseWriter, r *http.Request, _ int64) {
	httpx.Fail(w, r, errEventsNotReady)
}

func (m *Module) DeleteCalendarEvent(w http.ResponseWriter, r *http.Request, _ int64) {
	httpx.Fail(w, r, errEventsNotReady)
}
