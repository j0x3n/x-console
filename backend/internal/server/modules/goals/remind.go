package goals

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const (
	// OptionsKey holds the module's settings.
	OptionsKey = "goals.options"
	// lastNoticeKey holds the Monday (YYYY-MM-DD) of the week that was
	// reminded last, so a week gets one reminder.
	lastNoticeKey = "goals.last_review_notice"

	defaultReviewTime = "20:00"
)

var timeRe = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

// options reads the settings. A missing or broken value gives the defaults.
func (m *Module) options(ctx context.Context) api.GoalSettings {
	o := api.GoalSettings{ReviewReminder: true, ReviewTime: defaultReviewTime}
	var saved api.GoalSettings
	if err := m.d.Settings.Get(ctx, OptionsKey, &saved); err == nil && timeRe.MatchString(saved.ReviewTime) {
		o = saved
	}
	return o
}

// GetGoalSettings implements api.ServerInterface.
func (m *Module) GetGoalSettings(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, m.options(r.Context()))
}

// PutGoalSettings implements api.ServerInterface.
func (m *Module) PutGoalSettings(w http.ResponseWriter, r *http.Request) {
	var body api.GoalSettings
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	if !timeRe.MatchString(body.ReviewTime) {
		httpx.Fail(w, r, httpx.Invalid("时间要写成 HH:MM，比如 20:00"))
		return
	}
	err := m.d.Settings.Set(r.Context(), OptionsKey, body)
	m.d.Audit.Record(r.Context(), "goal.settings", "", map[string]any{"reviewTime": body.ReviewTime, "reviewReminder": body.ReviewReminder}, err)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, body)
}

// remindTick sends the review reminder when it is due. It runs every minute.
// It sends on Sunday from the set time on, once per week, only when there is
// an active goal this week and the week has no review text yet.
func (m *Module) remindTick(ctx context.Context, now time.Time) error {
	opts := m.options(ctx)
	if !opts.ReviewReminder {
		return nil
	}
	local := now.In(m.loc())
	if local.Weekday() != time.Sunday {
		return nil
	}
	hh, _ := strconv.Atoi(opts.ReviewTime[:2])
	mm, _ := strconv.Atoi(opts.ReviewTime[3:])
	if local.Hour()*60+local.Minute() < hh*60+mm {
		return nil
	}
	ws := weekStart(local, m.loc())
	key := ws.Format(dateLayout)
	var last string
	if err := m.d.Settings.Get(ctx, lastNoticeKey, &last); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return err
	}
	if last == key {
		return nil
	}
	if row, err := m.q.GetGoalReview(ctx, key); err == nil && answered(row) {
		return nil
	}
	goals, err := m.weekGoals(ctx, ws, now)
	if err != nil {
		return err
	}
	if len(goals) == 0 {
		return nil
	}
	body := fmt.Sprintf("这周有 %d 个目标在进行。写几句复盘，再定下周的安排。", len(goals))
	for _, g := range goals {
		if g.State == api.GoalStateBehind {
			body = fmt.Sprintf("这周有 %d 个目标在进行，其中有落后的。写几句复盘，再定下周的安排。", len(goals))
			break
		}
	}
	if _, err := m.d.Notify.Send(ctx, notify.Notification{
		Kind: "goals.review", Source: "goals", Link: "/goals/review", Scope: "goals:review",
		Title: "该做每周复盘了", Body: body, Priority: notify.PriorityNormal,
	}); err != nil {
		return err
	}
	return m.d.Settings.Set(ctx, lastNoticeKey, key)
}
