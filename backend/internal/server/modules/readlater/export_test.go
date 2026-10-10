package readlater

import (
	"context"
	"net/url"
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
	m.xclient = newXClient(true)
	m.mu.Unlock()
}

// SetXBases points the X interfaces ("syndication", "graphql", "fxtwitter") at test servers.
func SetXBases(m *Module, bases map[string]string) {
	m.mu.Lock()
	m.xBases = bases
	m.mu.Unlock()
}

// UseStrictClient goes back to the client that refuses local addresses.
func UseStrictClient(m *Module) {
	m.mu.Lock()
	m.client = newFetchClient(false)
	m.xclient = newXClient(false)
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

// Sanitize runs the page cleaner with no picture handling.
func Sanitize(raw, base string) string {
	u, _ := url.Parse(base)
	s := &sanitizer{base: u}
	return s.clean(raw)
}

// CookieHostOK checks where a cookie may be sent, with the strict rules.
func CookieHostOK(target string) error { return (&Module{}).cookieHostOK(target) }

// ParseTweetURL reads the address of a post.
func ParseTweetURL(raw string) (handle, id string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	return parseTweetURL(u)
}

var SyndicationToken = syndicationToken

// Archive cleans a page and saves its pictures.
func Archive(m *Module, ctx context.Context, id int64, pageURL, raw string) (string, map[string]bool) {
	u, _ := url.Parse(pageURL)
	return m.archive(ctx, id, u, raw)
}
