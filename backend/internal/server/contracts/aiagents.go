package contracts

import "context"

// ---- B47 AI agents ----

// Registry keys of B47.
const (
	GitConnectionsKey    = "aiagents.git"       // B47 aiagents provides, coding uses
	AIAgentsKey          = "aiagents.agents"    // B47 aiagents provides, coding uses
	GitHubCredentialsKey = "github.credentials" // M13 provides, B47 uses
)

// GitHubCredentials is provided by M13 so a B47 Git connection can reuse
// the token already set up in the GitHub module.
type GitHubCredentials interface {
	// Credentials returns the API address and token, or
	// httpx.ErrIntegrationMissing when no token is set.
	Credentials(ctx context.Context) (apiURL, token string, err error)
}

// GitConnections is provided by B47 aiagents.
type GitConnections interface {
	// CloneAuth returns the user name and token an agent uses to clone and
	// push. Never log or store them.
	CloneAuth(ctx context.Context, connectionID int64) (username, token string, err error)
	// CreatePR opens a pull request on GitHub or Forgejo. in.Repo is
	// "owner/name".
	CreatePR(ctx context.Context, connectionID int64, in CreatePR) (url string, number int, err error)
}

// AIAgent is a B47 agent as the coding module needs it.
type AIAgent struct {
	ID            int64
	Name          string
	Kind          string // claude_code, codex, builtin
	Model         string
	Instructions  string
	RunnerAgentID string // empty when not set
	CLIPermission string // workspace, full
	RepoIDs       []int64
	MaxParallel   int
	AutoBuild     bool
	BuildRetries  int
	Enabled       bool
	OverBudget    bool // this month's cost reached the budget
}

// AIAgents is provided by B47 aiagents.
type AIAgents interface {
	Get(ctx context.Context, id int64) (AIAgent, error)
}
