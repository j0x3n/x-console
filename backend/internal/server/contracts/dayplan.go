package contracts

import "context"

// DayPlanSourceKey is where the day plan module (B135) registers DayPlanSource.
const DayPlanSourceKey = "dayplan.source"

// DayPlanSource gives the brief the plan of a day.
type DayPlanSource interface {
	// Lines returns the plan of the date (YYYY-MM-DD) as one line per item,
	// for example "09:00–10:30 写周报", and its status ("draft" or
	// "confirmed"). It returns no lines when the day has no plan.
	Lines(ctx context.Context, date string) (status string, lines []string, err error)
}
