package ai

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

// DefaultEmbedModel is used when the caller names no model.
const DefaultEmbedModel = "text-embedding-3-small"

// GoogleEmbedModel is the default for Google's OpenAI compatible endpoint
// (B197), which has no text-embedding-3-small.
const GoogleEmbedModel = "gemini-embedding-001"

// maxEmbedInputs is how many texts go in one request. OpenAI allows 2048,
// Google's batch limit is 100.
const maxEmbedInputs = 100

// Embedder returns the module as a contracts.Embedder.
func (m *Module) embedder() contracts.Embedder { return embedder{m} }

type embedder struct{ m *Module }

// Available says whether the provider exists. 0 means the provider for fast
// AI calls.
func (e embedder) Available(ctx context.Context, providerID int64) bool {
	_, err := e.m.resolveEmbed(ctx, providerID)
	return err == nil
}

// config is resolveEmbed with messages the index status can show.
func (e embedder) config(ctx context.Context, providerID int64) (llm.Config, error) {
	cfg, err := e.m.resolveEmbed(ctx, providerID)
	if errors.Is(err, llm.ErrNotConfigured) {
		return llm.Config{}, errors.New("还没有配置 AI 供应商，不能建索引")
	}
	if errors.Is(err, httpx.ErrNotFound) {
		return llm.Config{}, errors.New("嵌入用的 AI 供应商已经删除，请在资料问答设置里重新选")
	}
	return cfg, err
}

// resolveEmbed finds the base URL and key for the embedding calls.
func (m *Module) resolveEmbed(ctx context.Context, providerID int64) (llm.Config, error) {
	if providerID == 0 {
		return m.resolveLLM(ctx, "fast")
	}
	p, err := m.provider(ctx, providerID)
	if err != nil {
		return llm.Config{}, err
	}
	cfg := llm.Config{ProviderID: p.Id, ProviderName: p.Name, BaseURL: p.BaseUrl}
	if p.key.Valid && p.key.String != "" {
		if cfg.APIKey, err = m.d.Secrets.Open(p.key.String); err != nil {
			return llm.Config{}, err
		}
	}
	return cfg, nil
}

// isGoogle says whether the base URL is Google's Gemini API.
func isGoogle(baseURL string) bool {
	u, err := url.Parse(baseURL)
	return err == nil && strings.EqualFold(u.Hostname(), "generativelanguage.googleapis.com")
}

// Embed sends the texts to the OpenAI compatible /embeddings endpoint of the
// chosen provider.
func (e embedder) Embed(ctx context.Context, providerID int64, model string, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	cfg, err := e.config(ctx, providerID)
	if err != nil {
		return nil, err
	}
	key := cfg.APIKey
	if key == "" {
		key = "local-model"
	}
	client := openai.NewClient(option.WithBaseURL(cfg.BaseURL), option.WithAPIKey(key))
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultEmbedModel
		if isGoogle(cfg.BaseURL) {
			model = GoogleEmbedModel
		}
	}
	out := make([][]float32, 0, len(inputs))
	for start := 0; start < len(inputs); start += maxEmbedInputs {
		end := min(start+maxEmbedInputs, len(inputs))
		res, err := client.Embeddings.New(ctx, openai.EmbeddingNewParams{
			Model: openai.EmbeddingModel(model),
			Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: inputs[start:end]},
		})
		if err != nil {
			return nil, fmt.Errorf("嵌入接口出错：%w", err)
		}
		if len(res.Data) != end-start {
			return nil, fmt.Errorf("嵌入接口返回了 %d 条，要的是 %d 条", len(res.Data), end-start)
		}
		batch := make([][]float32, len(res.Data))
		for _, d := range res.Data {
			if d.Index < 0 || int(d.Index) >= len(batch) {
				return nil, errors.New("嵌入接口返回的顺序不对")
			}
			v := make([]float32, len(d.Embedding))
			for i, x := range d.Embedding {
				v[i] = float32(x)
			}
			batch[d.Index] = v
		}
		out = append(out, batch...)
	}
	return out, nil
}
