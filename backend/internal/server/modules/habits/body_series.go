package habits

import (
	"context"
	"math"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// bodySeriesSource gives the experiments module (B133) the daily body data.
type bodySeriesSource struct{ m *Module }

// Daily implements contracts.BodySeries.
func (b bodySeriesSource) Daily(ctx context.Context, metric string, from, to time.Time) ([]contracts.BodyPoint, error) {
	if !slices.Contains(bodyMetrics, metric) {
		return nil, httpx.Invalid("不认识的身体数据")
	}
	rules, err := b.m.loadSchedule(ctx)
	if err != nil {
		return nil, err
	}
	loc := rules.location()
	since, until := dateKey(from, loc), dateKey(to, loc)
	rows, err := b.m.personalDays(ctx, since)
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Date < rows[j].Date })
	var out []contracts.BodyPoint
	for _, d := range rows {
		if d.Date < since || d.Date > until {
			continue
		}
		text := bodyValue(d.PersonalDay, metric)
		if text == "" {
			continue
		}
		v, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		out = append(out, contracts.BodyPoint{Date: d.Date, Value: v})
	}
	return out, nil
}
