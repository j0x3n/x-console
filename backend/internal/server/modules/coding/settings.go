package coding

import (
	"context"
	"errors"
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Setting keys.
const (
	keyMaxConcurrent  = "coding.max_concurrent"
	keyDefaultTimeout = "coding.default_timeout_minutes"

	defaultMaxConcurrent  = 2
	defaultTimeoutMinutes = 60
)

func (m *Module) intSetting(ctx context.Context, key string, def int) int {
	var v int
	if err := m.d.Settings.Get(ctx, key, &v); err != nil || v <= 0 {
		if err != nil && !errors.Is(err, settings.ErrNotSet) {
			m.d.Log.Warn("coding: read setting", "key", key, "err", err)
		}
		return def
	}
	return v
}

func (m *Module) maxConcurrent(ctx context.Context) int {
	return m.intSetting(ctx, keyMaxConcurrent, defaultMaxConcurrent)
}

func (m *Module) defaultTimeout(ctx context.Context) int {
	return m.intSetting(ctx, keyDefaultTimeout, defaultTimeoutMinutes)
}

func (m *Module) github() (contracts.GitHub, bool) {
	return module.Lookup[contracts.GitHub](m.d.Registry, contracts.GitHubKey)
}

func (m *Module) settingsView(ctx context.Context) api.CodingSettings {
	_, pr := m.github()
	return api.CodingSettings{MaxConcurrent: m.maxConcurrent(ctx), DefaultTimeoutMinutes: m.defaultTimeout(ctx), PrAvailable: pr}
}

// GetCodingSettings implements GET /coding/settings.
func (m *Module) GetCodingSettings(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, m.settingsView(r.Context()))
}

// UpdateCodingSettings implements PUT /coding/settings.
func (m *Module) UpdateCodingSettings(w http.ResponseWriter, r *http.Request) {
	var body api.UpdateCodingSettings
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	err := m.updateSettings(ctx, body)
	m.d.Audit.Record(ctx, "coding.settings", "", map[string]any{
		"maxConcurrent": body.MaxConcurrent, "defaultTimeoutMinutes": body.DefaultTimeoutMinutes}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	view := m.settingsView(ctx)
	m.d.Bus.Publish("coding_settings.updated", view)
	go m.dispatch() // a higher limit starts queued tasks
	httpx.JSON(w, http.StatusOK, view)
}

func (m *Module) updateSettings(ctx context.Context, body api.UpdateCodingSettings) error {
	if body.MaxConcurrent != nil && (*body.MaxConcurrent < 1 || *body.MaxConcurrent > 10) {
		return httpx.Invalid("同时运行的任务数要在 1 到 10 之间")
	}
	if body.DefaultTimeoutMinutes != nil && (*body.DefaultTimeoutMinutes < 1 || *body.DefaultTimeoutMinutes > 1440) {
		return httpx.Invalid("超时时间要在 1 到 1440 分钟之间")
	}
	if body.MaxConcurrent != nil {
		if err := m.d.Settings.Set(ctx, keyMaxConcurrent, *body.MaxConcurrent); err != nil {
			return err
		}
	}
	if body.DefaultTimeoutMinutes != nil {
		if err := m.d.Settings.Set(ctx, keyDefaultTimeout, *body.DefaultTimeoutMinutes); err != nil {
			return err
		}
	}
	return nil
}
