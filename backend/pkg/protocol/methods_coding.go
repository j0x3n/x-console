package protocol

import (
	"encoding/json"
	"time"
)

// Methods of M4 coding tasks. See docs/modules/M4.md.
//
// A task runs in a git worktree at <repo>/.x-console/worktrees/<taskId> on
// its own branch. The agent keeps no state between calls: every method gets
// the repository path and the task id and derives the worktree from them.
const (
	MethodCodingExecutors = "coding.executors" // nil -> CodingExecutorList
	MethodCodingRepos     = "coding.repos"     // CodingReposParams -> CodingRepoList

	// MethodCodingRun is a stream with params CodingRunParams. Every chunk
	// from the agent is one JSON CodingEvent followed by a newline. The last
	// one has Kind CodingEventDone. The server may send one JSON
	// CodingControl chunk to cancel the run.
	MethodCodingRun = "coding.run"

	MethodCodingDiff    = "coding.diff"    // CodingTaskParams -> CodingDiff
	MethodCodingCommit  = "coding.commit"  // CodingCommitParams -> CodingCommitResult
	MethodCodingPush    = "coding.push"    // CodingPushParams -> nil
	MethodCodingDiscard = "coding.discard" // CodingTaskParams -> nil
)

// Executors.
const (
	ExecutorClaude = "claude"
	ExecutorCodex  = "codex"
)

// CodingExecutor is one executor as detected on the agent.
type CodingExecutor struct {
	Name      string `json:"name"` // claude or codex
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"` // why it is not available
}

// CodingExecutorList answers MethodCodingExecutors.
type CodingExecutorList struct {
	Items []CodingExecutor `json:"items"`
}

// CodingReposParams scans for git repositories. Empty Roots means the roots
// from the agent config (default: code, projects and src in the home
// directory). Depth 0 means the configured depth (default 3); a negative
// depth only looks at the roots themselves, which is how a single path is
// checked before it is registered.
type CodingReposParams struct {
	Roots []string `json:"roots,omitempty"`
	Depth int      `json:"depth,omitempty"`
}

// CodingRepo is one git repository found on the agent.
type CodingRepo struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	CurrentBranch string `json:"currentBranch"`
	DefaultBranch string `json:"defaultBranch"`
	RemoteURL     string `json:"remoteUrl"`
}

// CodingRepoList answers MethodCodingRepos.
type CodingRepoList struct {
	Items []CodingRepo `json:"items"`
	Roots []string     `json:"roots"` // the roots that were scanned
}

// CodingRunParams starts a task. BaseBranch empty means the repository's
// current HEAD. Branch is created from it; it must not exist yet.
type CodingRunParams struct {
	TaskID         int64  `json:"taskId"`
	RepoPath       string `json:"repoPath"`
	Executor       string `json:"executor"`
	Prompt         string `json:"prompt"`
	BaseBranch     string `json:"baseBranch,omitempty"`
	Branch         string `json:"branch"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"` // 0 means 60 minutes
}

// Event kinds of a coding.run stream.
const (
	CodingEventText   = "text"   // assistant text, or stderr output (Data.stream = "stderr")
	CodingEventTool   = "tool"   // a tool call (Data.id, name, input) or its result (Data.id, result: true)
	CodingEventStatus = "status" // progress: Data.code is session_started, worktree_ready, result, ...
	CodingEventError  = "error"  // an error reported by the executor or the agent
	CodingEventDone   = "done"   // last event: ExitCode and Data CodingDone
)

// CodingEvent is one normalized executor output event.
type CodingEvent struct {
	Kind     string          `json:"kind"`
	Text     string          `json:"text,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	ExitCode *int            `json:"exitCode,omitempty"` // only on done
	At       time.Time       `json:"at"`
}

// How a run ended (CodingDone.Reason).
const (
	CodingEndExited      = "exited"       // the executor exited by itself
	CodingEndTimeout     = "timeout"      // killed after TimeoutSeconds
	CodingEndCanceled    = "canceled"     // the server sent CodingControl{Op: "cancel"}
	CodingEndStartFailed = "start_failed" // worktree or executor could not start
)

// CodingDone is the Data of the done event. The worktree and branch are
// kept when Reason is exited, and removed otherwise.
type CodingDone struct {
	Reason     string              `json:"reason"`
	BaseCommit string              `json:"baseCommit,omitempty"`
	Files      []CodingChangedFile `json:"files,omitempty"`
	Error      string              `json:"error,omitempty"`
}

// CodingControl is sent by the server on a coding.run stream.
type CodingControl struct {
	Op string `json:"op"` // "cancel"
}

// CodingTaskParams names a task's worktree and branch. BaseCommit is the
// commit the branch started from (reported in the worktree_ready status
// event and in CodingDone); empty means the merge base with BaseBranch.
type CodingTaskParams struct {
	TaskID     int64  `json:"taskId"`
	RepoPath   string `json:"repoPath"`
	Branch     string `json:"branch"`
	BaseBranch string `json:"baseBranch,omitempty"`
	BaseCommit string `json:"baseCommit,omitempty"`
}

// CodingChangedFile is one changed path. Status is A, M, D or T.
type CodingChangedFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary,omitempty"`
}

// CodingDiffMax caps CodingDiff.Diff.
const CodingDiffMax = 2 << 20

// CodingDiff answers MethodCodingDiff: all changes of the task branch and
// its worktree (including untracked files) against the base commit.
type CodingDiff struct {
	Files     []CodingChangedFile `json:"files"`
	Diff      string              `json:"diff"`
	Truncated bool                `json:"truncated"`
}

// CodingCommitParams commits everything in the worktree, then removes the
// worktree and keeps the branch.
type CodingCommitParams struct {
	CodingTaskParams
	Message string `json:"message"`
}

// CodingCommitResult answers MethodCodingCommit.
type CodingCommitResult struct {
	SHA string `json:"sha"`
}

// CodingPushParams pushes the task branch. Remote empty means origin.
type CodingPushParams struct {
	CodingTaskParams
	Remote string `json:"remote,omitempty"`
}
