// Package knowledge answers questions from the user's own material (B139).
// Notes, saved articles, the diary and review cards are cut into passages and
// turned into vectors by the AI provider's embedding endpoint. A question is
// matched against the passages, and the AI answers from the best ones with
// links to the sources. Off by default. See docs/specs/B139.md.
package knowledge

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/knowledge/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/knowledge/db"
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "knowledge.module"

const (
	syncEvery      = 10 * time.Minute
	embedBudget    = 500 // passages embedded per sync
	maxQuestion    = 500 // runes
	topK           = 8
	perDocLimit    = 2
	minScore       = 0.2
	pageSize       = 50
	embedBatchSize = 64
)

// AllSources are the sources the module can index, in display order.
var AllSources = []string{"notes", "readlater", "journal", "cards"}

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time
	// budget is how many passages one sync embeds.
	budget int

	mu      sync.Mutex
	syncing bool
	last    time.Time
	lastErr string
	pending int
	cache   []vec // nil when it has to be loaded again
	cached  bool
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now, budget: embedBudget}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "knowledge" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the sync.
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("knowledge.sync", syncEvery, func(ctx context.Context) error {
		m.sync(ctx)
		return nil
	})
	return nil
}

func (m *Module) now() time.Time { return m.nowFn() }

func fail(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	httpx.Fail(w, r, err)
	return true
}

func ptr[T any](v T) *T { return &v }

func (m *Module) embedder() (contracts.Embedder, bool) {
	return module.Lookup[contracts.Embedder](m.d.Registry, contracts.EmbedderKey)
}

// noProvider is the message when the embedding provider is missing.
func noProvider(o api.KnowledgeSettings) string {
	if providerID(o) > 0 {
		return "嵌入用的 AI 供应商已经删除，请在知识库设置里重新选"
	}
	return "还没有配置 AI 供应商，不能建索引"
}

// sources returns the registered sources by name.
func (m *Module) sources() map[string]contracts.KnowledgeSource {
	out := map[string]contracts.KnowledgeSource{}
	for key, src := range module.All[contracts.KnowledgeSource](m.d.Registry) {
		if name, ok := strings.CutPrefix(key, contracts.KnowledgeSourcePrefix); ok && name != "" {
			out[name] = src
		}
	}
	return out
}

func (m *Module) hiddenFn(ctx context.Context) func(string) bool {
	h, _ := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
	cache := map[string]bool{}
	return func(name string) bool {
		if h == nil || name == "" {
			return false
		}
		if v, ok := cache[name]; ok {
			return v
		}
		v := h.Hidden(ctx, name)
		cache[name] = v
		return v
	}
}

func sourceEnabled(opts api.KnowledgeSettings, name string) bool {
	return slices.Contains(opts.Sources, name)
}

// ---------- status ----------

// GetKnowledgeStatus implements api.ServerInterface.
func (m *Module) GetKnowledgeStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := m.q.KnowledgeCounts(ctx)
	if fail(w, r, err) {
		return
	}
	st := api.KnowledgeStatus{Sources: []api.KnowledgeSourceCount{}}
	byName := map[string]api.KnowledgeSourceCount{}
	for _, row := range rows {
		byName[row.Source] = api.KnowledgeSourceCount{Source: row.Source, Docs: int(row.Docs), Chunks: int(row.Chunks)}
		st.Docs += int(row.Docs)
		st.Chunks += int(row.Chunks)
	}
	for _, name := range AllSources {
		if c, ok := byName[name]; ok {
			st.Sources = append(st.Sources, c)
		}
	}
	opts := m.options(ctx)
	st.Enabled = opts.Enabled
	if em, ok := m.embedder(); ok {
		st.Available = em.Available(ctx, providerID(opts))
	}
	m.mu.Lock()
	st.Syncing = m.syncing
	if !m.last.IsZero() {
		st.LastSyncAt = ptr(m.last)
	}
	if m.lastErr != "" {
		st.LastError = ptr(m.lastErr)
	}
	if m.pending > 0 {
		st.Pending = ptr(m.pending)
	}
	m.mu.Unlock()
	httpx.JSON(w, http.StatusOK, st)
}

// SyncKnowledge implements api.ServerInterface.
func (m *Module) SyncKnowledge(w http.ResponseWriter, r *http.Request) {
	if !m.options(r.Context()).Enabled {
		httpx.Fail(w, r, httpx.Invalid("知识库还没开启"))
		return
	}
	go m.sync(context.WithoutCancel(r.Context()))
	w.WriteHeader(http.StatusAccepted)
}

// ClearKnowledgeIndex implements api.ServerInterface.
func (m *Module) ClearKnowledgeIndex(w http.ResponseWriter, r *http.Request) {
	err := m.q.ClearKnowledgeDocs(r.Context())
	m.d.Audit.Record(r.Context(), "knowledge.clear", "", nil, err)
	if fail(w, r, err) {
		return
	}
	m.invalidate()
	m.mu.Lock()
	m.pending, m.lastErr = 0, ""
	m.mu.Unlock()
	httpx.NoContent(w)
}

func (m *Module) invalidate() {
	m.mu.Lock()
	m.cache, m.cached = nil, false
	m.mu.Unlock()
}
