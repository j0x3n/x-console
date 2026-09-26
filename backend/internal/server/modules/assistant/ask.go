package assistant

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Ask is also used by automation steps. It has no tool access.
func (m *Module) Ask(ctx context.Context, prompt string) (string, error) {
	var key string
	if err := m.d.Settings.Get(ctx, apiKeyKey, &key); err != nil {
		return "", err
	}
	cfg, err := m.settings(ctx)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	opts := []option.RequestOption{option.WithAPIKey(key), option.WithHeader("anthropic-beta", "server-side-fallback-2026-07-01"), option.WithJSONSet("fallbacks", "default"), option.WithHTTPClient(&http.Client{Timeout: 10 * time.Minute})}
	if m.baseURL != "" {
		opts = append(opts, option.WithBaseURL(m.baseURL))
	}
	client := anthropic.NewClient(opts...)
	stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{Model: anthropic.Model(cfg.Model), MaxTokens: 2048, Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))}, Thinking: anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}})
	defer stream.Close()
	var msg anthropic.Message
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return "", err
		}
	}
	if err := stream.Err(); err != nil {
		return "", err
	}
	var text strings.Builder
	for _, block := range msg.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	if text.Len() == 0 {
		return "", errors.New("Claude 未返回文本")
	}
	return text.String(), nil
}

// Polish writes a short summary for the daily brief.
func (m *Module) Polish(ctx context.Context, markdown string) (string, error) {
	return m.Ask(ctx, "请用简短中文总结以下早报。保留重要事实，不编造内容。\n\n"+markdown)
}
