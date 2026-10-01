package protocol

// B47: repositories registered from a Git connection, task options of AI
// agents and builds. See docs/specs/B47.md.

// CapCodingRemote: the agent has coding.ensure_repo and understands the B47
// fields of CodingRunParams and CodingPushParams.
const CapCodingRemote = "coding.remote"

// MethodCodingEnsureRepo clones a remote repository into the agent's
// repository folder, or fetches when it is there already.
// CodingEnsureRepoParams -> CodingRepo.
const MethodCodingEnsureRepo = "coding.ensure_repo"

// CodingGitAuth lets git reach a private remote during one call. The agent
// hands it to git through the environment (GIT_CONFIG_*), never on the
// command line or on disk, and replaces it with *** in messages.
type CodingGitAuth struct {
	Username string `json:"username"`
	Token    string `json:"token"`
}

// CodingEnsureRepoParams names the clone. Dir is relative to the agent's
// repository folder, slash separated, for example "3/team/x-console".
type CodingEnsureRepoParams struct {
	Dir      string         `json:"dir"`
	CloneURL string         `json:"cloneUrl"`
	Auth     *CodingGitAuth `json:"auth,omitempty"`
}

// Values of CodingRunParams.Permission.
const (
	CodingPermissionWorkspace = "workspace" // may change files in the worktree only (default)
	CodingPermissionFull      = "full"      // no sandbox, no approval prompts
)

// MethodCodingBuild runs build steps in a task's worktree (B47, needs
// CapCodingRemote). It is a stream like coding.run: params
// CodingBuildParams, every chunk from the agent is one JSON CodingEvent and
// a newline, the last has Kind CodingEventDone and Data CodingBuildDone.
// Status events: code "build_step" {index, name, command} when a step
// starts, "build_step_done" {index, exitCode, durationMs} when it ends.
// Text events carry {stream: "stdout"|"stderr", index}. The server may send
// CodingControl{Op: "cancel"}.
const MethodCodingBuild = "coding.build"

// CodingBuildStep is one command. Artifacts are glob patterns relative to
// the worktree; "dir/**" means every file below dir.
type CodingBuildStep struct {
	Name           string   `json:"name"`
	Command        string   `json:"command"`
	TimeoutSeconds int      `json:"timeoutSeconds,omitempty"` // 0 means 30 minutes
	Artifacts      []string `json:"artifacts,omitempty"`
}

// CodingBuildParams names the worktree (it must exist: the task is in
// review) and the steps, run in order until one fails.
type CodingBuildParams struct {
	CodingTaskParams
	Steps []CodingBuildStep `json:"steps"`
}

// How a build ended (CodingBuildDone.Reason).
const (
	CodingBuildPassed   = "passed"
	CodingBuildFailed   = "failed"   // a step exited with a non-zero code
	CodingBuildTimeout  = "timeout"  // a step ran longer than its timeout
	CodingBuildCanceled = "canceled" // cancel chunk or stream closed
	CodingBuildError    = "error"    // the build could not start
)

// Build limits.
const (
	CodingArtifactMaxFiles = 50
	CodingArtifactMaxSize  = 2 << 30
)

// CodingArtifact is one file a build produced. Path is absolute on the
// agent (read it with files.read), Name relative to the worktree.
type CodingArtifact struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// CodingBuildDone is the Data of the last event of a build.
type CodingBuildDone struct {
	Reason     string           `json:"reason"`
	FailedStep int              `json:"failedStep"` // index, -1 when no step failed
	Artifacts  []CodingArtifact `json:"artifacts,omitempty"`
	Error      string           `json:"error,omitempty"`
}
