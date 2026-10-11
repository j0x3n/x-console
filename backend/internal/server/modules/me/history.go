package me

import (
	"context"
	"net/http"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/me/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/me/db"
)

// GetMeHistory implements api.ServerInterface.
func (m *Module) GetMeHistory(w http.ResponseWriter, r *http.Request, params api.GetMeHistoryParams) {
	days := 7
	if params.Days != nil {
		days = *params.Days
	}
	if days < 1 || days > 180 {
		httpx.Fail(w, r, httpx.Invalid("天数要在 1 到 180 之间"))
		return
	}
	out, err := m.history(r.Context(), days)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// history cuts the state rows into days of the schedule's time zone.
func (m *Module) history(ctx context.Context, days int) (api.MeHistory, error) {
	loc := m.d.Config.Location
	if s := m.loadSchedule(ctx); s != nil && s.Loc != nil {
		loc = s.Loc
	}
	if loc == nil {
		loc = time.UTC
	}
	now := m.now()
	local := now.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	first := today.AddDate(0, 0, -(days - 1))
	since := first.UTC()
	rows, err := m.q.ListMeStatesSince(ctx, &since)
	if err != nil {
		return api.MeHistory{}, err
	}
	out := api.MeHistory{Days: make([]api.MeDay, 0, days)}
	for i := 0; i < days; i++ {
		start := today.AddDate(0, 0, -i)
		end := start.AddDate(0, 0, 1)
		day := api.MeDay{Date: start.Format(time.DateOnly), Segments: []api.MeSegment{}}
		var best *db.MeState
		for k := range rows {
			row := rows[k]
			rowEnd := now
			if row.EndedAt != nil {
				rowEnd = *row.EndedAt
			}
			if !row.StartedAt.Before(end) || !rowEnd.After(start) || (row.EndedAt != nil && !rowEnd.After(row.StartedAt)) {
				continue
			}
			seg := api.MeSegment{State: api.MeStateName(row.State), StartedAt: maxTime(row.StartedAt, start), Guessed: row.Guessed != 0, Manual: row.Manual != 0}
			if row.EndedAt != nil && row.EndedAt.Before(end) {
				e := *row.EndedAt
				seg.EndedAt = &e
			} else if row.EndedAt != nil {
				e := end
				seg.EndedAt = &e
			} else if now.After(end) {
				e := end
				seg.EndedAt = &e
			}
			day.Segments = append(day.Segments, seg)
			// The sleep of a day is the longest sleep that ended in it.
			if row.State == contracts.MeSleeping && row.EndedAt != nil && !row.EndedAt.Before(start) && row.EndedAt.Before(end) &&
				row.EndedAt.Sub(row.StartedAt) >= minSleep && (best == nil || row.EndedAt.Sub(row.StartedAt) > best.EndedAt.Sub(best.StartedAt)) {
				best = &rows[k]
			}
		}
		if best != nil {
			slept, woke := best.StartedAt, *best.EndedAt
			minutes := int(woke.Sub(slept) / time.Minute)
			guessed := best.Guessed != 0
			day.SleptAt, day.WokeAt, day.SleepMinutes, day.SleepGuessed = &slept, &woke, &minutes, &guessed
		}
		out.Days = append(out.Days, day)
	}
	return out, nil
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
