package contracts

import (
	"context"
	"time"
)

// CheckinSeriesKey is where the check-in module (B132) registers CheckinSeries.
const CheckinSeriesKey = "checkins.series"

// CheckinDay is the average score of one question on one day.
type CheckinDay struct {
	QuestionID int64
	Label      string
	Date       string // YYYY-MM-DD in the server time zone
	Average    float64
	Count      int
}

// CheckinSeries gives other modules (B133 experiments) the daily scores.
type CheckinSeries interface {
	// Daily returns one entry per question and day that has answers, from
	// the date of from to the date of to, both included, oldest first.
	Daily(ctx context.Context, from, to time.Time) ([]CheckinDay, error)
}
