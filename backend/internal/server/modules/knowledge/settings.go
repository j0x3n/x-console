package knowledge

import (
	"context"
	"errors"
	"fmt"
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

// providerID is the AI provider for embeddings, 0 for the fast AI one (B197).
func providerID(o api.KnowledgeSettings) int64 {
	if o.ProviderId == nil || *o.ProviderId < 0 {
		return 0
	}
	return *o.ProviderId
}

// indexKey tells vectors from different providers and models apart, so that
// switching either rebuilds the index. Without a chosen provider it is the
// model alone, as before B197.
func indexKey(o api.KnowledgeSettings) string {
	if id := providerID(o); id > 0 {
		return fmt.Sprintf("p%d/%s", id, model(o))
	}
	return model(o)
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
	if id := providerID(body); id > 0 {
		if em, ok := m.embedder(); !ok || !em.Available(r.Context(), id) {
			httpx.Fail(w, r, httpx.Invalid("找不到这个 AI 供应商"))
			return
		}
	} else {
		body.ProviderId = nil
	}
	err := m.d.Settings.Set(r.Context(), OptionsKey, body)
	m.d.Audit.Record(r.Context(), "knowledge.settings", "", map[string]any{"enabled": body.Enabled, "sources": body.Sources, "provider": providerID(body)}, err)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, body)
}
