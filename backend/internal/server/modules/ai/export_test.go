package ai

// ParseModelsDevForTest exposes the catalog parser to external module tests.
func ParseModelsDevForTest(raw []byte) (any, error) { return parseModelsDev(raw) }
