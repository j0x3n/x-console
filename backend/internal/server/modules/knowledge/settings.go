package knowledge

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/knowledge/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// OptionsKey holds the module's settings.
const OptionsKey = "knowledge.options"

// DefaultModel is the embedding model used when the setting is empty.
const DefaultModel = "text-embedding-3-small"

func defaults() api.KnowledgeSettings {
	return api.KnowledgeSettings{Enabled: false, Model: "", Sources: slices.Clone(AllSources)}
}

func (m *Module) options(ctx context.Context) api.KnowledgeSettings {
	var saved api.KnowledgeSettings
	err := m.d.Settings.Get(ctx, OptionsKey, &saved)
	if err == nil {
		if saved.Sources == nil {
			saved.Sources = []string{}
		}
		return saved
	}
	if !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("load knowledge settings", "err", err)
	}
	return defaults()
}

// model is the embedding model in use.
func model(o api.KnowledgeSettings) string {
	if o.Model == "" {
		return DefaultModel
	}
	return o.Model
}

// GetKnowledgeSettings implements api.ServerInterface.
func (m *Module) GetKnowledgeSettings(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, m.options(r.Context()))
}

// PutKnowledgeSettings implements api.ServerInterface.
func (m *Module) PutKnowledgeSettings(w http.ResponseWriter, r *http.Request) {
	var body api.KnowledgeSettings
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	body.Model = strings.TrimSpace(body.Model)
	if utf8.RuneCountInString(body.Model) > 100 {
		httpx.Fail(w, r, httpx.Invalid("模型名太长了"))
		return
	}
	var clean []string
	for _, s := range AllSources {
		if slices.Contains(body.Sources, s) {
			clean = append(clean, s)
		}
	}
	for _, s := range body.Sources {
		if !slices.Contains(AllSources, s) {
			httpx.Fail(w, r, httpx.Invalid("不认识的来源："+s))
			return
		}
	}
	if clean == nil {
		clean = []string{}
	}
	body.Sources = clean
	err := m.d.Settings.Set(r.Context(), OptionsKey, body)
	m.d.Audit.Record(r.Context(), "knowledge.settings", "", map[string]any{"enabled": body.Enabled, "sources": body.Sources}, err)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, body)
}
