package readlater

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/readlater/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Settings for reading X posts (B143). The cookie is kept encrypted; the rest
// is plain.
const (
	xCookieKey = "readlater.x.cookie"
	xConfigKey = "readlater.x.config"
)

type xCookie struct {
	AuthToken string `json:"authToken"`
	CT0       string `json:"ct0"`
}

type xConfig struct {
	QueryID    string     `json:"queryId,omitempty"`
	Fxtwitter  bool       `json:"fxtwitter,omitempty"`
	Status     string     `json:"status,omitempty"` // ok, expired, error
	Message    string     `json:"message,omitempty"`
	VerifiedAt *time.Time `json:"verifiedAt,omitempty"`
	NotifiedAt *time.Time `json:"notifiedAt,omitempty"`
}

var cookieValue = regexp.MustCompile(`^[A-Za-z0-9%_.\-=+/]{8,512}$`)

func (m *Module) xConfig(ctx context.Context) xConfig {
	var c xConfig
	if err := m.d.Settings.Get(ctx, xConfigKey, &c); err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("readlater x config", "err", err)
	}
	return c
}

func (m *Module) saveXConfig(ctx context.Context, c xConfig) error {
	return m.d.Settings.Set(ctx, xConfigKey, c)
}

// xCookie returns the saved cookie, if there is one.
func (m *Module) xCookie(ctx context.Context) (xCookie, bool) {
	var c xCookie
	if err := m.d.Settings.Get(ctx, xCookieKey, &c); err != nil || c.AuthToken == "" || c.CT0 == "" {
		return xCookie{}, false
	}
	return c, true
}

func (m *Module) xState(ctx context.Context) api.XAuth {
	cfg := m.xConfig(ctx)
	_, has := m.xCookie(ctx)
	out := api.XAuth{Configured: has, Status: "none", Fxtwitter: cfg.Fxtwitter, QueryId: cfg.QueryID, VerifiedAt: cfg.VerifiedAt}
	if has {
		out.Status = "unverified"
		switch cfg.Status {
		case "ok":
			out.Status = "ok"
		case "expired":
			out.Status = "expired"
		case "error":
			out.Status = "error"
		}
		if cfg.Message != "" {
			msg := cfg.Message
			out.Message = &msg
		}
	}
	return out
}

// xExpired records that X refused the cookie, and tells the user at most once a day.
func (m *Module) xExpired(ctx context.Context, cfg xConfig) {
	now := m.now().UTC()
	cfg.Status, cfg.Message = "expired", "X 拒绝了登录 Cookie，可能过期了，请重新填写"
	send := cfg.NotifiedAt == nil || now.Sub(*cfg.NotifiedAt) > 24*time.Hour
	if send {
		cfg.NotifiedAt = &now
	}
	if err := m.saveXConfig(ctx, cfg); err != nil {
		m.d.Log.Warn("readlater x config", "err", err)
		return
	}
	if send {
		_, _ = m.d.Notify.Send(ctx, notify.Notification{
			Kind: "readlater.xauth", Source: "readlater", Link: "/settings",
			Title: "X 登录 Cookie 可能过期了", Body: "存推文时 X 拒绝了 Cookie。请在设置里重新填写。", Priority: notify.PriorityNormal,
		})
	}
}

// GetReadXAuth implements api.ServerInterface.
func (m *Module) GetReadXAuth(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, m.xState(r.Context()))
}

// SaveReadXAuth implements api.ServerInterface.
func (m *Module) SaveReadXAuth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.SaveReadXAuthJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err := m.saveXAuth(ctx, body)
	// the audit record names what changed, never the cookie
	m.d.Audit.Record(ctx, "readlater.xauth.save", "", map[string]any{"cookie": body.AuthToken != nil || body.Ct0 != nil, "fxtwitter": body.Fxtwitter != nil}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m.xState(ctx))
}

func (m *Module) saveXAuth(ctx context.Context, body api.XAuthInput) error {
	cfg := m.xConfig(ctx)
	if body.AuthToken != nil || body.Ct0 != nil {
		if body.AuthToken == nil || body.Ct0 == nil {
			return httpx.Invalid("auth_token 和 ct0 要一起填")
		}
		c := xCookie{AuthToken: strings.TrimSpace(*body.AuthToken), CT0: strings.TrimSpace(*body.Ct0)}
		if !cookieValue.MatchString(c.AuthToken) || !cookieValue.MatchString(c.CT0) {
			return httpx.Invalid("auth_token 或 ct0 的格式不对，只填值，不要带名字和分号")
		}
		if err := m.d.Settings.SetSecret(ctx, xCookieKey, c); err != nil {
			return err
		}
		cfg.Status, cfg.Message, cfg.VerifiedAt, cfg.NotifiedAt = "", "", nil, nil
	}
	if body.Fxtwitter != nil {
		cfg.Fxtwitter = *body.Fxtwitter
	}
	if body.QueryId != nil {
		q := strings.TrimSpace(*body.QueryId)
		if q != "" && !regexp.MustCompile(`^[A-Za-z0-9_-]{6,64}$`).MatchString(q) {
			return httpx.Invalid("queryId 的格式不对")
		}
		cfg.QueryID = q
	}
	return m.saveXConfig(ctx, cfg)
}

// ClearReadXAuth implements api.ServerInterface.
func (m *Module) ClearReadXAuth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err := m.d.Settings.Delete(ctx, xCookieKey)
	if err == nil {
		cfg := m.xConfig(ctx)
		cfg.Status, cfg.Message, cfg.VerifiedAt, cfg.NotifiedAt = "", "", nil, nil
		err = m.saveXConfig(ctx, cfg)
	}
	m.d.Audit.Record(ctx, "readlater.xauth.clear", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m.xState(ctx))
}

// TestReadXAuth implements api.ServerInterface.
func (m *Module) TestReadXAuth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cookie, ok := m.xCookie(ctx)
	if !ok {
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "no_cookie", "还没有填 X 登录 Cookie"))
		return
	}
	cfg := m.xConfig(ctx)
	_, err := m.tweetGraphQL(ctx, testTweetID, cookie, cfg.QueryID)
	now := m.now().UTC()
	switch {
	case err == nil:
		cfg.Status, cfg.Message, cfg.VerifiedAt = "ok", "", &now
	case errors.Is(err, errXAuth):
		cfg.Status, cfg.Message = "expired", err.Error()+"，可能过期了"
	default:
		cfg.Status, cfg.Message = "error", err.Error()
	}
	saveErr := m.saveXConfig(ctx, cfg)
	m.d.Audit.Record(ctx, "readlater.xauth.test", "", map[string]any{"result": cfg.Status}, saveErr)
	if saveErr != nil {
		httpx.Fail(w, r, saveErr)
		return
	}
	httpx.JSON(w, http.StatusOK, m.xState(ctx))
}
