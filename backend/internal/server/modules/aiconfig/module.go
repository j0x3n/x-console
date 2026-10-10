// Package aiconfig keeps one Claude Code and Codex configuration in the panel
// and writes it to the machines the user picks (B121). Only what the panel
// wrote is managed: the agent adds a marked block to the rules files and the
// permission rules and MCP servers it added itself, and never touches the
// user's own entries. See docs/specs/B121.md.
package aiconfig

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiconfig/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	docKey       = "aiconfig.doc"
	callTimeout  = 20 * time.Second
	maxHostCount = 50
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "aiconfig.module"

// Module implements api.ServerInterface.
type Module struct {
	d *module.Deps
}

var _ api.ServerInterface = (*Module)(nil)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d}
	m.registerActions()
	module.Provide[*Module](d.Registry, ServiceKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "aiconfig" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// stored is what is kept in the settings table.
type stored struct {
	Config    protocol.AIConfigDoc `json:"config"`
	HostIDs   []string             `json:"hostIds"`
	UpdatedAt time.Time            `json:"updatedAt"`
}

func (m *Module) load(ctx context.Context) (stored, error) {
	var s stored
	err := m.d.Settings.Get(ctx, docKey, &s)
	if err != nil && !errors.Is(err, settings.ErrNotSet) {
		return s, err
	}
	return s, nil
}

type host struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Online    bool   `json:"online"`
	Supported bool   `json:"supported"`
}

func (m *Module) hosts(ctx context.Context) ([]host, error) {
	agents, err := m.d.Agents.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]host, 0, len(agents))
	for _, a := range agents {
		out = append(out, host{ID: a.ID, Name: a.Name, Online: a.Online, Supported: a.Has(protocol.CapAIConfig)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// view is what GET and PUT /aiconfig answer.
type view struct {
	Claude    protocol.AIConfigTool `json:"claude"`
	Codex     protocol.AIConfigTool `json:"codex"`
	HostIDs   []string              `json:"hostIds"`
	Hosts     []host                `json:"hosts"`
	UpdatedAt *time.Time            `json:"updatedAt,omitempty"`
}

func lists(t protocol.AIConfigTool) protocol.AIConfigTool {
	if t.Allow == nil {
		t.Allow = []string{}
	}
	if t.Ask == nil {
		t.Ask = []string{}
	}
	if t.Deny == nil {
		t.Deny = []string{}
	}
	if t.MCP == nil {
		t.MCP = []protocol.AIConfigMCP{}
	}
	return t
}

func (m *Module) view(ctx context.Context, s stored) (view, error) {
	hosts, err := m.hosts(ctx)
	if err != nil {
		return view{}, err
	}
	known := map[string]bool{}
	for _, h := range hosts {
		known[h.ID] = true
	}
	ids := []string{}
	for _, id := range s.HostIDs {
		if known[id] {
			ids = append(ids, id)
		}
	}
	v := view{Claude: lists(s.Config.Claude), Codex: lists(s.Config.Codex), HostIDs: ids, Hosts: hosts}
	if !s.UpdatedAt.IsZero() {
		at := s.UpdatedAt
		v.UpdatedAt = &at
	}
	return v, nil
}

// GetAIConfig implements api.ServerInterface.
func (m *Module) GetAIConfig(w http.ResponseWriter, r *http.Request) {
	s, err := m.load(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := m.view(r.Context(), s)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

// SaveAIConfig implements api.ServerInterface.
func (m *Module) SaveAIConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body struct {
		Claude  protocol.AIConfigTool `json:"claude"`
		Codex   protocol.AIConfigTool `json:"codex"`
		HostIDs []string              `json:"hostIds"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	doc := protocol.AIConfigDoc{Claude: body.Claude, Codex: body.Codex}.Normalize()
	if err := doc.Validate(); err != nil {
		httpx.Fail(w, r, httpx.Invalid(err.Error()))
		return
	}
	hosts, err := m.hosts(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	known := map[string]bool{}
	for _, h := range hosts {
		known[h.ID] = true
	}
	ids := slices.Clone(body.HostIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) > maxHostCount {
		httpx.Fail(w, r, httpx.Invalid("最多选 50 台机器"))
		return
	}
	for _, id := range ids {
		if !known[id] {
			httpx.Fail(w, r, httpx.Invalid("机器 "+id+" 不存在"))
			return
		}
	}
	s := stored{Config: doc, HostIDs: ids, UpdatedAt: time.Now().UTC()}
	err = m.d.Settings.Set(ctx, docKey, s)
	m.d.Audit.Record(ctx, "aiconfig.save", "", map[string]any{"hosts": len(ids), "claudeServers": len(doc.Claude.MCP), "codexServers": len(doc.Codex.MCP)}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("aiconfig.updated", map[string]any{})
	v, err := m.view(ctx, s)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

// item and hostStatus are what the status and apply endpoints answer.
type item struct {
	Tool    string   `json:"tool"`
	Item    string   `json:"item"`
	State   string   `json:"state"`
	Reason  string   `json:"reason,omitempty"`
	Names   []string `json:"names,omitempty"`
	Changed bool     `json:"changed,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type hostStatus struct {
	HostID    string    `json:"hostId"`
	Name      string    `json:"name"`
	State     string    `json:"state"`
	Items     []item    `json:"items"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

// overall folds the items of one machine into one state.
func overall(items []item) string {
	state, all := protocol.AIConfigOK, true
	for _, it := range items {
		if it.Error != "" {
			return "error"
		}
		if it.State != protocol.AIConfigAbsent {
			all = false
		}
		switch {
		case it.State == protocol.AIConfigConflict:
			state = protocol.AIConfigConflict
		case it.State == protocol.AIConfigDrift && state != protocol.AIConfigConflict:
			state = protocol.AIConfigDrift
		}
	}
	if all && len(items) > 0 {
		return protocol.AIConfigAbsent
	}
	return state
}

func (m *Module) check(ctx context.Context, h host, doc protocol.AIConfigDoc, apply bool) hostStatus {
	st := hostStatus{HostID: h.ID, Name: h.Name, Items: []item{}, CheckedAt: time.Now().UTC()}
	switch {
	case !h.Online:
		st.State = "offline"
		return st
	case !h.Supported:
		st.State = "unsupported"
		return st
	}
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	var out protocol.AIConfigSyncResult
	if err := m.d.Agents.Call(cctx, h.ID, protocol.MethodAIConfigSync, protocol.AIConfigSyncParams{Config: doc, Apply: apply}, &out); err != nil {
		st.State, st.Error = "error", err.Error()
		return st
	}
	for _, it := range out.Items {
		st.Items = append(st.Items, item(it))
	}
	st.State = overall(st.Items)
	return st
}

// checkAll runs check on every machine at the same time.
func (m *Module) checkAll(ctx context.Context, ids []string, doc protocol.AIConfigDoc, apply bool) ([]hostStatus, error) {
	hosts, err := m.hosts(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]host{}
	for _, h := range hosts {
		byID[h.ID] = h
	}
	var picked []host
	for _, id := range ids {
		if h, ok := byID[id]; ok {
			picked = append(picked, h)
		}
	}
	out := make([]hostStatus, len(picked))
	var wg sync.WaitGroup
	for i, h := range picked {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = m.check(ctx, h, doc, apply)
		}()
	}
	wg.Wait()
	return out, nil
}

// GetAIConfigStatus implements api.ServerInterface.
func (m *Module) GetAIConfigStatus(w http.ResponseWriter, r *http.Request) {
	res, err := m.status(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"hosts": res})
}

func (m *Module) status(ctx context.Context) ([]hostStatus, error) {
	s, err := m.load(ctx)
	if err != nil {
		return nil, err
	}
	return m.checkAll(ctx, s.HostIDs, s.Config, false)
}

// ApplyAIConfig implements api.ServerInterface.
func (m *Module) ApplyAIConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body struct {
		HostIDs *[]string `json:"hostIds"`
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<16))
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid("读取请求体失败"))
		return
	}
	if len(strings.TrimSpace(string(raw))) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			httpx.Fail(w, r, httpx.Invalid("请求体格式不正确: "+err.Error()))
			return
		}
	}
	s, err := m.load(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ids := s.HostIDs
	if body.HostIDs != nil {
		ids = []string{}
		for _, id := range *body.HostIDs {
			if !slices.Contains(s.HostIDs, id) {
				httpx.Fail(w, r, httpx.Invalid("机器 "+id+" 没有选中下发，请先选中并保存"))
				return
			}
			ids = append(ids, id)
		}
	}
	res, err := m.checkAll(ctx, ids, s.Config, true)
	if err == nil {
		results := map[string]any{}
		for _, h := range res {
			changed := []string{}
			for _, it := range h.Items {
				if it.Changed {
					changed = append(changed, it.Tool+"/"+it.Item)
				}
			}
			results[h.Name] = map[string]any{"state": h.State, "changed": changed}
		}
		m.d.Audit.Record(ctx, "aiconfig.apply", strings.Join(ids, ","), map[string]any{"hosts": results}, nil)
		m.d.Bus.Publish("aiconfig.updated", map[string]any{})
	} else {
		m.d.Audit.Record(ctx, "aiconfig.apply", strings.Join(ids, ","), nil, err)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"hosts": res})
}
