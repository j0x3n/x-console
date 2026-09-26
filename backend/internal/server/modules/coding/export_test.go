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
