package monitoring

import (
	"math"
	"slices"
	"strings"
	"time"
)

// Pure scheduling and state logic. Everything here takes the current time as
// an argument so tests drive it directly.

// ---- monitors ----

// failuresToAlert is how many failed checks in a row mark a monitor down.
const failuresToAlert = 2

// Monitor statuses.
const (
	statusUnknown = "unknown"
	statusUp      = "up"
	statusDown    = "down"
)

// transition is what a check result changed.
type transition int

const (
	noChange transition = iota
	wentDown
	cameUp
)

// monitorState is the alerting state kept on the monitor row.
type monitorState struct {
	Status   string // unknown, up, down
	Failures int
}

// nextState applies one check result. A monitor goes down after
// failuresToAlert failures in a row and comes back up on the first success.
// Only these two changes produce a notification.
func nextState(s monitorState, ok bool) (monitorState, transition) {
	if ok {
		was := s.Status
		s = monitorState{Status: statusUp}
		if was == statusDown {
			return s, cameUp
		}
		return s, noChange
	}
	// One failure is not an outage yet: the status stays as it was.
	s.Failures++
	if s.Status != statusDown && s.Failures >= failuresToAlert {
		s.Status = statusDown
		return s, wentDown
	}
	return s, noChange
}

// Expiry reminder thresholds in days.
var (
	tlsThresholds    = []int{14, 7, 3}
	domainThresholds = []int{30, 7}
)

func thresholdsFor(kind string) []int {
	if kind == kindDomain {
		return domainThresholds
	}
	return tlsThresholds
}

// daysLeft is the time until expiry in days, rounded down to 0.1.
func daysLeft(expires, now time.Time) float64 {
	d := expires.Sub(now).Hours() / 24
	return math.Floor(d*10) / 10
}

// expiryDue decides whether an expiry reminder is due. notified holds the
// thresholds already sent for this expiry date. When the first check finds
// 5 days left, only one reminder goes out (for the 7-day threshold) and
// every threshold at or above the remaining time is marked as sent, so a
// later check does not send the 14-day one after the 7-day one.
func expiryDue(left float64, thresholds, notified []int) (threshold int, due bool, marked []int) {
	marked = slices.Clone(notified)
	threshold = -1
	for _, t := range thresholds {
		if left > float64(t) || slices.Contains(notified, t) {
			continue
		}
		if threshold < 0 || t < threshold {
			threshold = t
		}
		marked = append(marked, t)
	}
	slices.Sort(marked)
	return threshold, threshold >= 0, slices.Compact(marked)
}

// sameExpiry reports whether two expiry times are the same certificate or
// registration (registries sometimes change the time of day).
func sameExpiry(a *time.Time, b time.Time) bool {
	if a == nil {
		return false
	}
	d := a.Sub(b)
	return d < 12*time.Hour && d > -12*time.Hour
}

// isDue reports whether a monitor should be checked at now. A few seconds of
// slack keep a 60 second monitor from slipping to every 75 seconds with a
// 15 second scheduler tick.
func isDue(last *time.Time, interval time.Duration, now time.Time) bool {
	if last == nil {
		return true
	}
	return !now.Before(last.Add(interval - 5*time.Second))
}

// resultPoint is one monitor result used by downsample.
type resultPoint struct {
	At         time.Time
	OK         bool
	StatusCode *int64
	LatencyMs  int64
	Error      string
	Detail     string
}

// maxPoints is the most points /monitors/{id}/results returns.
const maxPoints = 500

// downsample merges results into at most max buckets over [since, since+span).
// A bucket is ok only if every result in it was ok; latency is the average;
// the first error and the last status code and detail are kept. It returns
// the step in seconds, 0 when nothing was merged.
func downsample(points []resultPoint, since time.Time, span time.Duration, max int) ([]resultPoint, int) {
	if len(points) <= max {
		return points, 0
	}
	step := int(math.Ceil(span.Seconds() / float64(max)))
	if step < 1 {
		step = 1
	}
	var out []resultPoint
	var count int64
	var sum int64
	bucket := -1
	for _, p := range points {
		b := int(p.At.Sub(since).Seconds()) / step
		if b != bucket || len(out) == 0 {
			if len(out) > 0 && count > 0 {
				out[len(out)-1].LatencyMs = sum / count
			}
			bucket = b
			out = append(out, resultPoint{At: since.Add(time.Duration(b*step) * time.Second), OK: true})
			count, sum = 0, 0
		}
		cur := &out[len(out)-1]
		cur.OK = cur.OK && p.OK
		if cur.Error == "" {
			cur.Error = p.Error
		}
		cur.StatusCode = p.StatusCode
		cur.Detail = p.Detail
		sum += p.LatencyMs
		count++
	}
	if len(out) > 0 && count > 0 {
		out[len(out)-1].LatencyMs = sum / count
	}
	return out, step
}

// uptime returns the percent of ok results and the average latency.
func uptime(points []resultPoint) (float64, float64) {
	if len(points) == 0 {
		return 0, 0
	}
	ok, latSum := 0, int64(0)
	for _, p := range points {
		if p.OK {
			ok++
		}
		latSum += p.LatencyMs
	}
	return round2(float64(ok) / float64(len(points)) * 100), round2(float64(latSum) / float64(len(points)))
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// ---- scripts ----

// scriptCommand turns a script into the command for Hosts.Exec, which runs
// /bin/sh -c on Linux and PowerShell on Windows.
func scriptCommand(shell, body string) string {
	switch shell {
	case shellBash:
		return "bash -c " + shQuote(body)
	default: // sh on Linux, powershell on Windows: the agent's own shell
		return body
	}
}

// shQuote quotes s for a POSIX shell.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ---- subscriptions ----

// civil dates are time.Time values at midnight UTC, so date math is exact.
const dateLayout = "2006-01-02"

func parseDate(s string) (time.Time, error) { return time.Parse(dateLayout, s) }

// today is the user's current date.
func today(now time.Time, loc *time.Location) time.Time {
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// daysBetween counts days from a to b (both civil dates).
func daysBetween(a, b time.Time) int { return int(math.Round(b.Sub(a).Hours() / 24)) }

// addMonths adds n months and clamps the day to the end of the month, so
// January 31 plus one month is February 28 (or 29).
func addMonths(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	first := time.Date(y, m+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(d, last), 0, 0, 0, 0, time.UTC)
}

// advance moves a renewal date forward by one cycle.
func advance(t time.Time, cycle string, days int) time.Time {
	switch cycle {
	case cycleYearly:
		return addMonths(t, 12)
	case cycleCustom:
		return t.AddDate(0, 0, max(days, 1))
	default:
		return addMonths(t, 1)
	}
}

// renewUntil advances an auto-renewing date until it is not before day.
func renewUntil(next, day time.Time, cycle string, days int) time.Time {
	for i := 0; next.Before(day) && i < 10000; i++ {
		next = advance(next, cycle, days)
	}
	return next
}

// reminderHour is the local hour from which renewal reminders go out, so
// they do not arrive at midnight.
const reminderHour = 9

// reminderDue decides whether a renewal reminder is due with left days to
// go. reminded holds the thresholds already sent for this renewal date.
// Like expiryDue, one reminder covers every threshold it passed.
func reminderDue(left int, remind, reminded []int) (bool, []int) {
	if left < 0 {
		return false, reminded
	}
	due := false
	marked := slices.Clone(reminded)
	for _, t := range remind {
		if left > t || slices.Contains(reminded, t) {
			continue
		}
		due = true
		marked = append(marked, t)
	}
	slices.Sort(marked)
	return due, slices.Compact(marked)
}

// monthlyCost converts an amount per cycle to an amount per month.
func monthlyCost(amount float64, cycle string, days int) float64 {
	switch cycle {
	case cycleYearly:
		return amount / 12
	case cycleCustom:
		if days <= 0 {
			return 0
		}
		return amount * (365.25 / 12) / float64(days)
	default:
		return amount
	}
}
