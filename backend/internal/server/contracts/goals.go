package contracts

import (
	"context"
	"errors"
	"time"
)

// GoalMetricPrefix is the registry key prefix of the modules that give the
// goals module (B128) numbers to measure progress with. The last part of the
// key is the module name, for example "goals.metrics.habits".
const GoalMetricPrefix = "goals.metrics."

// Goal metric kinds.
const (
	GoalMetricHabitCheckins   = "habit_checkins"    // habits: check-ins of one habit
	GoalMetricWorkouts        = "workouts"          // habits: workout logs
	GoalMetricFocusMinutes    = "focus_minutes"     // focus: minutes of finished sessions
	GoalMetricScreenMinutes   = "screen_minutes"    // screentime: minutes, optionally one category
	GoalMetricGitHubCommits   = "github_commits"    // github: own commits
	GoalMetricGitHubMergedPRs = "github_merged_prs" // github: own merged pull requests
)

// Kinds of the choice a metric needs.
const (
	GoalRefNone     = "none"
	GoalRefHabit    = "habit"
	GoalRefCategory = "category"
)

// GoalMetricKind describes one number a module can measure.
type GoalMetricKind struct {
	Kind string
	// Module is the hideable module the numbers come from (habits,
	// screentime, github, ...). While it is hidden and locked the goals
	// module does not read the metric.
	Module string
	// RefKind says what the user has to choose: GoalRefNone, GoalRefHabit or
	// GoalRefCategory.
	RefKind string
}

// GoalMetricOption is one choice for a metric, such as a habit.
type GoalMetricOption struct {
	Value string
	Label string
}

// ErrGoalMetricMissing is what Measure returns when the data behind a metric
// is not set up, such as a GitHub account that is not connected. The goals
// module shows it as "no data" instead of zero.
var ErrGoalMetricMissing = errors.New("goal metric: source not available")

// GoalMetricSource is offered by modules whose data can be a key result. It
// does not check hidden modules and does not depend on the goals module.
type GoalMetricSource interface {
	Kinds() []GoalMetricKind
	// Options lists what the user can choose for kind. Empty when the kind
	// needs no choice.
	Options(ctx context.Context, kind string) ([]GoalMetricOption, error)
	// Measure adds up kind over [from, until). ref is one of the option
	// values, or empty.
	Measure(ctx context.Context, kind, ref string, from, until time.Time) (float64, error)
}
