package ai

import (
	"context"
	"errors"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
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
	cfg := llm.Config{ProviderID: p.Id, ProviderName: p.Name, BaseURL: p.BaseUrl, Model: ref.Model, APIStyle: string(p.ApiStyle)}
	if p.key.Valid && p.key.String != "" {
		cfg.APIKey, err = m.d.Secrets.Open(p.key.String)
		if err != nil {
			return llm.Config{}, err
		}
	}
	effort := selected.ReasoningEffort
	if purpose == "fast" {
		effort = selected.FastReasoningEffort
	}
	if effort != api.Off && effort != "" {
		unsupported, err := m.reasoningUnsupported(ctx, p.Id)
		if err != nil {
			return llm.Config{}, err
		}
		if !unsupported {
			cfg.ReasoningEffort = string(effort)
		}
	}
	models, err := m.listModels(ctx, &p.Id)
	if err != nil {
		return llm.Config{}, err
	}
	for _, model := range models {
		if model.Id == ref.Model {
			cfg.InputPrice, cfg.OutputPrice = model.InputPrice, model.OutputPrice
			cfg.CacheReadPrice, cfg.CacheWritePrice = model.CacheReadPrice, model.CacheWritePrice
			return cfg, nil
		}
	}
	return llm.Config{}, llm.ErrNotConfigured
}

// reasoningUnsupported reports whether this provider rejected the reasoning
// setting before (markReasoningUnsupported).
func (m *Module) reasoningUnsupported(ctx context.Context, providerID int64) (bool, error) {
	var ids []int64
	if err := optionalSetting(ctx, m, "ai.reasoning_unsupported", &ids); err != nil {
		return false, err
	}
	for _, id := range ids {
		if id == providerID {
			return true, nil
		}
	}
	return false, nil
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
	prices := usagePrices{Input: cfg.InputPrice, Output: cfg.OutputPrice, CacheRead: cfg.CacheReadPrice, CacheWrite: cfg.CacheWritePrice}
	source := contracts.AIUsageFrom(ctx)
	m.writeUsage(ctx, usageRow{
		ProviderID: &cfg.ProviderID, ProviderName: cfg.ProviderName, Model: cfg.Model, Purpose: purpose,
		Source: source.Source, Ref: source.Ref,
		Input: result.InputTokens, Cached: result.CachedInputTokens, CacheWrite: result.CacheWriteTokens,
		Output: result.OutputTokens, Reasoning: result.ReasoningTokens, Duration: duration, Err: callErr,
	}, prices, nil)
}
