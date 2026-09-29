package ai

import (
	"context"
	"errors"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

func (m *Module) resolveLLM(ctx context.Context, purpose string) (llm.Config, error) {
	if purpose != "fast" && purpose != "agent" {
		return llm.Config{}, errors.New("AI 用途不正确")
	}
	selected, err := m.modelSettings(ctx)
	if err != nil {
		return llm.Config{}, err
	}
	ref := selected.Agent
	if purpose == "fast" && selected.Fast != nil {
		ref = selected.Fast
	}
	if ref == nil {
		return llm.Config{}, llm.ErrNotConfigured
	}
	p, err := m.provider(ctx, ref.ProviderId)
	if errors.Is(err, httpx.ErrNotFound) {
		return llm.Config{}, llm.ErrNotConfigured
	}
	if err != nil {
		return llm.Config{}, err
	}
	cfg := llm.Config{ProviderID: p.Id, ProviderName: p.Name, BaseURL: p.BaseUrl, Model: ref.Model}
	if p.key.Valid && p.key.String != "" {
		cfg.APIKey, err = m.d.Secrets.Open(p.key.String)
		if err != nil {
			return llm.Config{}, err
		}
	}
	if purpose == "agent" && !selected.ReasoningUnsupported && selected.ReasoningEffort != "off" {
		cfg.ReasoningEffort = string(selected.ReasoningEffort)
	}
	models, err := m.listModels(ctx, &p.Id)
	if err != nil {
		return llm.Config{}, err
	}
	for _, model := range models {
		if model.Id == ref.Model {
			cfg.InputPrice, cfg.OutputPrice = model.InputPrice, model.OutputPrice
			return cfg, nil
		}
	}
	return llm.Config{}, llm.ErrNotConfigured
}

func (m *Module) markReasoningUnsupported(ctx context.Context, providerID int64) error {
	m.reasonMu.Lock()
	defer m.reasonMu.Unlock()
	var ids []int64
	err := m.d.Settings.Get(ctx, "ai.reasoning_unsupported", &ids)
	if err != nil && !errors.Is(err, settings.ErrNotSet) {
		return err
	}
	for _, id := range ids {
		if id == providerID {
			return nil
		}
	}
	ids = append(ids, providerID)
	return m.d.Settings.Set(ctx, "ai.reasoning_unsupported", ids)
}

func (m *Module) recordLLM(ctx context.Context, cfg llm.Config, purpose string, result llm.Result, duration time.Duration, callErr error) {
	var cost any
	if cfg.InputPrice != nil && cfg.OutputPrice != nil {
		cost = (float64(result.InputTokens)*float64(*cfg.InputPrice) + float64(result.OutputTokens)*float64(*cfg.OutputPrice)) / 1_000_000
	}
	_, err := m.d.DB.ExecContext(ctx, `INSERT INTO ai_usage(provider_id,provider_name,model,purpose,input_tokens,output_tokens,duration_ms,cost,created_at)
 VALUES(?,?,?,?,?,?,?,?,?)`, cfg.ProviderID, cfg.ProviderName, cfg.Model, purpose, result.InputTokens, result.OutputTokens, duration.Milliseconds(), cost, time.Now().UTC())
	if err != nil {
		m.d.Log.Error("AI usage write failed", "err", err, "callErr", callErr)
	}
}
