package contracts

import "context"

const GitIssuesKey = "aiagents.git_issues"
const BoardGitKey = "projects.git"
const BoardWebhookKey = "projects.git_webhook"

// GitRepository contains public repository metadata, never credentials.
type GitRepository struct {
	ConnectionID                                                     int64
	ConnectionName, Kind, FullName, HTMLURL, CloneURL, DefaultBranch string
}

type GitIssue struct {
	Number                  int64
	Title, Body, State, URL string
	Labels                  []string
}

// GitIssues is provided by aiagents so projects reuses its authenticated clients.
type GitIssues interface {
	Repository(context.Context, int64, string) (GitRepository, error)
	Issues(context.Context, int64, string) ([]GitIssue, error)
	UpdateGitIssue(context.Context, int64, string, GitIssue) error
}

// BoardGit resolves the repository of a card for coding agents.
type BoardGit interface {
	RepositoryForIssue(context.Context, string) (GitRepository, error)
}
