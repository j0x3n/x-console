package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

// DefaultEmbedModel is used when the caller names no model.
const DefaultEmbedModel = "text-embedding-3-small"

// maxEmbedInputs is how many texts go in one request (the API allows 2048).
const maxEmbedInputs = 128

// Embedder returns the module as a contracts.Embedder.
func (m *Module) embedder() contracts.Embedder { return embedder{m} }

type embedder struct{ m *Module }

// Available says whether a provider for fast AI calls is configured.
func (e embedder) Available(ctx context.Context) bool {
	_, err := e.m.resolveLLM(ctx, "fast")
	return err == nil
}

// Embed sends the texts to the OpenAI compatible /embeddings endpoint of the
// provider chosen for fast AI calls.
func (e embedder) Embed(ctx context.Context, model string, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	cfg, err := e.m.resolveLLM(ctx, "fast")
	if err != nil {
		if errors.Is(err, llm.ErrNotConfigured) {
			return nil, errors.New("还没有配置 AI 供应商，不能建索引")
		}
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
