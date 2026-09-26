package homeassistant

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/homeassistant/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/homeassistant/db"
)

func (m *Module) GetHAConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := m.loadConfig(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, configToAPI(cfg))
}

func configToAPI(cfg config) api.HAConfig {
	return api.HAConfig{Url: cfg.URL, Mode: cfg.mode(), AgentId: cfg.AgentID, HasToken: cfg.Token != "", Token: maskToken(cfg.Token)}
}

// PutHAConfig saves the config. It can send the stored token to a new
// address, so it always needs elevation.
func (m *Module) PutHAConfig(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.HAConfigInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cfg, err := m.saveConfig(r.Context(), body)
	m.d.Audit.Record(r.Context(), "ha.config", cfg.URL, map[string]any{"mode": string(body.Mode), "tokenChanged": body.Token != nil && *body.Token != ""}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.c.Reload()
	httpx.JSON(w, http.StatusOK, configToAPI(cfg))
}

// resolveInput merges form values with the stored token.
func (m *Module) resolveInput(ctx context.Context, in api.HAConfigInput) (config, error) {
	old, err := m.loadConfig(ctx)
	if err != nil {
		return config{}, err
	}
	u, err := normalizeURL(in.Url)
	if err != nil {
		return config{}, err
	}
	cfg := config{URL: u, Token: old.Token}
	if in.Token != nil && strings.TrimSpace(*in.Token) != "" {
		cfg.Token = strings.TrimSpace(*in.Token)
	}
	if cfg.Token == "" {
		return config{}, httpx.Invalid("请填写长期访问令牌")
	}
	switch in.Mode {
	case api.Direct:
	case api.Agent:
		agentID := ""
		if in.AgentId != nil {
			agentID = strings.TrimSpace(*in.AgentId)
		}
		if err := m.checkAgent(ctx, agentID); err != nil {
			return config{}, err
		}
		cfg.AgentID = agentID
	default:
		return config{}, httpx.Invalid("连接方式只能是 direct 或 agent")
	}
	return cfg, nil
}

func (m *Module) saveConfig(ctx context.Context, in api.HAConfigInput) (config, error) {
	if strings.TrimSpace(in.Url) == "" {
		// An empty address removes the integration.
		for _, k := range []string{keyURL, keyToken, keyAgentID} {
			if err := m.d.Settings.Delete(ctx, k); err != nil {
				return config{}, err
			}
		}
		return config{}, nil
	}
	cfg, err := m.resolveInput(ctx, in)
	if err != nil {
		return config{}, err
	}
	if err := m.d.Settings.Set(ctx, keyURL, cfg.URL); err != nil {
		return config{}, err
	}
	if err := m.d.Settings.SetSecret(ctx, keyToken, cfg.Token); err != nil {
		return config{}, err
	}
	if err := m.d.Settings.Set(ctx, keyAgentID, cfg.AgentID); err != nil {
		return config{}, err
	}
	return cfg, nil
}

// TestHA tests the stored config, or the form values in the body. Testing
// form values can send the stored token to a new address, so it needs elevation.
func (m *Module) TestHA(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var cfg config
	if len(bytes.TrimSpace(raw)) == 0 {
		if cfg, err = m.requireConfigured(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	} else {
		if err := auth.RequireElevated(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		var body api.HAConfigInput
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			httpx.Fail(w, r, httpx.Invalid("请求体格式不正确: "+err.Error()))
			return
		}
		if cfg, err = m.resolveInput(r.Context(), body); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	httpx.JSON(w, http.StatusOK, m.test(r.Context(), cfg))
}

func (m *Module) GetHAStatus(w http.ResponseWriter, r *http.Request) {
	cfg, err := m.loadConfig(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	st := m.c.status()
	out := api.HAStatus{Configured: cfg.configured(), Connected: st.connected, Mode: cfg.mode(), EntityCount: st.entityCount}
	if st.connected {
		out.Version = &st.version
		out.ConnectedAt = &st.connectedAt
	}
	if st.lastErr != "" && !st.connected {
		out.Error = &st.lastErr
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) ListHAStates(w http.ResponseWriter, r *http.Request, params api.ListHAStatesParams) {
	if _, err := m.requireConfigured(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var domain, q string
	if params.Domain != nil {
		domain = *params.Domain
	}
	if params.Q != nil {
		q = *params.Q
	}
	httpx.JSON(w, http.StatusOK, m.c.list(domain, q))
}

func (m *Module) GetHAState(w http.ResponseWriter, r *http.Request, entityID string) {
	st, err := m.State(r.Context(), entityID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (m *Module) CallHAService(w http.ResponseWriter, r *http.Request, domain, service string) {
	var body api.HAServiceCall
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var entityID string
	if body.EntityId != nil {
		entityID = *body.EntityId
	}
	var data map[string]any
	if body.Data != nil {
		data = *body.Data
	}
	if isDangerous(domain, service, entityID, data) {
		if err := auth.RequireElevated(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if err := m.callService(r.Context(), domain, service, entityID, data); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) ListHAFavorites(w http.ResponseWriter, r *http.Request) {
	out, err := m.favorites(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) PutHAFavorites(w http.ResponseWriter, r *http.Request) {
	var body api.PutHAFavoritesJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err := m.saveFavorites(r.Context(), body.Items)
	m.d.Audit.Record(r.Context(), "ha.favorites", "", map[string]any{"count": len(body.Items)}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.favorites(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) favorites(ctx context.Context) ([]api.HAFavorite, error) {
	rows, err := m.q.ListFavorites(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.HAFavorite, 0, len(rows))
	for _, row := range rows {
		f := api.HAFavorite{EntityId: row.EntityID, Alias: row.Alias, SortOrder: int(row.SortOrder)}
		if st, ok := m.c.state(row.EntityID); ok {
			f.State = stateToAPI(st)
		}
		out = append(out, f)
	}
	return out, nil
}

func stateToAPI(st contracts.HAState) *api.HAState {
	return &api.HAState{EntityId: st.EntityID, State: st.State, Attributes: st.Attributes, LastChanged: st.LastChanged}
}

func (m *Module) saveFavorites(ctx context.Context, items []api.HAFavoriteInput) error {
	if len(items) > 200 {
		return httpx.Invalid("收藏最多 200 个")
	}
	seen := map[string]bool{}
	for _, it := range items {
		if err := validEntityID(it.EntityId); err != nil {
			return err
		}
		if seen[it.EntityId] {
			return httpx.Invalid("收藏里有重复的实体: " + it.EntityId)
		}
		seen[it.EntityId] = true
		if it.Alias != nil && len([]rune(*it.Alias)) > 64 {
			return httpx.Invalid("别名最多 64 个字")
		}
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	if err := q.DeleteAllFavorites(ctx); err != nil {
		return err
	}
	ids := make([]string, 0, len(items))
	for i, it := range items {
		alias := ""
		if it.Alias != nil {
			alias = strings.TrimSpace(*it.Alias)
		}
		if err := q.InsertFavorite(ctx, db.InsertFavoriteParams{EntityID: it.EntityId, SortOrder: int64(i), Alias: alias}); err != nil {
			return err
		}
		ids = append(ids, it.EntityId)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	m.c.setFavorites(ids)
	return nil
}
