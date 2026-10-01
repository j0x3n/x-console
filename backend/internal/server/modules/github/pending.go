package github

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// These endpoints are in the contract but not built yet. See docs/specs/B70.md and B71.md.

func (m *Module) GetGitHubNotify(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) PutGitHubNotify(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
