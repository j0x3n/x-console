package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

func optionalSetting[T any](ctx context.Context, m *Module, key string, value *T) error {
	err := m.d.Settings.Get(ctx, key, value)
	if errors.Is(err, settings.ErrNotSet) {
		return nil
	}
	return err
}

func (m *Module) modelSettings(ctx context.Context) (api.AiModelSettings, error) {
	out := api.AiModelSettings{ReasoningEffort: api.Off, FastReasoningEffort: api.Off}
	if err := optionalSetting(ctx, m, "ai.fast_model", &out.Fast); err != nil {
		return out, err
	}
	if err := optionalSetting(ctx, m, "ai.agent_model", &out.Agent); err != nil {
		return out, err
	}
	if err := optionalSetting(ctx, m, "ai.reasoning_effort", &out.ReasoningEffort); err != nil {
		return out, err
	}
	if err := optionalSetting(ctx, m, "ai.fast_reasoning_effort", &out.FastReasoningEffort); err != nil {
		return out, err
	}
	if err := optionalSetting(ctx, m, confirmSetting, &out.ConfirmAllWrites); err != nil {
		return out, err
	}
	// B60: until a default level is chosen, the old "confirm all writes"
	// switch decides it, so nothing changes for existing setups.
	def := api.AiModelSettingsDefaultPermission(permWrite)
	if out.ConfirmAllWrites {
		def = api.AiModelSettingsDefaultPermission(permManual)
	}
	if err := optionalSetting(ctx, m, defaultPermissionSetting, &def); err != nil {
		return out, err
	}
	out.DefaultPermission = &def
	if err := optionalSetting(ctx, m, "ai.modelsdev_synced_at", &out.ModelsDevSyncedAt); err != nil {
		return out, err
	}
	var unsupported []int64
	if err := optionalSetting(ctx, m, "ai.reasoning_unsupported", &unsupported); err != nil {
		return out, err
	}
	if out.Agent != nil {
		for _, providerID := range unsupported {
			if providerID == out.Agent.ProviderId {
				out.ReasoningUnsupported = true
			}
		}
	}
	var count int
	if err := m.d.DB.QueryRowContext(ctx, `SELECT count(*) FROM ai_providers`).Scan(&count); err != nil {
		return out, err
	}
	if count == 0 {
		key, err := m.apiKey(ctx)
		if err != nil {
			return out, err
		}
		legacy := key != ""
		out.LegacyAnthropic = &legacy
	}
	return out, nil
}

func (m *Module) GetAiModelSettings(w http.ResponseWriter, r *http.Request) {
	out, err := m.modelSettings(r.Context())
	if m.fail(w, r, err) {
		return
	}
	httpx.JSON(w, 200, out)
}

func (m *Module) validateModelRef(ctx context.Context, ref *api.ModelRef, agent bool) error {
	if ref == nil {
		return nil
	}
	models, err := m.listModels(ctx, &ref.ProviderId)
	if err != nil {
		return err
	}
	for _, model := range models {
		if model.Id == ref.Model {
			if agent && model.ToolCall != nil && !*model.ToolCall {
				return httpx.NewError(400, "model_without_tools", "这个模型不支持工具调用")
			}
			return nil
		}
	}
	return httpx.NewError(400, "unknown_model", "模型不在供应商的列表里")
}

func (m *Module) PutAiModelSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if m.fail(w, r, auth.RequireElevated(ctx)) {
		return
	}
	var body api.AiModelSettingsInput
	if m.fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if body.ReasoningEffort != nil && !body.ReasoningEffort.Valid() || body.FastReasoningEffort != nil && !body.FastReasoningEffort.Valid() {
		httpx.Fail(w, r, httpx.Invalid("思考程度不正确"))
		return
	}
	if m.fail(w, r, m.validateModelRef(ctx, body.Agent, true)) || m.fail(w, r, m.validateModelRef(ctx, body.Fast, false)) {
		return
	}
	previous, err := m.modelSettings(ctx)
	if m.fail(w, r, err) {
		return
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if m.fail(w, r, err) {
		return
	}
	defer tx.Rollback()
	for _, item := range []struct {
		key   string
		value *api.ModelRef
	}{{"ai.agent_model", body.Agent}, {"ai.fast_model", body.Fast}} {
		if item.value == nil {
			_, err = tx.ExecContext(ctx, `DELETE FROM settings WHERE key=?`, item.key)
		} else {
			err = setAISetting(ctx, tx, item.key, item.value)
		}
		if m.fail(w, r, err) {
			return
		}
	}
	if body.ReasoningEffort != nil {
		if m.fail(w, r, setAISetting(ctx, tx, "ai.reasoning_effort", body.ReasoningEffort)) {
			return
		}
	}
	if body.FastReasoningEffort != nil {
		if m.fail(w, r, setAISetting(ctx, tx, "ai.fast_reasoning_effort", body.FastReasoningEffort)) {
			return
		}
	}
	if body.ConfirmAllWrites != nil {
		if m.fail(w, r, setAISetting(ctx, tx, confirmSetting, body.ConfirmAllWrites)) {
			return
		}
	}
	if body.DefaultPermission != nil {
		// B60: "all" is never a default; it needs elevation each time.
		if p := string(*body.DefaultPermission); p != permManual && p != permWrite {
			httpx.Fail(w, r, httpx.Invalid("默认权限只能是手动或写入"))
			return
		}
		if m.fail(w, r, setAISetting(ctx, tx, defaultPermissionSetting, body.DefaultPermission)) {
			return
		}
	}
	if !reflect.DeepEqual(previous.Agent, body.Agent) || !reflect.DeepEqual(previous.Fast, body.Fast) ||
		body.ReasoningEffort != nil && previous.ReasoningEffort != *body.ReasoningEffort ||
		body.FastReasoningEffort != nil && previous.FastReasoningEffort != *body.FastReasoningEffort {
		_, err = tx.ExecContext(ctx, `DELETE FROM settings WHERE key='ai.reasoning_unsupported'`)
		if m.fail(w, r, err) {
			return
		}
	}
	if m.fail(w, r, tx.Commit()) {
		return
	}
	m.d.Audit.Record(ctx, "ai.settings.update", "", map[string]any{"agent": body.Agent, "fast": body.Fast}, nil)
	m.d.Bus.Publish("ai.provider_changed", nil)
	m.GetAiModelSettings(w, r)
}

func setAISetting(ctx context.Context, tx *sql.Tx, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO settings(key,value,encrypted,updated_at) VALUES(?,?,0,?)
 ON CONFLICT(key) DO UPDATE SET value=excluded.value,encrypted=0,updated_at=excluded.updated_at`, key, string(raw), time.Now().UTC())
	return err
}

func monthBounds(value string, location *time.Location) (string, time.Time, time.Time, error) {
	if value == "" {
		value = time.Now().In(location).Format("2006-01")
	}
	if len(value) != 7 {
		return "", time.Time{}, time.Time{}, httpx.Invalid("月份格式应为 YYYY-MM")
	}
	start, err := time.ParseInLocation("2006-01", value, location)
	if err != nil || start.Format("2006-01") != value {
		return "", time.Time{}, time.Time{}, httpx.Invalid("月份格式应为 YYYY-MM")
	}
	return value, start.UTC(), start.AddDate(0, 1, 0).UTC(), nil
}

func (m *Module) GetAiUsage(w http.ResponseWriter, r *http.Request, params api.GetAiUsageParams) {
	value := ""
	if params.Month != nil {
		value = *params.Month
	}
	month, start, end, err := monthBounds(value, m.d.Config.Location)
	if m.fail(w, r, err) {
		return
	}
	rows, err := m.d.DB.QueryContext(r.Context(), `SELECT COALESCE(provider_id,0),provider_name,model,purpose,
 count(*),sum(input_tokens),sum(output_tokens),sum(cost),sum(cached_input_tokens),sum(cache_write_tokens)
 FROM ai_usage WHERE created_at>=? AND created_at<?
 GROUP BY provider_id,provider_name,model,purpose ORDER BY sum(cost) IS NULL,sum(cost) DESC,provider_name,model,purpose`, start, end)
	if m.fail(w, r, err) {
		return
	}
	defer rows.Close()
	out := api.AiUsage{Month: month}
	var totalCached, totalCacheWrite int64
	out.ByModel = make([]struct {
		Calls        int                        `json:"calls"`
		Cost         *float32                   `json:"cost,omitempty"`
		InputTokens  int64                      `json:"inputTokens"`
		Model        string                     `json:"model"`
		OutputTokens int64                      `json:"outputTokens"`
		ProviderId   int64                      `json:"providerId"`
		ProviderName string                     `json:"providerName"`
		Purpose      *api.AiUsageByModelPurpose `json:"purpose,omitempty"`
	}, 0)
	for rows.Next() {
		var item struct {
			Calls        int                        `json:"calls"`
			Cost         *float32                   `json:"cost,omitempty"`
			InputTokens  int64                      `json:"inputTokens"`
			Model        string                     `json:"model"`
			OutputTokens int64                      `json:"outputTokens"`
			ProviderId   int64                      `json:"providerId"`
			ProviderName string                     `json:"providerName"`
			Purpose      *api.AiUsageByModelPurpose `json:"purpose,omitempty"`
		}
		var cost *float32
		var purpose api.AiUsageByModelPurpose
		var cached, cacheWrite int64
		if err := rows.Scan(&item.ProviderId, &item.ProviderName, &item.Model, &purpose, &item.Calls, &item.InputTokens, &item.OutputTokens, &cost, &cached, &cacheWrite); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		item.Cost, item.Purpose = cost, &purpose
		out.Calls += item.Calls
		out.InputTokens += item.InputTokens
		out.OutputTokens += item.OutputTokens
		totalCached += cached
		totalCacheWrite += cacheWrite
		if cost != nil {
			if out.Cost == nil {
				out.Cost = new(float32)
			}
			*out.Cost += *cost
		}
		out.ByModel = append(out.ByModel, item)
	}
	if m.fail(w, r, rows.Err()) {
		return
	}
	out.CachedInputTokens, out.CacheWriteTokens = &totalCached, &totalCacheWrite
	if out.InputTokens > 0 {
		rate := float32(float64(totalCached) / float64(out.InputTokens))
		out.CacheHitRate = &rate
	}
	httpx.JSON(w, 200, out)
}
