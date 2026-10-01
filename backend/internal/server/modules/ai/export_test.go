package ai

import (
	"context"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"time"
)

// ParseModelsDevForTest exposes the catalog parser to external module tests.
func ParseModelsDevForTest(raw []byte) (any, error) { return parseModelsDev(raw) }

// LLMForTest exposes the configured call boundary to external module tests.
func (m *Module) LLMForTest() llm.Client { return m.llm }

// SetNowForTest lets external integration tests advance permission expiry.
func (m *Module) SetNowForTest(now func() time.Time) { m.now = now }

// UsageCostForTest exposes the B42 price calculation.
func UsageCostForTest(input, cached, cacheWrite, output int64, in, out, read, write *float32) (*float64, bool) {
	return usageCost(input, cached, cacheWrite, output, usagePrices{Input: in, Output: out, CacheRead: read, CacheWrite: write})
}

// SetLLMForTest replaces the call boundary, for example with llm.NewFake (B47).
func (m *Module) SetLLMForTest(c llm.Client) { m.llm = c }

// PolishPromptForTest exposes the B56 prompt builder.
func PolishPromptForTest(scene, request string) (string, bool) {
	return polishPrompt(api.PolishScene(scene), request)
}

// HostSystemForTest exposes the machine conversation prompt (B61).
func (m *Module) HostSystemForTest(ctx context.Context, hostID string) string {
	return m.hostSystem(ctx, hostID)
}
