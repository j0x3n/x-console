package skills

import (
	"time"
)

const weekCount = 8

const dateLayout = "2006-01-02"

// monday returns the Monday (00:00) of the week that day falls in.
func monday(day time.Time) time.Time {
	back := (int(day.Weekday()) + 6) % 7
	return time.Date(day.Year(), day.Month(), day.Day()-back, 0, 0, 0, 0, day.Location())
}

// weekStarts returns the Mondays of the last weekCount weeks, oldest first.
// The last one is the week of now.
func weekStarts(now time.Time) []string {
	this := monday(now)
	out := make([]string, weekCount)
	for i := range out {
		out[i] = this.AddDate(0, 0, -7*(weekCount-1-i)).Format(dateLayout)
	}
	return out
}

// bucketWeeks adds up minutes per day into the weeks that start at starts
// (sorted, oldest first). Days before the first week are left out.
func bucketWeeks(starts []string, daily map[string]int) []int {
	out := make([]int, len(starts))
	for day, minutes := range daily {
		for i := len(starts) - 1; i >= 0; i-- {
			if day >= starts[i] {
				out[i] += minutes
				break
			}
		}
	}
	return out
}
