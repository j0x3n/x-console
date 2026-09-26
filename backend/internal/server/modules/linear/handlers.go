package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/linear/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/linear/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

func (m *Module) configToAPI(ctx context.Context, cfg config) (api.LinearConfig, error) {
	teams, err := m.q.ListTeams(ctx)
	if err != nil {
		return api.LinearConfig{}, err
	}
	out := api.LinearConfig{HasKey: cfg.Key != "", ApiKey: maskKey(cfg.Key), ApiUrl: cfg.APIURL, Mappings: make([]api.LinearMapping, len(teams))}
	for i, t := range teams {
		out.Mappings[i] = api.LinearMapping{TeamId: t.TeamID, TeamKey: t.TeamKey, TeamName: t.TeamName, ProjectId: t.ProjectID, LastPulledAt: t.Cursor}
	}
	return out, nil
}

func (m *Module) GetLinearConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := m.loadConfig(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.configToAPI(r.Context(), cfg)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// PutLinearConfig saves the key and the team mappings. Changing the key or
// the API address needs elevation.
func (m *Module) PutLinearConfig(w http.ResponseWriter, r *http.Request) {
	var body api.LinearConfigInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cfg, err := m.saveConfig(r.Context(), body)
	m.d.Audit.Record(r.Context(), "linear.config", "", map[string]any{
		"mappings": len(body.Mappings), "keyChanged": body.ApiKey != nil && *body.ApiKey != "" || body.ClearKey != nil && *body.ClearKey,
	}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.configToAPI(r.Context(), cfg)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) saveConfig(ctx context.Context, in api.LinearConfigInput) (config, error) {
	old, err := m.loadConfig(ctx)
	if err != nil {
		return config{}, err
	}
	apiURL := old.APIURL
	if in.ApiUrl != nil {
		if apiURL, err = normalizeAPIURL(*in.ApiUrl); err != nil {
			return config{}, err
		}
	}
	newKey := in.ApiKey != nil && strings.TrimSpace(*in.ApiKey) != ""
	clear := in.ClearKey != nil && *in.ClearKey
	if newKey || clear || apiURL != old.APIURL {
		if err := auth.RequireElevated(ctx); err != nil {
			return config{}, err
		}
	}
	key := old.Key
	switch {
	case clear:
		key = ""
	case newKey:
		key = strings.TrimSpace(*in.ApiKey)
	}
	// Validate the mappings: one team per project and the other way round.
	teams, projects := map[string]bool{}, map[int64]bool{}
	for _, mp := range in.Mappings {
		mp.TeamId = strings.TrimSpace(mp.TeamId)
		if mp.TeamId == "" || mp.ProjectId <= 0 {
			return config{}, httpx.Invalid("每一行都要选团队和项目")
		}
		if teams[mp.TeamId] || projects[mp.ProjectId] {
			return config{}, httpx.Invalid("一个团队只能对应一个项目，一个项目也只能对应一个团队")
		}
		teams[mp.TeamId], projects[mp.ProjectId] = true, true
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	old2, err := m.q.ListTeams(ctx)
	if err != nil {
		return config{}, err
	}
	prev := map[string]db.LinearTeam{}
	for _, t := range old2 {
		prev[t.TeamID] = t
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return config{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := m.q.WithTx(tx)
	if err := q.DeleteTeams(ctx); err != nil {
		return config{}, err
	}
	now := m.now()
	for _, mp := range in.Mappings {
		id := strings.TrimSpace(mp.TeamId)
		p := db.InsertTeamParams{TeamID: id, TeamKey: strings.TrimSpace(deref(mp.TeamKey)), TeamName: strings.TrimSpace(deref(mp.TeamName)),
			ProjectID: mp.ProjectId, CreatedAt: now}
		if old, ok := prev[id]; ok {
			if p.TeamKey == "" {
				p.TeamKey = old.TeamKey
			}
			if p.TeamName == "" {
				p.TeamName = old.TeamName
			}
			p.CreatedAt = old.CreatedAt
			if old.ProjectID == mp.ProjectId {
				p.Cursor = old.Cursor // same pairing: continue where we left off
			}
		}
		if err := q.InsertTeam(ctx, p); err != nil {
			return config{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return config{}, err
	}
	if key == "" {
		if err := m.d.Settings.Delete(ctx, keyAPIKey); err != nil {
			return config{}, err
		}
	} else if key != old.Key {
		if err := m.d.Settings.SetSecret(ctx, keyAPIKey, key); err != nil {
			return config{}, err
		}
	}
	if err := m.d.Settings.Set(ctx, keyAPIURL, apiURL); err != nil {
		return config{}, err
	}
	m.states = map[string][]lnState{}
	return m.loadConfig(ctx)
}

// TestLinear checks the stored key, or the form values in the body.
func (m *Module) TestLinear(w http.ResponseWriter, r *http.Request) {
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
		var body api.LinearTestInput
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			httpx.Fail(w, r, httpx.Invalid("请求体格式不正确: "+err.Error()))
			return
		}
		if cfg, err = m.loadConfig(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if body.ApiKey != nil && strings.TrimSpace(*body.ApiKey) != "" {
			cfg.Key = strings.TrimSpace(*body.ApiKey)
		}
		if body.ApiUrl != nil {
			if cfg.APIURL, err = normalizeAPIURL(*body.ApiUrl); err != nil {
				httpx.Fail(w, r, err)
				return
			}
		}
		if cfg.Key == "" {
			httpx.Fail(w, r, httpx.Invalid("请填写 API key"))
			return
		}
	}
	user, err := m.client(cfg).viewer(r.Context())
	if err != nil {
		msg := err.Error()
		httpx.JSON(w, http.StatusOK, api.LinearTestResult{Ok: false, Message: &msg})
		return
	}
	httpx.JSON(w, http.StatusOK, api.LinearTestResult{Ok: true, User: &user})
}

func (m *Module) ListLinearTeams(w http.ResponseWriter, r *http.Request) {
	cfg, err := m.requireConfigured(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	teams, err := m.client(cfg).teams(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "linear_error", err.Error()))
		return
	}
	out := make([]api.LinearTeam, len(teams))
	for i, t := range teams {
		out[i] = api.LinearTeam{Id: t.ID, Key: t.Key, Name: t.Name}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) status(ctx context.Context) (api.LinearStatus, error) {
	cfg, err := m.loadConfig(ctx)
	if err != nil {
		return api.LinearStatus{}, err
	}
	teams, err := m.q.ListTeams(ctx)
	if err != nil {
		return api.LinearStatus{}, err
	}
	out := api.LinearStatus{Configured: cfg.configured(), Syncing: m.isSyncing(), MappingCount: len(teams)}
	var last api.LinearSyncResult
	if err := m.d.Settings.Get(ctx, keyLastSync, &last); err == nil {
		if last.Errors == nil {
			last.Errors = []string{}
		}
		out.LastSync = &last
	} else if !errors.Is(err, settings.ErrNotSet) {
		return out, err
	}
	return out, nil
}

func (m *Module) GetLinearStatus(w http.ResponseWriter, r *http.Request) {
	out, err := m.status(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// SyncLinear syncs now and returns the status. Sync problems are in the
// result; only a missing setup is an error.
func (m *Module) SyncLinear(w http.ResponseWriter, r *http.Request) {
	if _, err := m.sync(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.status(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
