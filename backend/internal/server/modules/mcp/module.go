// Package mcp is B43: API tokens and the MCP endpoint that lets outside AI
// clients (Claude Code, Codex CLI, Cursor…) call the actions registry.
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	coredb "github.com/j0x3n/x-console/backend/internal/server/core/db"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mcp/api"
)

// Module implements api.ServerInterface and serves POST /mcp.
type Module struct {
	d *module.Deps
}

var _ api.ServerInterface = (*Module)(nil)

// New builds the module.
func New(d *module.Deps) (module.Module, error) { return &Module{d: d}, nil }

// Name implements module.Module.
func (m *Module) Name() string { return "mcp" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
	r.Post("/mcp", m.serveMCP)
	r.Get("/mcp", func(w http.ResponseWriter, _ *http.Request) {
		// Stateless server: no event stream to open.
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
}

func toAPIToken(t coredb.ApiToken) api.ApiToken {
	var modules []string
	_ = json.Unmarshal([]byte(t.Modules), &modules)
	if modules == nil {
		modules = []string{}
	}
	out := api.ApiToken{Id: t.ID, Name: t.Name, Prefix: t.Prefix, Access: api.ApiTokenAccess(t.Access),
		Modules: modules, ExpiresAt: t.ExpiresAt, CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, RevokedAt: t.RevokedAt}
	if t.LastUsedIp != "" {
		ip := t.LastUsedIp
		out.LastUsedIp = &ip
	}
	return out
}

func (m *Module) ListApiTokens(w http.ResponseWriter, r *http.Request) {
	rows, err := m.d.Auth.ListTokens(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.ApiToken, 0, len(rows))
	for _, t := range rows {
		out = append(out, toAPIToken(t))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateApiToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string   `json:"name"`
		Access        string   `json:"access"`
		Modules       []string `json:"modules"`
		ExpiresInDays *int     `json:"expiresInDays"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	known := map[string]bool{}
	for _, mod := range m.modules(r.Context()) {
		known[mod] = true
	}
	for _, mod := range body.Modules {
		if !known[mod] {
			httpx.Fail(w, r, httpx.Invalid("没有这个模块: "+mod))
			return
		}
	}
	var expires *time.Time
	if body.ExpiresInDays != nil {
		if *body.ExpiresInDays < 1 || *body.ExpiresInDays > 3650 {
			httpx.Fail(w, r, httpx.Invalid("有效期在 1 到 3650 天之间"))
			return
		}
		t := time.Now().UTC().AddDate(0, 0, *body.ExpiresInDays)
		expires = &t
	}
	tok, err := m.d.Auth.CreateToken(r.Context(), body.Name, body.Access, body.Modules, expires)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"token": toAPIToken(tok.Row), "secret": tok.Secret})
}

func (m *Module) RevokeApiToken(w http.ResponseWriter, r *http.Request, id int64) {
	if err := m.d.Auth.RevokeToken(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// modules lists the modules that have actions an outside caller could use.
func (m *Module) modules(ctx context.Context) []string {
	seen := map[string]bool{}
	for _, a := range m.d.Actions.ListForMCP(ctx) {
		if a.Effect != actions.Dangerous && !a.PanelOnly {
			seen[actions.Module(a.Name)] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (m *Module) ListApiTokenTools(w http.ResponseWriter, r *http.Request, params api.ListApiTokenToolsParams) {
	var mods []string
	if params.Modules != nil {
		mods = *params.Modules
	}
	type tool struct {
		Name    string `json:"name"`
		Title   string `json:"title"`
		Effect  string `json:"effect"`
		Deletes bool   `json:"deletes,omitempty"`
	}
	tools := []tool{}
	for _, a := range m.d.Actions.ListForMCP(r.Context()) {
		if actions.AllowedFor(a, string(params.Access), mods) {
			tools = append(tools, tool{toolName(a.Name), a.Title, string(a.Effect), actions.Deletes(a)})
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tools": tools, "modules": m.modules(r.Context())})
}

func (m *Module) ListApiTokenCalls(w http.ResponseWriter, r *http.Request) {
	if auth.TokenFrom(r.Context()) != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	rows, err := m.d.DB.QueryContext(r.Context(), `SELECT id, at, actor, target, result FROM audit_log
 WHERE actor LIKE 'token:%' AND action = 'mcp.call' ORDER BY id DESC LIMIT 50`)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	type call struct {
		ID     int64     `json:"id"`
		At     time.Time `json:"at"`
		Token  string    `json:"token"`
		Tool   string    `json:"tool"`
		Result string    `json:"result"`
	}
	out := []call{}
	for rows.Next() {
		var c call
		var actor string
		if err := rows.Scan(&c.ID, &c.At, &actor, &c.Tool, &c.Result); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		c.Token = strings.TrimPrefix(actor, "token:")
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// toolName turns notes.create into notes_create: some clients do not accept
// dots in tool names.
func toolName(action string) string { return strings.ReplaceAll(action, ".", "_") }
