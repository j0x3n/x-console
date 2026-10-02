package reminders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/teambition/rrule-go"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// ---- conversion and validation ----

func toAPI(r db.Reminder) api.Reminder {
	return api.Reminder{
		Icon: &r.Icon,
		Id:   r.ID, Title: r.Title, Body: r.Body, Link: r.Link, Rrule: r.Rrule, Dtstart: r.Dtstart,
		NextAt: r.NextAt, DueAt: dueAt(r), LastFiredAt: r.LastFiredAt, SnoozedUntil: r.SnoozedUntil,
		DoneAt: r.DoneAt, Enabled: r.Enabled == 1, Status: status(r), CreatedAt: r.CreatedAt,
	}
}

func cleanTitle(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", httpx.Invalid("标题不能为空")
	}
	if len([]rune(s)) > 200 {
		return "", httpx.Invalid("标题太长了")
	}
	return s, nil
}

func cleanLink(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") {
		return s, nil
	}
	return "", httpx.Invalid("链接要以 / 或 http(s):// 开头")
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.ErrNotFound
	}
	return err
}

func (m *Module) rule(r db.Reminder) (*rrule.RRule, error) {
	return parseRule(r.Rrule, r.Dtstart, m.d.Config.Location)
}

// ---- business logic (shared by handlers, actions and contracts) ----

func (m *Module) create(ctx context.Context, in contracts.CreateReminder, now time.Time) (db.Reminder, error) {
	if len([]rune(in.Icon)) > 64 {
		return db.Reminder{}, httpx.Invalid("图标太长了")
	}
	title, err := cleanTitle(in.Title)
	if err != nil {
		return db.Reminder{}, err
	}
	link, err := cleanLink(in.Link)
	if err != nil {
		return db.Reminder{}, err
	}
	if in.At.IsZero() {
		return db.Reminder{}, httpx.Invalid("时间不能为空")
	}
	rr := normalizeRule(in.RRule)
	rule, err := parseRule(rr, in.At, m.d.Config.Location)
	if err != nil {
		return db.Reminder{}, err
	}
	next := firstNext(rule, in.At, now)
	if rule != nil && next == nil {
		return db.Reminder{}, httpx.Invalid("这个重复规则以后不会再到点")
	}
	row, err := m.q.CreateReminder(ctx, db.CreateReminderParams{
		Title: title, Body: strings.TrimSpace(in.Body), Link: link, Rrule: rr,
		Icon:    strings.TrimSpace(in.Icon),
		Dtstart: in.At.UTC(), NextAt: next, CreatedAt: now.UTC(),
	})
	m.d.Audit.Record(ctx, "reminder.create", strconv.FormatInt(row.ID, 10), map[string]any{"title": title, "rrule": rr}, err)
	if err != nil {
		return row, err
	}
	m.d.Bus.Publish("reminder.created", toAPI(row))
	return row, nil
}

func (m *Module) get(ctx context.Context, id int64) (db.Reminder, error) {
	row, err := m.q.GetReminder(ctx, id)
	return row, notFound(err)
}

func (m *Module) list(ctx context.Context, rng string, now time.Time) ([]db.Reminder, error) {
	rows, err := m.q.ListReminders(ctx)
	if err != nil {
		return nil, err
	}
	endOfToday := startOfDay(now, m.d.Config.Location).AddDate(0, 0, 1)
	out := make([]db.Reminder, 0, len(rows))
	for _, r := range rows {
		if rng == "" || inRange(r, rng, endOfToday) {
			out = append(out, r)
		}
	}
	sortForRange(out, rng)
	if rng == rangeDone && len(out) > 100 {
		out = out[:100]
	}
	return out, nil
}

// save writes a changed reminder and publishes reminder.updated.
func (m *Module) save(ctx context.Context, r db.Reminder) (db.Reminder, error) {
	row, err := m.q.UpdateReminder(ctx, db.UpdateReminderParams{
		Title: r.Title, Body: r.Body, Link: r.Link, Rrule: r.Rrule, Dtstart: r.Dtstart, NextAt: r.NextAt,
		LastFiredAt: r.LastFiredAt, SnoozedUntil: r.SnoozedUntil, DoneAt: r.DoneAt, Enabled: r.Enabled, ID: r.ID,
		Icon: r.Icon,
	})
	if err != nil {
		return row, notFound(err)
	}
	m.d.Bus.Publish("reminder.updated", toAPI(row))
	return row, nil
}

func (m *Module) update(ctx context.Context, id int64, p api.ReminderPatch, now time.Time) (db.Reminder, error) {
	r, err := m.get(ctx, id)
	if err != nil {
		return r, err
	}
	if p.Title != nil {
		if r.Title, err = cleanTitle(*p.Title); err != nil {
			return r, err
		}
	}
	if p.Body != nil {
		r.Body = strings.TrimSpace(*p.Body)
	}
	if p.Icon != nil {
		if len([]rune(*p.Icon)) > 64 {
			return r, httpx.Invalid("图标太长了")
		}
		r.Icon = strings.TrimSpace(*p.Icon)
	}
	if p.Link != nil {
		if r.Link, err = cleanLink(*p.Link); err != nil {
			return r, err
		}
	}
	reschedule := false
	if p.At != nil && !p.At.Equal(r.Dtstart) {
		r.Dtstart, reschedule = p.At.UTC(), true
	}
	if p.Rrule != nil && normalizeRule(*p.Rrule) != r.Rrule {
		r.Rrule, reschedule = normalizeRule(*p.Rrule), true
	}
	if p.Enabled != nil {
		was := r.Enabled == 1
		r.Enabled = 0
		if *p.Enabled {
			r.Enabled = 1
		}
		// Turning a repeating reminder back on starts from now, without a backlog.
		if !was && *p.Enabled && r.Rrule != "" {
			reschedule = true
		}
	}
	if reschedule {
		rule, err := m.rule(r)
		if err != nil {
			return r, err
		}
		r.NextAt = firstNext(rule, r.Dtstart, now)
		if rule != nil && r.NextAt == nil {
			return r, httpx.Invalid("这个重复规则以后不会再到点")
		}
		r.SnoozedUntil, r.DoneAt, r.LastFiredAt = nil, nil, nil
	}
	row, err := m.save(ctx, r)
	m.d.Audit.Record(ctx, "reminder.update", strconv.FormatInt(id, 10), nil, err)
	return row, err
}

func (m *Module) remove(ctx context.Context, id int64) error {
	n, err := m.q.DeleteReminder(ctx, id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(ctx, "reminder.delete", strconv.FormatInt(id, 10), nil, err)
	if err != nil {
		return err
	}
	if files, ok := module.Lookup[contracts.Files](m.d.Registry, contracts.FilesKey); ok {
		if err := files.DeleteOwned(ctx, "reminder", id); err != nil {
			return err
		}
	}
	m.d.Bus.Publish("reminder.deleted", map[string]int64{"id": id})
	return nil
}

func (m *Module) complete(ctx context.Context, id int64, now time.Time) (db.Reminder, error) {
	r, err := m.get(ctx, id)
	if err != nil {
		return r, err
	}
	rule, err := m.rule(r)
	if err != nil {
		return r, err
	}
	row, err := m.save(ctx, complete(r, rule, now))
	m.d.Audit.Record(ctx, "reminder.done", strconv.FormatInt(id, 10), nil, err)
	return row, err
}

func (m *Module) snooze(ctx context.Context, id int64, minutes int, now time.Time) (db.Reminder, error) {
	if minutes < 1 || minutes > 7*24*60 {
		return db.Reminder{}, httpx.Invalid("稍后的分钟数要在 1 到 10080 之间")
	}
	r, err := m.get(ctx, id)
	if err != nil {
		return r, err
	}
	rule, err := m.rule(r)
	if err != nil {
		return r, err
	}
	row, err := m.save(ctx, snooze(r, rule, time.Duration(minutes)*time.Minute, now))
	m.d.Audit.Record(ctx, "reminder.snooze", strconv.FormatInt(id, 10), map[string]any{"minutes": minutes}, err)
	return row, err
}

// scan fires every reminder due at now. The scheduler calls it every 30
// seconds; tests call it with any time.
func (m *Module) scan(ctx context.Context, now time.Time) error {
	rows, err := m.q.ListDueReminders(ctx, &now)
	if err != nil {
		return err
	}
	for _, r := range rows {
		rule, err := m.rule(r)
		if err != nil {
			// A rule stored before validation changed: stop it instead of retrying forever.
			m.d.Log.Warn("reminder has an invalid rule, disabling", "id", r.ID, "err", err)
			r.Enabled = 0
			_, _ = m.save(ctx, r)
			continue
		}
		f, ok := fire(r, rule, now)
		if !ok {
			continue
		}
		row, err := m.save(ctx, f.rem)
		if err != nil {
			return err
		}
		if _, err := m.d.Notify.Send(ctx, m.dueNotification(row, f)); err != nil {
			return err
		}
		m.d.Bus.Publish("reminder.due", toAPI(row))
	}
	return nil
}

func (m *Module) dueNotification(r db.Reminder, f fired) notify.Notification {
	body := r.Body
	if f.missed {
		note := fmt.Sprintf("错过的提醒，原定 %s。", f.scheduled.In(m.d.Config.Location).Format("1月2日 15:04"))
		body = strings.TrimSpace(note + "\n" + body)
	}
	link := r.Link
	if link == "" {
		link = "/reminders"
	}
	id := strconv.FormatInt(r.ID, 10)
	return notify.Notification{
		Kind: "reminder.due", Title: r.Title, Body: body, Link: link, Source: "reminders",
		Actions: []notify.Action{
			{ID: "reminder.done:" + id, Label: "完成"},
			{ID: "reminder.snooze:" + id + ":10", Label: "10 分钟后"},
		},
		Data: map[string]any{"reminderId": r.ID, "missed": f.missed},
	}
}

// handleAction runs notification buttons: reminder.done:<id> and
// reminder.snooze:<id>:<minutes>.
func (m *Module) handleAction(ctx context.Context, actionID string) error {
	parts := strings.Split(strings.TrimPrefix(actionID, "reminder."), ":")
	if len(parts) < 2 {
		return httpx.ErrNotFound
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return httpx.ErrNotFound
	}
	now := time.Now()
	switch {
	case parts[0] == "done" && len(parts) == 2:
		_, err = m.complete(ctx, id, now)
	case parts[0] == "snooze" && len(parts) == 3:
		minutes, perr := strconv.Atoi(parts[2])
		if perr != nil {
			return httpx.ErrNotFound
		}
		_, err = m.snooze(ctx, id, minutes, now)
	default:
		return httpx.ErrNotFound
	}
	return err
}

// ---- contracts.Reminders ----

// Create implements contracts.Reminders.
func (m *Module) Create(ctx context.Context, in contracts.CreateReminder) (int64, error) {
	row, err := m.create(ctx, in, time.Now())
	return row.ID, err
}

// Upcoming implements contracts.Reminders: every occurrence of enabled
// reminders between now and until, soonest first (at most 50 per reminder).
func (m *Module) Upcoming(ctx context.Context, until time.Time) ([]contracts.ReminderRef, error) {
	rows, err := m.q.ListReminders(ctx)
	if err != nil {
		return nil, err
	}
	var out []contracts.ReminderRef
	for _, r := range rows {
		if r.Enabled == 0 {
			continue
		}
		ref := func(at time.Time) contracts.ReminderRef {
			return contracts.ReminderRef{ID: r.ID, Title: r.Title, At: at, Link: r.Link}
		}
		if r.SnoozedUntil != nil && !r.SnoozedUntil.After(until) {
			out = append(out, ref(*r.SnoozedUntil))
		}
		if r.NextAt == nil || r.NextAt.After(until) {
			continue
		}
		rule, err := m.rule(r)
		if err != nil || rule == nil {
			out = append(out, ref(*r.NextAt))
			continue
		}
		for i, at := range rule.Between(*r.NextAt, until, true) {
			if i >= 50 {
				break
			}
			out = append(out, ref(at.UTC()))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// ---- HTTP handlers ----

func (m *Module) ListReminders(w http.ResponseWriter, r *http.Request, params api.ListRemindersParams) {
	rng := ""
	if params.Range != nil {
		rng = string(*params.Range)
	}
	rows, err := m.list(r.Context(), rng, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := make([]api.Reminder, 0, len(rows))
	for _, row := range rows {
		items = append(items, toAPI(row))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (m *Module) CreateReminder(w http.ResponseWriter, r *http.Request) {
	var body api.CreateReminderJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := contracts.CreateReminder{Title: body.Title, At: body.At}
	if body.Icon != nil {
		in.Icon = *body.Icon
	}
	if body.Body != nil {
		in.Body = *body.Body
	}
	if body.Link != nil {
		in.Link = *body.Link
	}
	if body.Rrule != nil {
		in.RRule = *body.Rrule
	}
	row, err := m.create(r.Context(), in, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if files, ok := module.Lookup[contracts.Files](m.d.Registry, contracts.FilesKey); ok {
		// Saved already: log instead of failing, so the client does not retry.
		if err := files.Claim(r.Context(), "reminder", row.ID, row.Body); err != nil {
			m.d.Log.Error("claim images failed", "id", row.ID, "err", err)
		}
	}
	httpx.JSON(w, http.StatusCreated, toAPI(row))
}

func (m *Module) GetReminder(w http.ResponseWriter, r *http.Request, id api.ReminderId) {
	row, err := m.get(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPI(row))
}

func (m *Module) UpdateReminder(w http.ResponseWriter, r *http.Request, id api.ReminderId) {
	var body api.UpdateReminderJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.update(r.Context(), id, body, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if files, ok := module.Lookup[contracts.Files](m.d.Registry, contracts.FilesKey); ok {
		// Saved already: log instead of failing, so the client does not retry.
		if err := files.Claim(r.Context(), "reminder", id, row.Body); err != nil {
			m.d.Log.Error("claim images failed", "id", id, "err", err)
		}
	}
	httpx.JSON(w, http.StatusOK, toAPI(row))
}

func (m *Module) DeleteReminder(w http.ResponseWriter, r *http.Request, id api.ReminderId) {
	if err := m.remove(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) CompleteReminder(w http.ResponseWriter, r *http.Request, id api.ReminderId) {
	row, err := m.complete(r.Context(), id, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPI(row))
}

func (m *Module) SnoozeReminder(w http.ResponseWriter, r *http.Request, id api.ReminderId) {
	var body api.SnoozeReminderJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.snooze(r.Context(), id, body.Minutes, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPI(row))
}
