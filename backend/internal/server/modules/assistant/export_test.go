package assistant

// SetTestEndpoint redirects Claude calls to a local HTTP server in integration tests.
func (m *Module) SetTestEndpoint(url string) { m.baseURL = url }
