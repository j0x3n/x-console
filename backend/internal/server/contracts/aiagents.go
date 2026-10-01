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

// IssueWorkKey is provided by M5 for B47 agents working on cards.
const IssueWorkKey = "projects.work"

// IssueBrief is what an agent is told about a card.
type IssueBrief struct {
	Key         string
	Title       string
	Description string
	Status      string
	OpenItems   []string // unchecked checklist items
	Comments    []string // the last 10 comments, oldest first
}

// IssueWork is provided by M5 (B47).
type IssueWork interface {
	Brief(ctx context.Context, key string) (IssueBrief, error)
	// Comment adds a comment; author is "agent:<id>" for an agent.
	Comment(ctx context.Context, key, author, body string) error
	// AddMember adds a member (kind "agent", id the agent id) if missing.
	AddMember(ctx context.Context, key, kind, id string) error
}

// ToolRunnerKey is provided by M12 for B47 built-in agents.
const ToolRunnerKey = "ai.tools"

// ToolRun is one job of a built-in agent.
type ToolRun struct {
	Model    string // "<provider id>:<model id>" of the B32 providers
	System   string
	Prompt   string
	Access   string // read, write or write_delete: the same filter as B43 tokens
	MaxTurns int    // 0 means 20
	Source   string // usage record source and ref (B42)
	Ref      string
}

// ToolRunner is provided by M12 (B47): it lets a model call the actions
// the access level allows until it answers without a tool call.
type ToolRunner interface {
	RunTools(ctx context.Context, in ToolRun) (string, error)
}
