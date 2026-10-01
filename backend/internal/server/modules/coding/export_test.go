package coding

import "time"

// Hooks for the integration tests in package coding_test, which cannot live
// in this package because testutil imports app, which imports coding.

// SetTimeoutUnit makes one "timeout minute" last d.
func (m *Module) SetTimeoutUnit(d time.Duration) { m.timeoutUnit = d }

// Running is the number of tasks with an open stream.
func (m *Module) Running() int { return m.running() }

// Dispatch starts queued tasks now.
func (m *Module) Dispatch() { m.dispatch() }

var (
	Slugify    = slugify
	BranchName = branchName
	GithubRepo = githubRepo
)

// ParseExecutorUsageForTest exposes the B42 executor usage parser.
func ParseExecutorUsageForTest(executor string, raw []byte) (input, cached, cacheWrite, output int64, cost *float64, ok bool) {
	u, ok := parseExecutorUsage(executor, raw)
	return u.input, u.cached, u.cacheWrite, u.output, u.cost, ok
}
