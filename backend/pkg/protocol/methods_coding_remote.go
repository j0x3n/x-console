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
