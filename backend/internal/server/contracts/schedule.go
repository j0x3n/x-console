package contracts

import "context"

// ScheduleKey is the registry key of the habits module's daily schedule (B162).
const ScheduleKey = "habits.schedule"

// ScheduleRules is the daily schedule the user set in the habits module.
// WakeTime and SleepTime are "HH:MM" in Timezone. WorkDays are ISO weekdays, 1 (Monday) to 7 (Sunday).
type ScheduleRules struct {
	WorkDays  []int  `json:"workDays"`
	WakeTime  string `json:"wakeTime"`
	SleepTime string `json:"sleepTime"`
	Timezone  string `json:"timezone"`
}

// Schedule is provided by the habits module.
type Schedule interface {
	Rules(ctx context.Context) (ScheduleRules, error)
}
