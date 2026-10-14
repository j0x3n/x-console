package recipes

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

const (
	// maxTimers is how many timers can be open (running, paused or done and
	// not closed) at once.
	maxTimers = 20
	// timerCheckEvery is how often the server looks for timers that ran out.
	timerCheckEvery = 5 * time.Second
	// doneKeep is how long a finished timer stays when nobody closes it.
	doneKeep = 6 * time.Hour
)

func (m *Module) timerView(row db.CookTimer, now time.Time) api.CookTimer {
	v := api.CookTimer{
		Id: row.ID, RecipeId: row.RecipeID, RecipeTitle: row.RecipeTitle, Label: row.Label,
		Seconds: int(row.Seconds), State: api.CookTimerState(row.State), CreatedAt: row.CreatedAt,
	}
	if row.StepIndex != nil {
		v.StepIndex = ptr(int(*row.StepIndex))
	}
	switch row.State {
	case string(api.Paused):
		v.RemainingSeconds = int(row.RemainingSeconds)
	case string(api.Done):
		v.EndsAt = row.EndsAt
	default:
		v.EndsAt = row.EndsAt
		if row.EndsAt != nil {
			v.RemainingSeconds = remaining(*row.EndsAt, now)
		}
	}
	return v
}

// remaining is the whole seconds left until end, rounded up, never negative.
func remaining(end, now time.Time) int {
	d := end.Sub(now)
	if d <= 0 {
		return 0
	}
	return int((d + time.Second - 1) / time.Second)
}

func (m *Module) getTimer(ctx context.Context, id int64) (db.CookTimer, error) {
	row, err := m.q.GetCookTimer(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, httpx.ErrNotFound
	}
	return row, err
}

// listTimers returns the open timers, the ones that end first at the front.
func (m *Module) listTimers(ctx context.Context) ([]api.CookTimer, error) {
	rows, err := m.q.ListCookTimers(ctx)
	if err != nil {
		return nil, err
	}
	now := m.now()
	items := make([]api.CookTimer, len(rows))
	for i, row := range rows {
		items[i] = m.timerView(row, now)
	}
	// the order is by the time each one ends; a paused one ends when it
	// would if it were resumed now
	endOf := func(v api.CookTimer) time.Time {
		if v.EndsAt != nil {
			return *v.EndsAt
		}
		return now.Add(time.Duration(v.RemainingSeconds) * time.Second)
	}
	sort.SliceStable(items, func(i, j int) bool {
		ei, ej := endOf(items[i]), endOf(items[j])
		if !ei.Equal(ej) {
			return ei.Before(ej)
		}
		return items[i].Id < items[j].Id
	})
	return items, nil
}

// ListCookTimers implements api.ServerInterface.
func (m *Module) ListCookTimers(w http.ResponseWriter, r *http.Request) {
	items, err := m.listTimers(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// CreateCookTimer implements api.ServerInterface.
func (m *Module) CreateCookTimer(w http.ResponseWriter, r *http.Request) {
	var body api.CookTimerInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	v, err := m.startTimer(ctx, body)
	m.auditTimer(ctx, "cook_timer.create", v.Id, map[string]any{"label": body.Label}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, v)
}

func (m *Module) auditTimer(ctx context.Context, action string, id int64, detail map[string]any, err error) {
	target := ""
	if id != 0 {
		target = strconv.FormatInt(id, 10)
	}
	m.d.Audit.Record(ctx, action, target, detail, err)
}

// startTimer validates and starts a timer, and publishes the event.
func (m *Module) startTimer(ctx context.Context, in api.CookTimerInput) (api.CookTimer, error) {
	label := strings.TrimSpace(in.Label)
	if label == "" {
		return api.CookTimer{}, httpx.Invalid("计时的名字不能为空")
	}
	if len([]rune(label)) > maxTimerLabel {
		return api.CookTimer{}, httpx.Invalid("计时的名字太长了，最多 30 字")
	}
	if in.Seconds < 1 || in.Seconds > maxTimerSeconds {
		return api.CookTimer{}, httpx.Invalid("计时的时长要在 1 秒到 24 小时之间")
	}
	if in.StepIndex != nil && *in.StepIndex < 0 {
		return api.CookTimer{}, httpx.Invalid("步骤不正确")
	}
	var recipeID *int64
	title := ""
	if in.RecipeId != nil {
		row, err := m.q.GetRecipe(ctx, *in.RecipeId)
		if errors.Is(err, sql.ErrNoRows) {
			return api.CookTimer{}, httpx.Invalid("菜谱不存在")
		}
		if err != nil {
			return api.CookTimer{}, err
		}
		if in.StepIndex != nil && *in.StepIndex >= len(parseJSON[api.RecipeStep](row.Steps)) {
			return api.CookTimer{}, httpx.Invalid("这道菜没有第 " + strconv.Itoa(*in.StepIndex+1) + " 步")
		}
		recipeID, title = &row.ID, row.Title
	} else if in.StepIndex != nil {
		return api.CookTimer{}, httpx.Invalid("给了步骤就要同时给菜谱")
	}
	var step *int64
	if in.StepIndex != nil {
		step = ptr(int64(*in.StepIndex))
	}
	m.timerMu.Lock()
	defer m.timerMu.Unlock()
	n, err := m.q.CountCookTimers(ctx)
	if err != nil {
		return api.CookTimer{}, err
	}
	if n >= maxTimers {
		return api.CookTimer{}, httpx.Invalid("同时最多 20 个计时，先关掉几个")
	}
	now := m.now()
	ends := now.Add(time.Duration(in.Seconds) * time.Second).UTC()
	row, err := m.q.InsertCookTimer(ctx, db.InsertCookTimerParams{
		RecipeID: recipeID, RecipeTitle: title, StepIndex: step, Label: label, Seconds: int64(in.Seconds),
		EndsAt: &ends, CreatedAt: now.UTC(),
	})
	if err != nil {
		return api.CookTimer{}, err
	}
	v := m.timerView(row, now)
	m.d.Bus.Publish("cook_timer.created", v)
	return v, nil
}

// UpdateCookTimer implements api.ServerInterface.
func (m *Module) UpdateCookTimer(w http.ResponseWriter, r *http.Request, id api.TimerId) {
	var body api.CookTimerPatch
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	v, err := m.patchTimer(ctx, id, body)
	m.auditTimer(ctx, "cook_timer.update", id, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func conflict(msg string) error { return httpx.NewError(http.StatusConflict, "conflict", msg) }

func (m *Module) patchTimer(ctx context.Context, id int64, p api.CookTimerPatch) (api.CookTimer, error) {
	if (p.Action == nil) == (p.AddSeconds == nil) {
		return api.CookTimer{}, httpx.Invalid("一次只能做一件事：暂停、继续或者加时")
	}
	if p.Action != nil && !p.Action.Valid() {
		return api.CookTimer{}, httpx.Invalid("不认识的操作")
	}
	if p.AddSeconds != nil && (*p.AddSeconds == 0 || *p.AddSeconds < -maxTimerSeconds || *p.AddSeconds > maxTimerSeconds) {
		return api.CookTimer{}, httpx.Invalid("加的时间要在 -24 小时到 24 小时之间，不能是 0")
	}
	m.timerMu.Lock()
	defer m.timerMu.Unlock()
	cur, err := m.getTimer(ctx, id)
	if err != nil {
		return api.CookTimer{}, err
	}
	now := m.now()
	arg := db.UpdateCookTimerStateParams{ID: id, State: cur.State, EndsAt: cur.EndsAt, RemainingSeconds: cur.RemainingSeconds, DoneAt: cur.DoneAt}
	switch {
	case p.Action != nil && *p.Action == api.Pause:
		if cur.State != string(api.Running) {
			return api.CookTimer{}, conflict("只有在走的计时可以暂停")
		}
		left := 1
		if cur.EndsAt != nil {
			left = max(1, remaining(*cur.EndsAt, now))
		}
		arg.State, arg.EndsAt, arg.RemainingSeconds = string(api.Paused), nil, int64(left)
	case p.Action != nil:
		if cur.State != string(api.Paused) {
			return api.CookTimer{}, conflict("只有暂停的计时可以继续")
		}
		ends := now.Add(time.Duration(cur.RemainingSeconds) * time.Second).UTC()
		arg.State, arg.EndsAt, arg.RemainingSeconds = string(api.Running), &ends, 0
	default:
		add := *p.AddSeconds
		switch cur.State {
		case string(api.Paused):
			left, err := addLeft(int(cur.RemainingSeconds), add)
			if err != nil {
				return api.CookTimer{}, err
			}
			arg.RemainingSeconds = int64(left)
		case string(api.Done):
			left, err := addLeft(0, add)
			if err != nil {
				return api.CookTimer{}, err
			}
			ends := now.Add(time.Duration(left) * time.Second).UTC()
			arg.State, arg.EndsAt, arg.DoneAt = string(api.Running), &ends, nil
		default:
			left := 0
			if cur.EndsAt != nil {
				left = remaining(*cur.EndsAt, now)
			}
			left, err := addLeft(left, add)
			if err != nil {
				return api.CookTimer{}, err
			}
			ends := now.Add(time.Duration(left) * time.Second).UTC()
			arg.EndsAt = &ends
		}
	}
	row, err := m.q.UpdateCookTimerState(ctx, arg)
	if err != nil {
		return api.CookTimer{}, err
	}
	v := m.timerView(row, now)
	m.d.Bus.Publish("cook_timer.updated", v)
	return v, nil
}

// addLeft adds to the seconds left. At least one second stays.
func addLeft(left, add int) (int, error) {
	left = max(1, left+add)
	if left > maxTimerSeconds {
		return 0, httpx.Invalid("计时最长 24 小时")
	}
	return left, nil
}

// DeleteCookTimer implements api.ServerInterface.
func (m *Module) DeleteCookTimer(w http.ResponseWriter, r *http.Request, id api.TimerId) {
	ctx := r.Context()
	err := m.closeTimer(ctx, id)
	m.auditTimer(ctx, "cook_timer.delete", id, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) closeTimer(ctx context.Context, id int64) error {
	m.timerMu.Lock()
	defer m.timerMu.Unlock()
	n, err := m.q.DeleteCookTimer(ctx, id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	if err != nil {
		return err
	}
	m.d.Bus.Publish("cook_timer.deleted", map[string]any{"id": id})
	return nil
}

// handleAction handles the "cook_timer.dismiss:<id>" notification button.
func (m *Module) handleAction(ctx context.Context, actionID string) error {
	rest, ok := strings.CutPrefix(actionID, "cook_timer.dismiss:")
	if !ok {
		return httpx.ErrNotFound
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return httpx.ErrNotFound
	}
	err = m.closeTimer(ctx, id)
	if errors.Is(err, httpx.ErrNotFound) {
		// already closed in the app
		return nil
	}
	m.auditTimer(ctx, "cook_timer.delete", id, map[string]any{"via": "notification"}, err)
	return err
}

// checkTimers is the job that runs every few seconds: running timers that
// reached their time become done and notify, and timers that stayed done for
// too long are removed.
func (m *Module) checkTimers(ctx context.Context, now time.Time) error {
	now = now.UTC()
	due, err := m.q.ListDueCookTimers(ctx, &now)
	if err != nil {
		return err
	}
	for _, row := range due {
		if err := m.finishTimer(ctx, row.ID, now); err != nil {
			m.d.Log.Warn("cook timer finish", "id", row.ID, "err", err)
		}
	}
	cutoff := now.Add(-doneKeep)
	stale, err := m.q.ListStaleDoneCookTimers(ctx, &cutoff)
	if err != nil {
		return err
	}
	for _, id := range stale {
		if err := m.closeTimer(ctx, id); err != nil && !errors.Is(err, httpx.ErrNotFound) {
			m.d.Log.Warn("cook timer cleanup", "id", id, "err", err)
		}
	}
	return nil
}

func (m *Module) finishTimer(ctx context.Context, id int64, now time.Time) error {
	m.timerMu.Lock()
	n, err := m.q.FinishCookTimer(ctx, db.FinishCookTimerParams{DoneAt: &now, ID: id})
	if err != nil || n == 0 {
		m.timerMu.Unlock()
		return err
	}
	row, err := m.q.GetCookTimer(ctx, id)
	m.timerMu.Unlock()
	if err != nil {
		return err
	}
	v := m.timerView(row, now)
	m.d.Bus.Publish("cook_timer.done", v)
	_, err = m.d.Notify.Send(ctx, doneNotice(row))
	return err
}

// doneNotice is the notification of a timer that ran out.
func doneNotice(row db.CookTimer) notify.Notification {
	id := strconv.FormatInt(row.ID, 10)
	n := notify.Notification{
		Kind: "cook_timer.done", Source: "recipes", Priority: notify.PriorityHigh,
		Title: "时间到：" + row.Label, Link: "/recipes",
		Actions: []notify.Action{{ID: "cook_timer.dismiss:" + id, Label: "关掉"}},
		Data:    map[string]any{"timerId": row.ID},
	}
	if row.RecipeID != nil {
		n.Link = "/recipes/" + strconv.FormatInt(*row.RecipeID, 10) + "/cook"
	}
	n.Body = row.RecipeTitle
	if n.Body != "" && row.StepIndex != nil {
		n.Body += " · 第 " + strconv.FormatInt(*row.StepIndex+1, 10) + " 步"
	}
	return n
}
