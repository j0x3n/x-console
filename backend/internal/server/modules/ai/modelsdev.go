package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const modelsDevURL = "https://models.dev/api.json"
const modelsDevLimit = 20 << 20

type modelSpec struct {
	Name  string `json:"name,omitempty"`
	Limit struct {
		Context int `json:"context,omitempty"`
		Output  int `json:"output,omitempty"`
	} `json:"limit"`
	ToolCall   *bool `json:"tool_call,omitempty"`
	Reasoning  *bool `json:"reasoning,omitempty"`
	Modalities struct {
		Input []string `json:"input,omitempty"`
	} `json:"modalities"`
	Cost struct {
		Input  *float32 `json:"input,omitempty"`
		Output *float32 `json:"output,omitempty"`
	} `json:"cost"`
}

type modelCatalog map[string]map[string]modelSpec

func parseModelsDev(raw []byte) (modelCatalog, error) {
	var source map[string]struct {
		Models map[string]modelSpec `json:"models"`
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		return nil, err
	}
	if len(source) == 0 {
		return nil, errors.New("models.dev 没有供应商")
	}
	out := make(modelCatalog, len(source))
	for provider, value := range source {
		out[provider] = value.Models
	}
	return out, nil
}

func (m *Module) Start(ctx context.Context) error {
	// An action approved before a restart never finished. Mark it failed so
	// the conversation can go on (approved ones block new messages).
	if _, err := m.d.DB.ExecContext(ctx, "UPDATE ai_pending_actions SET status='failed',result=? WHERE status='approved'", `"服务重启，操作没有执行完，结果未知"`); err != nil {
		return err
	}
	m.d.Scheduler.Every("ai.modelsdev", 24*time.Hour, m.syncModelsDev)
	m.d.Scheduler.Every("ai.attachments.cleanup", 24*time.Hour, m.cleanupAttachments)
	go func() {
		if err := m.syncModelsDev(ctx); err != nil && ctx.Err() == nil {
			m.d.Log.Warn("models.dev sync failed", "err", err)
		}
	}()
	return nil
}

func (m *Module) syncModelsDev(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", modelsDevURL, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("models.dev 返回 %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, modelsDevLimit+1))
	if err != nil {
		return err
	}
	if len(raw) > modelsDevLimit {
		return errors.New("models.dev 响应超过 20 MB")
	}
	catalog, err := parseModelsDev(raw)
	if err != nil {
		return err
	}
	if err := m.d.Settings.Set(ctx, "ai.modelsdev", catalog); err != nil {
		return err
	}
	return m.d.Settings.Set(ctx, "ai.modelsdev_synced_at", time.Now().UTC())
}

func (m *Module) catalog(ctx context.Context) (modelCatalog, error) {
	var out modelCatalog
	err := m.d.Settings.Get(ctx, "ai.modelsdev", &out)
	if errors.Is(err, settings.ErrNotSet) {
		return modelCatalog{}, nil
	}
	return out, err
}

func knownProvider(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	return map[string]string{
		"api.openai.com": "openai", "api.deepseek.com": "deepseek", "openrouter.ai": "openrouter",
		"api.mistral.ai": "mistral", "api.groq.com": "groq.com",
		"generativelanguage.googleapis.com": "google", "api.together.xyz": "togetherai",
	}[host]
}

func applySpec(out *api.AiModel, spec modelSpec, source api.AiModelSpecSource) {
	out.SpecSource = source
	if spec.Name != "" {
		out.Name = &spec.Name
	}
	if spec.Limit.Context > 0 {
		out.ContextWindow = &spec.Limit.Context
	}
	if spec.Limit.Output > 0 {
		out.MaxOutput = &spec.Limit.Output
	}
	out.ToolCall, out.Reasoning = spec.ToolCall, spec.Reasoning
	image := false
	for _, kind := range spec.Modalities.Input {
		if kind == "image" {
			image = true
		}
	}
	out.ImageInput = &image
	out.InputPrice, out.OutputPrice = spec.Cost.Input, spec.Cost.Output
}

func specFor(catalog modelCatalog, baseURL, id string) api.AiModel {
	out := api.AiModel{Id: id, SpecSource: "unknown"}
	if provider := knownProvider(baseURL); provider != "" {
		if spec, ok := catalog[provider][id]; ok {
			applySpec(&out, spec, "exact")
			return out
		}
	}
	short := id
	if index := strings.LastIndexByte(id, '/'); index >= 0 {
		short = id[index+1:]
	}
	providers := make([]string, 0, len(catalog))
	for provider := range catalog {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	for _, provider := range providers {
		if spec, ok := catalog[provider][short]; ok {
			applySpec(&out, spec, "stripped")
			return out
		}
	}
	return out
}

func (m *Module) listModels(ctx context.Context, providerID *int64) ([]api.AiModel, error) {
	catalog, err := m.catalog(ctx)
	if err != nil {
		return nil, err
	}
	query := `SELECT p.id,p.base_url,pm.model_id,s.provider_id,s.context_window,s.tool_call,s.reasoning
 FROM ai_provider_models pm JOIN ai_providers p ON p.id=pm.provider_id
 LEFT JOIN ai_model_specs s ON s.provider_id=p.id AND s.model_id=pm.model_id`
	args := []any{}
	if providerID != nil {
		query += ` WHERE p.id=?`
		args = append(args, *providerID)
	}
	query += ` ORDER BY p.id,pm.model_id`
	rows, err := m.d.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []api.AiModel{}
	for rows.Next() {
		var id int64
		var baseURL, modelID string
		var manual, contextWindow, tools, reasoning sql.NullInt64
		if err := rows.Scan(&id, &baseURL, &modelID, &manual, &contextWindow, &tools, &reasoning); err != nil {
			return nil, err
		}
		model := specFor(catalog, baseURL, modelID)
		model.ProviderId = id
		if manual.Valid {
			model.SpecSource = "manual"
			if contextWindow.Valid {
				value := int(contextWindow.Int64)
				model.ContextWindow = &value
			}
			if tools.Valid {
				value := tools.Int64 != 0
				model.ToolCall = &value
			}
			if reasoning.Valid {
				value := reasoning.Int64 != 0
				model.Reasoning = &value
			}
		}
		out = append(out, model)
	}
	return out, rows.Err()
}

func (m *Module) ListAiModels(w http.ResponseWriter, r *http.Request, params api.ListAiModelsParams) {
	out, err := m.listModels(r.Context(), params.ProviderId)
	if m.fail(w, r, err) {
		return
	}
	httpx.JSON(w, 200, out)
}

func (m *Module) RefreshAiModels(w http.ResponseWriter, r *http.Request, id api.ProviderId) {
	if _, err := m.refreshModels(r.Context(), id); m.fail(w, r, err) {
		return
	}
	out, err := m.listModels(r.Context(), &id)
	if m.fail(w, r, err) {
		return
	}
	httpx.JSON(w, 200, out)
}

func (m *Module) SetAiModelSpec(w http.ResponseWriter, r *http.Request) {
	var body api.AiModelSpecInput
	if m.fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if body.ProviderId <= 0 || body.ModelId == "" || body.ContextWindow != nil && *body.ContextWindow <= 0 {
		httpx.Fail(w, r, httpx.Invalid("模型规格不正确"))
		return
	}
	var exists int
	err := m.d.DB.QueryRowContext(r.Context(), `SELECT 1 FROM ai_provider_models WHERE provider_id=? AND model_id=?`, body.ProviderId, body.ModelId).Scan(&exists)
	if m.fail(w, r, notFound(err)) {
		return
	}
	_, err = m.d.DB.ExecContext(r.Context(), `INSERT INTO ai_model_specs(provider_id,model_id,context_window,tool_call,reasoning) VALUES(?,?,?,?,?)
 ON CONFLICT(provider_id,model_id) DO UPDATE SET context_window=excluded.context_window,tool_call=excluded.tool_call,reasoning=excluded.reasoning`,
		body.ProviderId, body.ModelId, body.ContextWindow, nullableBool(body.ToolCall), nullableBool(body.Reasoning))
	if m.fail(w, r, err) {
		return
	}
	models, err := m.listModels(r.Context(), &body.ProviderId)
	if m.fail(w, r, err) {
		return
	}
	for _, model := range models {
		if model.Id == body.ModelId {
			m.d.Audit.Record(r.Context(), "ai.model_spec.update", body.ModelId, map[string]any{"providerId": body.ProviderId}, nil)
			m.d.Bus.Publish("ai.provider_changed", map[string]any{"id": body.ProviderId})
			httpx.JSON(w, 200, model)
			return
		}
	}
	httpx.Fail(w, r, httpx.ErrNotFound)
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}
