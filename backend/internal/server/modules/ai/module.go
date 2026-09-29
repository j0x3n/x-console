package ai

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/db"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief"
)

const defaultModel = "claude-opus-5-5"
const keySetting = "ai.api_key"
const modelSetting = "ai.model"
const confirmSetting = "ai.confirm_all_writes"

type Module struct {
	d        *module.Deps
	q        *db.Queries
	llm      llm.Client
	baseURL  string
	mu       sync.Mutex
	reasonMu sync.Mutex
	running  map[int64]*generation
}
type generation struct{ cancel context.CancelFunc }

var _ api.ServerInterface = (*Module)(nil)

func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), baseURL: os.Getenv("XC_ANTHROPIC_BASE_URL"), running: map[int64]*generation{}}
	m.llm = llm.New(m.resolveLLM, m.recordLLM, m.markReasoningUnsupported)
	module.Provide[brief.Polisher](d.Registry, brief.PolisherKey, m)
	return m, nil
}
func (m *Module) Name() string { return "ai" }
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}
func (m *Module) fail(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	httpx.Fail(w, r, err)
	return true
}
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.ErrNotFound
	}
	return err
}
