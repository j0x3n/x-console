package mail

import (
	"context"
	"crypto/tls"
	"time"
)

// Hooks for the external test package.

// SetTLSConfig makes IMAP connections trust the test server.
func SetTLSConfig(m *Module, cfg *tls.Config) { m.tlsConfig = cfg }

// SetTimings shortens the reconnect and poll waits.
func SetTimings(m *Module, retry, poll time.Duration) {
	m.retry = []time.Duration{retry}
	m.poll = poll
}

// FileCount counts stored files under prefix.
func FileCount(m *Module, prefix string) int {
	n := 0
	for _, err := range m.files.List(context.Background(), prefix) {
		if err == nil {
			n++
		}
	}
	return n
}

// SnippetText exposes the snippet decoder.
func SnippetText(raw []byte, encoding, charset string, html bool) string {
	return snippetText(raw, encoding, charset, html)
}
