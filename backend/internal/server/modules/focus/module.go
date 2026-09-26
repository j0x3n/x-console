// Package focus is the pomodoro part of M11. The browser shows the countdown;
// the server records every session and sends the "time is up" notification
// (kind focus.done), so it arrives even when the page is closed.
package focus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/focus/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/focus/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

const (
	defaultMinutes = 25
	maxMinutes     = 180
	// staleAfter closes a session nobody stopped, this long after its end.
	staleAfter = time.Hour
)

// ServiceKey is where the module registers itself, for tests and for other
// modules that want the running session.
const ServiceKey = "focus.sessions"

// Module implements api.ServerInterface.
type Module struct {
	d *module.Deps
	q *db.Queries

	// minute is the length of a planned minute. Tests shorten it.
	minute time.Duration

	mu     sync.Mutex
	timers map[int64]*time.Timer
	ctx    context.Context
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), minute: time.Minute, timers: map[int64]*time.Timer{}, ctx: context.Background()}
	d.Notify.OnAction("focus.", m.handleAction)
	module.Provide[*Module](d.Registry, ServiceKey, m)
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "focus" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start arms timers for running sessions and schedules the safety sweep.
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	rows, err := m.q.ListOpenFocus(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.NotifiedAt == nil {
			m.arm(r)
		}
	}
	m.d.Scheduler.Every("focus.sweep", time.Minute, func(ctx context.Context) error {
		return m.sweep(ctx, time.Now())
	})
	go func() {
		<-ctx.Done()
		m.mu.Lock()
		for id, t := range m.timers {
			t.Stop()
			delete(m.timers, id)
		}
		m.mu.Unlock()
	}()
	return nil
}

func (m *Module) endsAt(r db.FocusSession) time.Time {
	return r.StartedAt.Add(time.Duration(r.PlannedMinutes) * m.minute)
}

// arm starts the in-memory timer that sends the notification on time.
func (m *Module) arm(r db.FocusSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx.Err() != nil {
		return
	}
	if t, ok := m.timers[r.ID]; ok {
		t.Stop()
	}
	ctx, id := m.ctx, r.ID
	m.timers[id] = time.AfterFunc(time.Until(m.endsAt(r)), func() {
		m.mu.Lock()
		delete(m.timers, id)
		m.mu.Unlock()
		if err := m.fire(ctx, id, time.Now()); err != nil && ctx.Err() == nil {
			m.d.Log.Warn("focus notification", "id", id, "err", err)
		}
	})
}

func (m *Module) disarm(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.timers[id]; ok {
		t.Stop()
		delete(m.timers, id)
	}
}

// fire sends the "time is up" notification once, if the session is still
// running and its time has come.
func (m *Module) fire(ctx context.Context, id int64, now time.Time) error {
	r, err := m.q.GetFocus(ctx, id)
	if err != nil {
		return notFound(err)
	}
	if r.EndedAt != nil || r.NotifiedAt != nil || now.Before(m.endsAt(r)) {
		return nil
	}
	at := now.UTC()
	n, err := m.q.MarkFocusNotified(ctx, db.MarkFocusNotifiedParams{NotifiedAt: &at, ID: id})
	if err != nil || n == 0 {
		return err
	}
	body := fmt.Sprintf("%d 分钟到了，休息一下吧。", r.PlannedMinutes)
	link := "/calendar/focus"
	if r.IssueKey != "" {
		body = fmt.Sprintf("%s 专注了 %d 分钟，休息一下吧。", m.issueLabel(ctx, r.IssueKey), r.PlannedMinutes)
		link = issuePath(r.IssueKey)
	}
	_, err = m.d.Notify.Send(ctx, notify.Notification{
		Kind: "focus.done", Title: "番茄钟结束了", Body: body, Link: link, Priority: notify.PriorityHigh, Source: "focus",
		Actions: []notify.Action{{ID: fmt.Sprintf("focus.again:%d", id), Label: "再来一个"}},
		Data:    map[string]any{"focusId": id},
	})
	r.NotifiedAt = &at
	m.d.Bus.Publish("focus.done", m.toAPI(ctx, r))
	return err
}

// sweep is the safety net behind the timers (for example after a restart):
// it notifies sessions whose time is up and closes the ones nobody stopped.
// The scheduler calls it every minute; tests call it with any time.
func (m *Module) sweep(ctx context.Context, now time.Time) error {
	rows, err := m.q.ListOpenFocus(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range rows {
		if r.NotifiedAt == nil && !now.Before(m.endsAt(r)) {
			errs = append(errs, m.fire(ctx, r.ID, now))
		}
		if now.Sub(m.endsAt(r)) >= staleAfter {
			done := true
			_, err := m.stop(ctx, r.ID, &done, nil, now)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.ErrNotFound
	}
	return err
}

// issuePath is the in-app page of an issue: XC-12 lives at /projects/XC/12.
func issuePath(key string) string {
	i := strings.LastIndex(key, "-")
	if i <= 0 {
		return "/projects"
	}
	if _, err := strconv.Atoi(key[i+1:]); err != nil {
		return "/projects"
	}
	return "/projects/" + key[:i] + "/" + key[i+1:]
}

// issueLabel is "XC-12 标题" when the projects module can tell the title.
func (m *Module) issueLabel(ctx context.Context, key string) string {
	if title := m.issueTitle(ctx, key); title != "" {
		return key + " " + title
	}
	return key
}

func (m *Module) issueTitle(ctx context.Context, key string) string {
	if key == "" {
		return ""
	}
	issues, ok := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey)
	if !ok {
		return ""
	}
	ref, err := issues.Get(ctx, key)
	if err != nil {
		return ""
	}
	return ref.Title
}

func (m *Module) toAPI(ctx context.Context, r db.FocusSession) api.FocusSession {
	out := api.FocusSession{
		Id: r.ID, IssueKey: r.IssueKey, StartedAt: r.StartedAt, EndsAt: m.endsAt(r), EndedAt: r.EndedAt,
		PlannedMinutes: int(r.PlannedMinutes), ActualSeconds: int(r.ActualSeconds), Completed: r.Completed == 1,
		Note: r.Note, NotifiedAt: r.NotifiedAt,
	}
	if title := m.issueTitle(ctx, r.IssueKey); title != "" {
		out.IssueTitle = &title
	}
	return out
}

// ---- business methods ----

var errRunning = httpx.NewError(http.StatusConflict, "focus_running", "已经有一个番茄钟在进行")

func (m *Module) start(ctx context.Context, in api.FocusStart, now time.Time) (db.FocusSession, error) {
	minutes := defaultMinutes
	if in.Minutes != nil {
		minutes = *in.Minutes
	}
	if minutes < 1 || minutes > maxMinutes {
		return db.FocusSession{}, httpx.Invalid("时长要在 1 到 180 分钟之间")
	}
	key := strings.ToUpper(strings.TrimSpace(deref(in.IssueKey)))
	if len(key) > 40 {
		return db.FocusSession{}, httpx.Invalid("Issue 编号太长了")
	}
	if key != "" {
		if issues, ok := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey); ok {
			if _, err := issues.Get(ctx, key); err != nil {
				return db.FocusSession{}, httpx.Invalid("找不到 Issue " + key)
			}
		}
	}
	note := strings.TrimSpace(deref(in.Note))
	if len([]rune(note)) > 500 {
		return db.FocusSession{}, httpx.Invalid("备注太长了")
	}

	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return db.FocusSession{}, err
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	open, err := q.ListOpenFocus(ctx)
	if err != nil {
		return db.FocusSession{}, err
	}
	var closed []db.FocusSession
	for _, r := range open {
		if now.Before(m.endsAt(r)) {
			return db.FocusSession{}, errRunning
		}
		// Its time is up and nobody pressed stop: count it as done.
		c, err := q.StopFocus(ctx, stopParams(r, m.endsAt(r), true, r.Note, now))
		if err != nil {
			return db.FocusSession{}, err
		}
		closed = append(closed, c)
	}
	row, err := q.StartFocus(ctx, db.StartFocusParams{IssueKey: key, StartedAt: now.UTC(), PlannedMinutes: int64(minutes), Note: note})
	if err != nil {
		return row, err
	}
	if err := tx.Commit(); err != nil {
		return row, err
	}
	for _, c := range closed {
		m.disarm(c.ID)
		m.d.Bus.Publish("focus.stopped", m.toAPI(ctx, c))
	}
	m.d.Audit.Record(ctx, "focus.start", strconv.FormatInt(row.ID, 10), map[string]any{"minutes": minutes, "issue": key}, nil)
	m.arm(row)
	m.d.Bus.Publish("focus.started", m.toAPI(ctx, row))
	return row, nil
}

// stopParams computes the end of a session. Time after the planned end does
// not count, so a forgotten timer does not inflate the stats.
func stopParams(r db.FocusSession, plannedEnd time.Time, completed bool, note string, now time.Time) db.StopFocusParams {
	end := now
	if end.After(plannedEnd) {
		end = plannedEnd
	}
	secs := int64(end.Sub(r.StartedAt) / time.Second)
	if secs < 0 {
		secs = 0
	}
	var done int64
	if completed {
		done = 1
	}
	endedAt := now.UTC()
	return db.StopFocusParams{EndedAt: &endedAt, ActualSeconds: secs, Completed: done, Note: note, ID: r.ID}
}

func (m *Module) stop(ctx context.Context, id int64, completed *bool, note *string, now time.Time) (db.FocusSession, error) {
	r, err := m.q.GetFocus(ctx, id)
	if err != nil {
		return r, notFound(err)
	}
	if r.EndedAt != nil {
		return r, httpx.NewError(http.StatusConflict, "focus_ended", "这个番茄钟已经结束了")
	}
	done := !now.Before(m.endsAt(r).Add(-5 * time.Second))
	if completed != nil {
		done = *completed
	}
	text := r.Note
	if note != nil {
		text = strings.TrimSpace(*note)
		if len([]rune(text)) > 500 {
			return r, httpx.Invalid("备注太长了")
		}
	}
	row, err := m.q.StopFocus(ctx, stopParams(r, m.endsAt(r), done, text, now))
	if errors.Is(err, sql.ErrNoRows) {
		return r, httpx.NewError(http.StatusConflict, "focus_ended", "这个番茄钟已经结束了")
	}
	m.d.Audit.Record(ctx, "focus.stop", strconv.FormatInt(id, 10), map[string]any{"completed": done}, err)
	if err != nil {
		return r, err
	}
	m.disarm(id)
	m.d.Bus.Publish("focus.stopped", m.toAPI(ctx, row))
	return row, nil
}

func (m *Module) current(ctx context.Context) (*api.FocusSession, error) {
	r, err := m.q.CurrentFocus(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := m.toAPI(ctx, r)
	return &out, nil
}

func (m *Module) stats(ctx context.Context, days int, now time.Time) (api.FocusStats, error) {
	if days < 1 || days > 90 {
		return api.FocusStats{}, httpx.Invalid("days 要在 1 到 90 之间")
	}
	loc := m.d.Config.Location
	l := now.In(loc)
	first := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(days - 1))
	rows, err := m.q.ListFocusSince(ctx, first.UTC())
	if err != nil {
		return api.FocusStats{}, err
	}
	out := api.FocusStats{Days: make([]api.FocusDay, days), ByIssue: []api.FocusIssueTotal{}, Recent: []api.FocusSession{}}
	index := map[string]int{}
	for i := range days {
		d := first.AddDate(0, 0, i).Format(time.DateOnly)
		out.Days[i] = api.FocusDay{Date: d}
		index[d] = i
	}
	byIssue := map[string]int{}
	for _, r := range rows {
		if r.EndedAt == nil {
			continue
		}
		i, ok := index[r.StartedAt.In(loc).Format(time.DateOnly)]
		if !ok {
			continue
		}
		out.Days[i].Seconds += int(r.ActualSeconds)
		out.TotalSeconds += int(r.ActualSeconds)
		if r.Completed == 1 {
			out.Days[i].Sessions++
			out.Completed++
		}
		if r.IssueKey != "" {
			byIssue[r.IssueKey] += int(r.ActualSeconds)
		}
	}
	for k, s := range byIssue {
		out.ByIssue = append(out.ByIssue, api.FocusIssueTotal{IssueKey: k, Seconds: s})
	}
	sort.Slice(out.ByIssue, func(i, j int) bool {
		if out.ByIssue[i].Seconds != out.ByIssue[j].Seconds {
			return out.ByIssue[i].Seconds > out.ByIssue[j].Seconds
		}
		return out.ByIssue[i].IssueKey < out.ByIssue[j].IssueKey
	})
	recent, err := m.q.ListRecentFocus(ctx, 10)
	if err != nil {
		return out, err
	}
	for _, r := range recent {
		out.Recent = append(out.Recent, m.toAPI(ctx, r))
	}
	return out, nil
}

// handleAction handles "focus.again:<id>" from the notification button.
func (m *Module) handleAction(ctx context.Context, actionID string) error {
	rest, ok := strings.CutPrefix(actionID, "focus.again:")
	if !ok {
		return httpx.ErrNotFound
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return httpx.ErrNotFound
	}
	r, err := m.q.GetFocus(ctx, id)
	if err != nil {
		return notFound(err)
	}
	minutes := int(r.PlannedMinutes)
	_, err = m.start(ctx, api.FocusStart{Minutes: &minutes, IssueKey: &r.IssueKey}, time.Now())
	return err
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// ---- HTTP handlers ----

func (m *Module) StartFocus(w http.ResponseWriter, r *http.Request) {
	var body api.StartFocusJSONRequestBody
	if r.ContentLength != 0 {
		if err := httpx.Decode(r, &body); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	row, err := m.start(r.Context(), body, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, m.toAPI(r.Context(), row))
}

func (m *Module) StopFocus(w http.ResponseWriter, r *http.Request, id int64) {
	var body api.StopFocusJSONRequestBody
	if r.ContentLength != 0 {
		if err := httpx.Decode(r, &body); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	row, err := m.stop(r.Context(), id, body.Completed, body.Note, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m.toAPI(r.Context(), row))
}

func (m *Module) CurrentFocus(w http.ResponseWriter, r *http.Request) {
	cur, err := m.current(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := map[string]any{}
	if cur != nil {
		out["session"] = cur
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) FocusStats(w http.ResponseWriter, r *http.Request, params api.FocusStatsParams) {
	days := 7
	if params.Days != nil {
		days = *params.Days
	}
	out, err := m.stats(r.Context(), days, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
