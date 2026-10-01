package core

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/core/api"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// clientErrorLimit is how many browser errors one session may log per minute.
const clientErrorLimit = 30

// clientErrorLog counts browser error reports per session (B41).
type clientErrorLog struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

// allow records a report for key and says whether it is within the limit.
func (l *clientErrorLog) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[string][]time.Time{}
	}
	cutoff := now.Add(-time.Minute)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= clientErrorLimit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	// Drop idle sessions so the map does not grow forever.
	if len(l.hits) > 1000 {
		for k, v := range l.hits {
			if len(v) == 0 || v[len(v)-1].Before(cutoff) {
				delete(l.hits, k)
			}
		}
	}
	return true
}

func (h *Handlers) ReportClientError(w http.ResponseWriter, r *http.Request) {
	var body api.ReportClientErrorJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	key := ""
	if s := auth.FromContext(r.Context()); s != nil {
		key = s.ID
	}
	if !h.clientErrors.allow(key, time.Now()) {
		httpx.NoContent(w)
		return
	}
	slog.WarnContext(r.Context(), "client error",
		"title", clip(body.Title, 200),
		"message", clip(body.Message, 2000),
		"page", clip(deref(body.Page), 500),
		"request_id", clip(deref(body.RequestId), 100),
		"detail", clip(deref(body.Detail), 8192),
		"user_agent", clip(r.UserAgent(), 300),
	)
	httpx.NoContent(w)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
