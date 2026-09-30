package reminders

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
)

func (m *Module) ListExternalReminders(w http.ResponseWriter, r *http.Request, params api.ListExternalRemindersParams) {
	loc := m.d.Scheduler.Location()
	if loc == nil {
		loc = time.Local
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	from, until := today, today.AddDate(0, 0, 1)
	switch params.Range {
	case api.ListExternalRemindersParamsRangeToday:
	case api.ListExternalRemindersParamsRangeUpcoming:
		from = until
		until = from.AddDate(0, 0, 30)
	default:
		httpx.Fail(w, r, httpx.Invalid("范围无效"))
		return
	}
	items := []api.ExternalReminder{}
	for key, source := range module.All[contracts.ReminderSource](m.d.Registry) {
		if !strings.HasPrefix(key, contracts.ReminderSourcePrefix) {
			continue
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		found, err := source.Upcoming(ctx, from, until)
		cancel()
		if err != nil {
			m.d.Log.Warn("external reminders source failed", "source", key, "err", err)
			continue
		}
		for _, entry := range found {
			if !entry.At.Before(until) || (entry.At.Before(from) && (params.Range == api.ListExternalRemindersParamsRangeUpcoming || entry.Done)) {
				continue
			}
			kind := api.ExternalReminderSource(entry.Source)
			if !kind.Valid() {
				kind = api.ExternalReminderSourceOther
			}
			items = append(items, api.ExternalReminder{Id: entry.ID, Source: kind, SourceLabel: entry.SourceLabel, Title: entry.Title, At: entry.At, Link: entry.Link, Done: entry.Done})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].At.Equal(items[j].At) {
			return items[i].Id < items[j].Id
		}
		return items[i].At.Before(items[j].At)
	})
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}
