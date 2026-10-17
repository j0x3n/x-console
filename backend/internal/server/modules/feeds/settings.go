package feeds

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/feeds/api"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// defaultBaseURL is where the small service listens on the VM.
const defaultBaseURL = "http://127.0.0.1:8790"

// errNotConfigured is answered when the page is opened before the settings.
var errNotConfigured = httpx.NewError(http.StatusConflict, "not_configured", "还没配置资讯接口，请到设置里选一台机器")

// config is the stored setup. Token is the plain token, only kept in memory
// for the length of one request.
type config struct {
	AgentID string
	BaseURL string
	Token   string
}

func (c config) configured() bool { return c.AgentID != "" && c.BaseURL != "" && c.Token != "" }

// loadConfig reads the settings row. A missing row is an empty config.
func (m *Module) loadConfig(ctx context.Context) (config, error) {
	row, err := m.q.GetFeedsSettings(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return config{}, nil
	}
	if err != nil {
		return config{}, err
	}
	c := config{AgentID: row.AgentID, BaseURL: row.BaseUrl}
	if row.TokenEnc != "" {
		if c.Token, err = m.d.Secrets.Open(row.TokenEnc); err != nil {
			return config{}, err
		}
	}
	return c, nil
}

func toAPI(c config) api.FeedsSettings {
	base := c.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	return api.FeedsSettings{AgentId: c.AgentID, BaseUrl: base, HasToken: c.Token != "", Configured: c.configured()}
}

// GetFeedsSettings implements api.ServerInterface. The token is never sent.
func (m *Module) GetFeedsSettings(w http.ResponseWriter, r *http.Request) {
	c, err := m.loadConfig(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPI(c))
}

// PutFeedsSettings implements api.ServerInterface. The settings decide which
// address on the agent's network the panel talks to, so it needs elevation.
func (m *Module) PutFeedsSettings(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.FeedsSettingsInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cfg, tokenChanged, err := m.saveConfig(r.Context(), body)
	target := strings.TrimSpace(body.AgentId)
	detail := map[string]any{"baseUrl": cfg.BaseURL, "tokenChanged": tokenChanged}
	if target == "" {
		detail = map[string]any{"cleared": true}
	}
	m.d.Audit.Record(r.Context(), "feeds.settings", target, detail, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPI(cfg))
}

func (m *Module) saveConfig(ctx context.Context, in api.FeedsSettingsInput) (config, bool, error) {
	agentID := strings.TrimSpace(in.AgentId)
	if agentID == "" {
		// No machine means the integration is switched off, token included.
		return config{}, false, m.q.DeleteFeedsSettings(ctx)
	}
	if err := m.checkAgent(ctx, agentID); err != nil {
		return config{}, false, err
	}
	base := defaultBaseURL
	if in.BaseUrl != nil && strings.TrimSpace(*in.BaseUrl) != "" {
		var err error
		if base, err = normalizeURL(*in.BaseUrl); err != nil {
			return config{}, false, err
		}
	}
	old, err := m.loadConfig(ctx)
	if err != nil {
		return config{}, false, err
	}
	cfg := config{AgentID: agentID, BaseURL: base, Token: old.Token}
	changed := false
	if in.Token != nil && strings.TrimSpace(*in.Token) != "" {
		cfg.Token = strings.TrimSpace(*in.Token)
		changed = true
	}
	if cfg.Token == "" {
		return cfg, false, httpx.Invalid("请填写接口令牌")
	}
	sealed, err := m.d.Secrets.Seal(cfg.Token)
	if err != nil {
		return cfg, false, err
	}
	err = m.q.UpsertFeedsSettings(ctx, dbUpsert(cfg, sealed))
	return cfg, changed, err
}

// checkAgent makes sure the machine exists and can forward requests. The
// capability list is the one the agent announced when it last connected.
func (m *Module) checkAgent(ctx context.Context, agentID string) error {
	a, err := m.d.Agents.Get(ctx, agentID)
	if errors.Is(err, httpx.ErrNotFound) {
		return httpx.Invalid("这台机器不存在或已吊销")
	}
	if err != nil {
		return err
	}
	if !a.Has(protocol.CapProxy) {
		return httpx.Invalid("这台机器的代理不支持转发。请把代理程序升级到新版本")
	}
	return nil
}

// normalizeURL accepts only http:// and https:// addresses and strips a
// trailing slash.
func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", httpx.Invalid("地址要以 http:// 或 https:// 开头，例如 " + defaultBaseURL)
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", httpx.Invalid("地址里不要带查询参数、锚点或用户名")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func nowUTC() time.Time { return time.Now().UTC() }
