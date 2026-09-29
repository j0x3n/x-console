package ai

import "github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"

// ParseModelsDevForTest exposes the catalog parser to external module tests.
func ParseModelsDevForTest(raw []byte) (any, error) { return parseModelsDev(raw) }

// LLMForTest exposes the configured call boundary to external module tests.
func (m *Module) LLMForTest() llm.Client { return m.llm }
