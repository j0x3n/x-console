package contracts

import (
	"context"
	"time"
)

// BodySeriesKey is where the habits module registers BodySeries (B133).
const BodySeriesKey = "habits.body_series"

// BodyPoint is one recorded value of a body metric.
type BodyPoint struct {
	Date  string // YYYY-MM-DD in the server time zone
	Value float64
}

// BodySeries gives the experiments module the daily body data: weight, waist,
// sleep, restingHr and steps. Days without a record are left out.
type BodySeries interface {
	// Daily returns the recorded values from the date of from to the date of
	// to, both included, oldest first.
	Daily(ctx context.Context, metric string, from, to time.Time) ([]BodyPoint, error)
}
