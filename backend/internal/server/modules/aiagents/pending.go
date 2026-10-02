package aiagents

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/api"
)

// These endpoints are in the contract but not built yet. See docs/specs/B86.md and B87.md.

func (m *Module) ListAiAgentRuns(w http.ResponseWriter, r *http.Request, params api.ListAiAgentRunsParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListAiAgentRunEvents(w http.ResponseWriter, r *http.Request, runID int64, params api.ListAiAgentRunEventsParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) CancelAiAgentRun(w http.ResponseWriter, r *http.Request, runID int64) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListAiAgentDecisions(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) AnswerAiAgentDecision(w http.ResponseWriter, r *http.Request, decisionID string) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) GetAiAgentNotify(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) PutAiAgentNotify(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
