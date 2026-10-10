package readlater

import (
	"context"
	"time"
)

// Hooks for the integration tests in package readlater_test, which cannot live
// in this package because testutil imports app, which imports readlater.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

// AllowPrivate lets the module fetch from a local test server.
func AllowPrivate(m *Module) {
	m.mu.Lock()
	m.allowPrivate = true
	m.client = newFetchClient(true)
	m.mu.Unlock()
}

// UseStrictClient goes back to the client that refuses local addresses.
func UseStrictClient(m *Module) {
	m.mu.Lock()
	m.client = newFetchClient(false)
	m.mu.Unlock()
}

func Digest(m *Module, ctx context.Context) error { return m.digest(ctx) }

func RetryStale(m *Module, ctx context.Context) error { return m.retryStale(ctx) }

var (
	NormalizeURL   = normalizeURL
	ExtractLinks   = extractLinks
	BlockedAddr    = blockedAddr
	NewFetchClient = newFetchClient
)
