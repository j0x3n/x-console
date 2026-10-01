package github

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
)

// These endpoints are in the contract but not built yet. See docs/specs/B70.md and B71.md.

func (m *Module) ListGitHubWatchedRepos(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListGitHubCommits(w http.ResponseWriter, r *http.Request, params api.ListGitHubCommitsParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListGitHubRunJobs(w http.ResponseWriter, r *http.Request, runID int64, params api.ListGitHubRunJobsParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) GetGitHubNotify(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) PutGitHubNotify(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
