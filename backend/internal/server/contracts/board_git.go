package contracts

import "context"

const GitIssuesKey = "aiagents.git_issues"
const BoardGitKey = "projects.git"
const BoardWebhookKey = "projects.git_webhook"
const BoardGitUnbindKey = "projects.git_unbind"

// GitRepository contains public repository metadata, never credentials.
type GitRepository struct {
	ConnectionID                                                     int64
	ConnectionName, Kind, FullName, HTMLURL, CloneURL, DefaultBranch string
}

type GitIssue struct {
	Number                  int64
	Title, Body, State, URL string
	Labels                  []string
	PullRequest             bool
}

// GitIssues is provided by aiagents so projects reuses its authenticated clients.
type GitIssues interface {
	Repository(context.Context, int64, string) (GitRepository, error)
	Issues(context.Context, int64, string) ([]GitIssue, error)
	// Issue reads one issue, for webhooks. PullRequest is set when the
	// number belongs to a pull request.
	Issue(context.Context, int64, string, int64) (GitIssue, error)
	UpdateGitIssue(context.Context, int64, string, GitIssue) error
}

// BoardGit resolves the repository of a card for coding agents.
type BoardGit interface {
	RepositoryForIssue(context.Context, string) (GitRepository, error)
}

// BoardGitUnbinder is called before a Git connection is deleted, so boards
// bound to it lose the binding instead of blocking the delete.
type BoardGitUnbinder interface {
	UnbindGitConnection(context.Context, int64) error
}
