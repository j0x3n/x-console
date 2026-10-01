package ai

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/db"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief"
)

const keySetting = "ai.api_key"
const confirmSetting = "ai.confirm_all_writes"

type Module struct {
	d           *module.Deps
	q           *db.Queries
	llm         llm.Client
	mu          sync.Mutex
	reasonMu    sync.Mutex
	running     map[int64]*generation
	permissions map[int64]hostPermission
	// panelAll holds the panel conversations switched to "all" (B60), with
	// their last message. It is never stored: a restart goes back to manual.
	panelAll map[int64]time.Time
	// lastAgent is the agent a panel conversation last asked to operate a
	// machine (B60), picked again when several are bound.
	lastAgent   map[int64]int64
	actionLocks map[int64]*sync.Mutex // see lockActions
	now         func() time.Time
}
type hostPermission struct {
	mode        api.HostAgentPermission
	lastMessage time.Time
}
type generation struct{ cancel context.CancelFunc }

var _ api.ServerInterface = (*Module)(nil)

func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), running: map[int64]*generation{}, permissions: map[int64]hostPermission{}, panelAll: map[int64]time.Time{}, lastAgent: map[int64]int64{}, actionLocks: map[int64]*sync.Mutex{}, now: time.Now}
	m.llm = llm.New(m.resolveLLM, m.recordLLM, m.markReasoningUnsupported)
	module.Provide[brief.Polisher](d.Registry, brief.PolisherKey, m)
	module.Provide[contracts.LLM](d.Registry, contracts.LLMKey, m)
	module.Provide[contracts.AIUsageRecorder](d.Registry, contracts.AIUsageKey, m)
	module.Provide[contracts.ToolRunner](d.Registry, contracts.ToolRunnerKey, m) // B47
	module.Provide[contracts.Memories](d.Registry, contracts.MemoriesKey, m)     // B61
	// A second module on the same registry (tests) keeps the first one's actions.
	if _, ok := d.Actions.Get(context.Background(), "memory.save"); !ok {
		m.registerMemoryActions()
		m.registerOperateHost()
	}
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
