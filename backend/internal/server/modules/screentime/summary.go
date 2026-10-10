package screentime

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/screentime/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/screentime/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	dateLayout = "2006-01-02"
	topApps    = 10
)

// bounds is the first day of the range and the day after its last day.
func bounds(rng string, day time.Time) (time.Time, time.Time) {
	switch rng {
	case "week":
		back := (int(day.Weekday()) + 6) % 7 // Monday is the first day
		start := day.AddDate(0, 0, -back)
		return start, start.AddDate(0, 0, 7)
	case "month":
		start := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
		return start, start.AddDate(0, 1, 0)
	default:
		return day, day.AddDate(0, 0, 1)
	}
}

func minuteOf(t time.Time) int64 { return t.Unix() / 60 }

// summary adds up one range. date is empty for today. hostID is empty for all
// computers.
func (m *Module) summary(ctx context.Context, rng, date, hostID string) (api.ScreenTimeSummary, error) {
	loc := m.location()
	if rng == "" {
		rng = "day"
	}
	if rng != "day" && rng != "week" && rng != "month" {
		return api.ScreenTimeSummary{}, httpx.Invalid("范围只能是 day、week、month")
	}
	n := m.now().In(loc)
	day := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	if date != "" {
		t, err := time.ParseInLocation(dateLayout, date, loc)
		if err != nil {
			return api.ScreenTimeSummary{}, httpx.Invalid("日期要写成 YYYY-MM-DD")
		}
		day = t
	}
	from, to := bounds(rng, day)
	out := api.ScreenTimeSummary{
		Range: api.ScreenTimeSummaryRange(rng), From: from.Format(dateLayout), To: to.AddDate(0, 0, -1).Format(dateLayout),
		Categories: []api.CategoryMinutes{}, Apps: []api.AppMinutes{}, Days: []api.DayMinutes{},
	}

	st, err := m.loadState(ctx)
	if err != nil {
		return out, err
	}
	out.State, err = m.stateOf(ctx, st)
	if err != nil {
		return out, err
	}

	cats, err := m.categoryTotals(ctx, minuteOf(from), minuteOf(to), hostID)
	if err != nil {
		return out, err
	}
	for _, c := range categories {
		if cats[c] > 0 {
			out.Categories = append(out.Categories, api.CategoryMinutes{Category: api.ScreenCategory(c), Minutes: cats[c]})
			out.Minutes += cats[c]
		}
	}
	sort.SliceStable(out.Categories, func(i, j int) bool { return out.Categories[i].Minutes > out.Categories[j].Minutes })

	apps, err := m.q.ScreenAppTotals(ctx, db.ScreenAppTotalsParams{FromMinute: minuteOf(from), ToMinute: minuteOf(to), Host: hostID})
	if err != nil {
		return out, err
	}
	out.Apps = topPrograms(apps)

	if rng != "day" {
		for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
			next := d.AddDate(0, 0, 1)
			byCat, err := m.categoryTotals(ctx, minuteOf(d), minuteOf(next), hostID)
			if err != nil {
				return out, err
			}
			item := api.DayMinutes{Date: d.Format(dateLayout), Categories: map[string]int{}}
			for c, v := range byCat {
				if v > 0 {
					item.Categories[c] = v
					item.Minutes += v
				}
			}
			out.Days = append(out.Days, item)
		}
	}
	return out, nil
}

func (m *Module) categoryTotals(ctx context.Context, from, to int64, hostID string) (map[string]int, error) {
	rows, err := m.q.ScreenCategoryTotals(ctx, db.ScreenCategoryTotalsParams{FromMinute: from, ToMinute: to, Host: hostID})
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, r := range rows {
		out[r.Category] = int(r.Minutes)
	}
	return out, nil
}

// topPrograms merges rows of one program (Code.exe and code.exe), gives it the
// category it spent most minutes in, and keeps the first ten.
func topPrograms(rows []db.ScreenAppTotalsRow) []api.AppMinutes {
	type acc struct {
		app      string
		minutes  int
		best     int
		category string
	}
	byName := map[string]*acc{}
	for _, r := range rows {
		key := normApp(r.App)
		a := byName[key]
		if a == nil {
			a = &acc{app: r.App}
			byName[key] = a
		}
		a.minutes += int(r.Minutes)
		if int(r.Minutes) > a.best {
			a.best, a.category = int(r.Minutes), r.Category
		}
	}
	list := make([]*acc, 0, len(byName))
	for _, a := range byName {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].minutes != list[j].minutes {
			return list[i].minutes > list[j].minutes
		}
		return strings.ToLower(list[i].app) < strings.ToLower(list[j].app)
	})
	if len(list) > topApps {
		list = list[:topApps]
	}
	out := make([]api.AppMinutes, len(list))
	for i, a := range list {
		out[i] = api.AppMinutes{App: a.app, Category: api.ScreenCategory(a.category), Minutes: a.minutes}
	}
	return out
}

// stateOf says why the page may be empty.
func (m *Module) stateOf(ctx context.Context, st state) (api.ScreenTimeState, error) {
	if !st.Enabled {
		return api.Disabled, nil
	}
	n, err := m.q.CountScreenMinutes(ctx)
	if err != nil {
		return "", err
	}
	if n > 0 {
		return api.Ok, nil
	}
	agents, err := m.d.Agents.List(ctx)
	if err != nil {
		return "", err
	}
	for _, a := range agents {
		if a.Has(protocol.CapScreenTime) {
			return api.Waiting, nil
		}
	}
	return api.NoAgent, nil
}

// GetScreenTimeSummary implements api.ServerInterface.
func (m *Module) GetScreenTimeSummary(w http.ResponseWriter, r *http.Request, params api.GetScreenTimeSummaryParams) {
	var rng, date, host string
	if params.Range != nil {
		rng = string(*params.Range)
	}
	if params.Date != nil {
		date = *params.Date
	}
	if params.HostId != nil {
		host = *params.HostId
	}
	v, err := m.summary(r.Context(), rng, date, host)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}
