package llm

import (
	"encoding/json"

	"github.com/openai/openai-go/v3"
)

// setUsage fills the token counts (B42). input counts what the provider
// reported as input; cached comes from the OpenAI style details. Providers
// that answer in the Anthropic style report cache reads and writes in
// separate fields and leave them out of input, so they are added back:
// InputTokens always covers every input token.
func (r *Result) setUsage(input, output, cached, reasoning, cacheRead, cacheWrite int64) {
	r.InputTokens, r.OutputTokens, r.ReasoningTokens = input, output, reasoning
	r.CachedInputTokens, r.CacheWriteTokens = cached, 0
	if cacheRead > 0 || cacheWrite > 0 {
		r.InputTokens += cacheRead + cacheWrite
		r.CachedInputTokens = cacheRead
		r.CacheWriteTokens = cacheWrite
	}
}

// anthropicUsage holds the extra fields some compatible endpoints add.
type anthropicUsage struct {
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

func (r *Result) setChatUsage(u openai.CompletionUsage) {
	var extra anthropicUsage
	if raw := u.RawJSON(); raw != "" {
		_ = json.Unmarshal([]byte(raw), &extra)
	}
	r.setUsage(u.PromptTokens, u.CompletionTokens, u.PromptTokensDetails.CachedTokens,
		u.CompletionTokensDetails.ReasoningTokens, extra.CacheReadInputTokens, extra.CacheCreationInputTokens)
}
