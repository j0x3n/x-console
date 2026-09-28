package ai

import (
	"context"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

func (m *Module) Polish(ctx context.Context, markdown string) (string, error) {
	key, err := m.apiKey(ctx)
	if err != nil {
		return "", err
	}
	if key == "" {
		return "", httpx.ErrIntegrationMissing
	}
	settings, err := m.settings(ctx)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	opts := []option.RequestOption{option.WithAPIKey(key)}
	if m.baseURL != "" {
		opts = append(opts, option.WithBaseURL(m.baseURL))
	}
	client := anthropic.NewClient(opts...)
	res, err := client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{Model: anthropic.Model(settings.Model), MaxTokens: 512, Messages: []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("用两三句中文总结这份早报，只返回总结。\n" + markdown))}, Thinking: anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}}, Fallbacks: anthropic.BetaFallbacksParamOfDefault(), Betas: []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}})
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, block := range res.Content {
		out.WriteString(block.Text)
	}
	return strings.TrimSpace(out.String()), nil
}
