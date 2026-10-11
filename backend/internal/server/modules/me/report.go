package me

// The phone report: a shortcut on the iPhone (or Home Assistant, or a script)
// posts one of four events with a bearer token. The route has no session, so
// the token is the only protection. Only its SHA-256 is stored.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/me/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const (
	reportKey    = "me.report"
	reportPath   = "/me/report"
	maxReport    = 1 << 10
	maxFutureAt  = 5 * time.Minute
	maxPastAt    = 12 * time.Hour
	reportSource = srcPhone
)

// reportToken is the stored state of the token. The token itself is never
// stored.
type reportToken struct {
	Hash         string     `json:"hash"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastReportAt *time.Time `json:"lastReportAt,omitempty"`
	LastEvent    string     `json:"lastEvent,omitempty"`
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (m *Module) loadToken(ctx context.Context) (reportToken, error) {
	var t reportToken
	if err := m.d.Settings.Get(ctx, reportKey, &t); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return reportToken{}, err
	}
	return t, nil
}

func (m *Module) reportURL(r *http.Request) string {
	base := strings.TrimRight(m.d.Config.PublicURL, "/")
	if base == "" {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return base + "/api/v1" + reportPath
}

// GetMeReport implements api.ServerInterface.
func (m *Module) GetMeReport(w http.ResponseWriter, r *http.Request) {
	t, err := m.loadToken(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := api.MeReportStatus{Enabled: t.Hash != "", ReportUrl: m.reportURL(r)}
	if t.Hash != "" {
		created := t.CreatedAt
		out.CreatedAt = &created
		out.LastReportAt = t.LastReportAt
		if t.LastEvent != "" {
			event := t.LastEvent
			out.LastEvent = &event
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// CreateMeReportToken implements api.ServerInterface. The old token stops
// working at once.
func (m *Module) CreateMeReportToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	token := hex.EncodeToString(raw)
	m.tokenMu.Lock()
	err := m.d.Settings.Set(ctx, reportKey, reportToken{Hash: hashToken(token), CreatedAt: time.Now().UTC()})
	m.tokenMu.Unlock()
	m.d.Audit.Record(ctx, "me.report_token", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	url := m.reportURL(r)
	example := "curl -X POST '" + url + "' \\\n  -H 'Authorization: Bearer " + token + "' \\\n  -H 'Content-Type: application/json' \\\n  -d '{\"event\": \"sleep.start\"}'"
	httpx.JSON(w, http.StatusOK, api.MeReportToken{Token: token, ReportUrl: url, Example: example})
}

// DeleteMeReport implements api.ServerInterface.
func (m *Module) DeleteMeReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m.tokenMu.Lock()
	err := m.d.Settings.Delete(ctx, reportKey)
	m.tokenMu.Unlock()
	m.d.Audit.Record(ctx, "me.report_token_delete", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func bearerOf(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// ReportMe implements api.ServerInterface: one event from the phone.
func (m *Module) ReportMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, err := m.loadToken(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	token := bearerOf(r)
	if t.Hash == "" || token == "" || subtle.ConstantTimeCompare([]byte(hashToken(token)), []byte(t.Hash)) != 1 {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxReport))
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid("上报内容读不出来或者太大"))
		return
	}
	event, at, err := parseReport(body, m.now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	_, err = m.ingest(ctx, event, reportSource, at)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	m.tokenMu.Lock()
	// Keep the token that was checked: a new one may have been made meanwhile.
	if cur, e := m.loadToken(ctx); e == nil && cur.Hash == t.Hash {
		cur.LastReportAt, cur.LastEvent = &now, event
		_ = m.d.Settings.Set(ctx, reportKey, cur)
	}
	m.tokenMu.Unlock()
	m.d.Audit.Record(ctx, "me.report", event, nil, nil)
	httpx.NoContent(w)
}

// parseReport reads the JSON of a report. Anything unknown is refused.
func parseReport(body []byte, now time.Time) (string, time.Time, error) {
	var in struct {
		Event string  `json:"event"`
		At    *string `json:"at"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || dec.More() {
		return "", time.Time{}, httpx.Invalid("请求体要是 JSON 对象，只有 event 和 at 两个字段")
	}
	switch in.Event {
	case evSleepStart, evSleepEnd, evHomeArrive, evHomeLeave:
	default:
		return "", time.Time{}, httpx.Invalid("不认识的事件，只收 sleep.start、sleep.end、home.arrive、home.leave")
	}
	at := now
	if in.At != nil && strings.TrimSpace(*in.At) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*in.At))
		if err != nil {
			return "", time.Time{}, httpx.Invalid("at 要写成 2026-10-11T03:40:00+08:00 这样")
		}
		if t.After(now.Add(maxFutureAt)) || t.Before(now.Add(-maxPastAt)) {
			return "", time.Time{}, httpx.Invalid("at 不能比现在晚 5 分钟以上，也不能比现在早 12 小时以上")
		}
		at = t
	}
	return in.Event, at, nil
}
