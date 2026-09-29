// Package brief is the daily brief of M11: every morning it collects the
// weather, today's calendar, due issues, reminders, last night's server
// alerts and habits into one Markdown message, stores it and sends it with
// notify.Send (kind brief.daily). It also serves GET /weather for the home
// page.
//
// Every section comes from another module through contracts. A module that
// is missing or fails only removes its own section.
package brief

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// ServiceKey is where the module registers itself (tests, the home page).
const ServiceKey = "brief.brief"

// Module implements api.ServerInterface.
type Module struct {
	d    *module.Deps
	q    *db.Queries
	http *http.Client

	sendMu sync.Mutex // one scheduled send at a time

	weatherMu    sync.Mutex
	weatherCache map[string]cachedWeather
	now          func() time.Time // 测试里换成假时钟
	geoBase      string           // Open-Meteo 地名接口，测试里换成假服务
	osmBase      string           // OpenStreetMap 地名接口，Open-Meteo 查不到时用
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), http: &http.Client{Timeout: 15 * time.Second}, weatherCache: map[string]cachedWeather{}, now: time.Now, geoBase: defaultGeoBase, osmBase: defaultOSMBase}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "brief" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start checks every minute whether the brief is due. The send time is a
// setting, so a fixed cron entry would need rebuilding on every change.
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("brief.daily", time.Minute, func(ctx context.Context) error {
		return m.tick(ctx, time.Now())
	})
	m.d.Scheduler.Every("brief.rain", 30*time.Minute, func(ctx context.Context) error {
		return m.checkRain(ctx, time.Now())
	})
	return nil
}

func (m *Module) polisher() (Polisher, bool) {
	return module.Lookup[Polisher](m.d.Registry, PolisherKey)
}

// ---- scheduling ----

// sendWindow is how late a brief may still go out, for example after the
// server was down at the planned time.
const sendWindow = 3 * time.Hour

// shouldSend decides whether the scheduled brief is due at now. It is pure;
// tests drive it with any clock.
func shouldSend(enabled bool, at string, now time.Time, loc *time.Location, sentToday bool) bool {
	if !enabled || sentToday {
		return false
	}
	planned, ok := plannedAt(at, now, loc)
	if !ok {
		return false
	}
	return !now.Before(planned) && now.Before(planned.Add(sendWindow))
}

// plannedAt is today's send time in loc.
func plannedAt(at string, now time.Time, loc *time.Location) (time.Time, bool) {
	t, err := time.Parse("15:04", at)
	if err != nil {
		return time.Time{}, false
	}
	l := now.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), t.Hour(), t.Minute(), 0, 0, loc), true
}

// nextRun is the next scheduled send after now.
func nextRun(at string, now time.Time, loc *time.Location, sentToday bool) (time.Time, bool) {
	planned, ok := plannedAt(at, now, loc)
	if !ok {
		return time.Time{}, false
	}
	if sentToday || !now.Before(planned.Add(sendWindow)) {
		l := planned.AddDate(0, 0, 1)
		return time.Date(l.Year(), l.Month(), l.Day(), planned.Hour(), planned.Minute(), 0, 0, loc), true
	}
	if now.After(planned) {
		return now, true
	}
	return planned, true
}

// sentOn reports whether the brief for now's day was already sent.
func (m *Module) sentOn(ctx context.Context, now time.Time) bool {
	row, err := m.q.GetBriefByDate(ctx, now.In(m.d.Config.Location).Format(time.DateOnly))
	return err == nil && row.SentAt != nil
}

// tick sends the brief when it is due. The scheduler calls it every minute.
func (m *Module) tick(ctx context.Context, now time.Time) error {
	m.sendMu.Lock()
	defer m.sendMu.Unlock()
	cfg, err := m.load(ctx)
	if err != nil {
		return err
	}
	if !shouldSend(cfg.Enabled, cfg.Time, now, m.d.Config.Location, m.sentOn(ctx, now)) {
		return nil
	}
	_, err = m.generateAndSend(ctx, cfg, now)
	return err
}

// generateAndSend builds today's brief, stores it and sends it.
func (m *Module) generateAndSend(ctx context.Context, cfg config, now time.Time) (db.Brief, error) {
	res := m.generate(ctx, cfg, m.d.Registry, now)
	raw, _ := json.Marshal(res.sections)
	row, err := m.q.UpsertBrief(ctx, db.UpsertBriefParams{Date: res.date, Content: res.content, Sections: string(raw), CreatedAt: now.UTC()})
	if err != nil {
		return row, err
	}
	if err := m.send(ctx, cfg, row, now); err != nil {
		return row, err
	}
	sent := now.UTC()
	row, err = m.q.MarkBriefSent(ctx, db.MarkBriefSentParams{SentAt: &sent, ID: row.ID})
	if err != nil {
		return row, err
	}
	m.d.Bus.Publish("brief.sent", toAPI(row))
	return row, nil
}

// send delivers a stored brief. With no channels chosen, the notification
// routing rules decide. With channels chosen, the notification is sent with
// low priority, which the default rule keeps in the app, and then delivered
// to exactly those channels.
func (m *Module) send(ctx context.Context, cfg config, row db.Brief, now time.Time) error {
	title := dateTitle(now.In(m.d.Config.Location))
	n := notify.Notification{
		Kind: "brief.daily", Title: title, Body: plainText(row.Content), Link: "/calendar/briefs?date=" + row.Date,
		Source: "brief", Data: map[string]any{"date": row.Date},
	}
	if len(cfg.Channels) > 0 {
		n.Priority = notify.PriorityLow
	}
	stored, err := m.d.Notify.Send(ctx, n)
	if err != nil {
		return err
	}
	for _, name := range cfg.Channels {
		ch, ok := m.d.Notify.Channel(name)
		if !ok {
			continue
		}
		sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := ch.Send(sendCtx, stored)
		cancel()
		if err != nil {
			m.d.Log.Warn("brief delivery failed", "channel", name, "err", err)
		}
	}
	return nil
}

// ---- conversion ----

func toAPI(r db.Brief) api.Brief {
	var sections []api.BriefSection
	_ = json.Unmarshal([]byte(r.Sections), &sections)
	if sections == nil {
		sections = []api.BriefSection{}
	}
	id := r.ID
	return api.Brief{Id: &id, Date: r.Date, Content: r.Content, Sections: sections, CreatedAt: r.CreatedAt, SentAt: r.SentAt}
}

func previewAPI(res result, now time.Time) api.Brief {
	return api.Brief{Date: res.date, Content: res.content, Sections: res.sections, CreatedAt: now.UTC()}
}

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ---- HTTP handlers ----

func (m *Module) ListBriefs(w http.ResponseWriter, r *http.Request, params api.ListBriefsParams) {
	before, err := httpx.DecodeIDCursor(params.Cursor)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	limit := httpx.Limit(params.Limit)
	rows, err := m.q.ListBriefs(r.Context(), db.ListBriefsParams{Before: before, Lim: limit})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := struct {
		Items      []api.Brief `json:"items"`
		NextCursor *string     `json:"nextCursor,omitempty"`
	}{Items: make([]api.Brief, 0, len(rows))}
	for _, row := range rows {
		out.Items = append(out.Items, toAPI(row))
	}
	if int64(len(rows)) == limit {
		c := httpx.EncodeIDCursor(rows[len(rows)-1].ID)
		out.NextCursor = &c
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) GetBrief(w http.ResponseWriter, r *http.Request, date string) {
	if !datePattern.MatchString(date) {
		httpx.Fail(w, r, httpx.Invalid("日期要写成 YYYY-MM-DD"))
		return
	}
	row, err := m.q.GetBriefByDate(r.Context(), date)
	if errors.Is(err, sql.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPI(row))
}

func (m *Module) GenerateBrief(w http.ResponseWriter, r *http.Request) {
	var body api.GenerateBriefJSONRequestBody
	if r.ContentLength != 0 {
		if err := httpx.Decode(r, &body); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	ctx, now := r.Context(), time.Now()
	cfg, err := m.load(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Send == nil || !*body.Send {
		httpx.JSON(w, http.StatusOK, previewAPI(m.generate(ctx, cfg, m.d.Registry, now), now))
		return
	}
	m.sendMu.Lock()
	row, err := m.generateAndSend(ctx, cfg, now)
	m.sendMu.Unlock()
	m.d.Audit.Record(ctx, "brief.send", row.Date, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPI(row))
}

func (m *Module) GetBriefSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := m.load(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m.view(r.Context(), cfg, time.Now()))
}

func (m *Module) PutBriefSettings(w http.ResponseWriter, r *http.Request) {
	var body api.PutBriefSettingsJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cfg, err := m.validate(body)
	if err == nil {
		err = m.save(r.Context(), cfg)
	}
	m.d.Audit.Record(r.Context(), "brief.settings", "", map[string]any{"enabled": cfg.Enabled, "time": cfg.Time}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := m.view(r.Context(), cfg, time.Now())
	m.d.Bus.Publish("brief.settings_updated", out)
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) GetWeather(w http.ResponseWriter, r *http.Request, params api.GetWeatherParams) {
	cfg, err := m.load(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	loc := cfg.Location
	if params.Lat != nil || params.Lon != nil {
		if params.Lat == nil || params.Lon == nil || *params.Lat < -90 || *params.Lat > 90 || *params.Lon < -180 || *params.Lon > 180 {
			httpx.Fail(w, r, httpx.Invalid("经纬度要一起传，并且在有效范围内"))
			return
		}
		loc = &api.BriefLocation{Lat: *params.Lat, Lon: *params.Lon}
	}
	if loc == nil {
		httpx.Fail(w, r, httpx.ErrIntegrationMissing)
		return
	}
	weather, err := m.fetchWeather(r.Context(), cfg.WeatherBase, *loc, params.Refresh != nil && *params.Refresh)
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "weather_unavailable", err.Error()))
		return
	}
	httpx.JSON(w, http.StatusOK, weather)
}

// ---- actions ----

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "brief.preview",
		Title: "查看今天的早报",
		Description: "Generate today's daily brief without sending it: weather, calendar, due issues, reminders, " +
			"server alerts since yesterday and habits. Returns {date, content} with content in Markdown.",
		Input:  actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			if len(raw) > 0 && string(raw) != "{}" && string(raw) != "null" {
				return nil, httpx.Invalid("这个动作不需要参数")
			}
			cfg, err := m.load(ctx)
			if err != nil {
				return nil, err
			}
			res := m.generate(ctx, cfg, m.d.Registry, time.Now())
			return map[string]any{"date": res.date, "content": res.content}, nil
		},
	})
}
