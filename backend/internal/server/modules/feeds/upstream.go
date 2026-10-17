package feeds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/feeds/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// callTimeout is how long one request to the small service may take.
const callTimeout = 15 * time.Second

func dbUpsert(c config, sealed string) db.UpsertFeedsSettingsParams {
	return db.UpsertFeedsSettingsParams{AgentID: c.AgentID, BaseUrl: c.BaseURL, TokenEnc: sealed, UpdatedAt: nowUTC()}
}

var (
	errUnauthorized = httpx.NewError(http.StatusBadGateway, "feeds_unauthorized", "资讯接口不认这个令牌。请在设置里重新填写接口令牌")
	errBadReply     = httpx.NewError(http.StatusBadGateway, "feeds_unreachable", "资讯接口返回的内容格式不对")
)

func unreachable(detail string) *httpx.Error {
	msg := "资讯接口没有响应。请检查虚拟机上的 x-console-feeds 服务"
	if detail != "" {
		msg += "（" + detail + "）"
	}
	return httpx.NewError(http.StatusBadGateway, "feeds_unreachable", msg)
}

// get asks the small service for path (one of the fixed paths built by the
// handlers) with the query parameters in q, and decodes the JSON answer into
// out. The browser never chooses the address: the handlers pass fixed paths
// and values they validated themselves.
func (m *Module) get(ctx context.Context, path string, q url.Values, out any) error {
	cfg, err := m.loadConfig(ctx)
	if err != nil {
		return err
	}
	if !cfg.configured() {
		return errNotConfigured
	}
	target := cfg.BaseURL + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	var res protocol.HTTPProxyResult
	err = m.d.Agents.Call(ctx, cfg.AgentID, protocol.MethodHTTPProxy, protocol.HTTPProxyParams{
		Method: http.MethodGet,
		URL:    target,
		Header: map[string][]string{
			"Authorization": {"Bearer " + cfg.Token},
			"Accept":        {"application/json"},
		},
		TimeoutSeconds: int(callTimeout / time.Second),
	}, &res)
	if err != nil {
		return m.callError(ctx, cfg.AgentID, err)
	}
	switch {
	case res.Status == http.StatusOK:
	case res.Status == http.StatusUnauthorized || res.Status == http.StatusForbidden:
		return errUnauthorized
	case res.Status == http.StatusNotFound && strings.HasPrefix(path, "/v1/news/") && path != "/v1/news/facets":
		return httpx.ErrNotFound
	default:
		return unreachable(fmt.Sprintf("返回了 %d", res.Status))
	}
	if err := json.Unmarshal(res.Body, out); err != nil {
		return errBadReply
	}
	return nil
}

// callError turns what the hub returns into the errors this page documents.
func (m *Module) callError(ctx context.Context, agentID string, err error) error {
	var he *httpx.Error
	if errors.As(err, &he) {
		if he.Code == httpx.ErrAgentOffline.Code {
			return m.offlineError(ctx, agentID)
		}
		if strings.HasPrefix(he.Code, "agent_") {
			return unreachable(he.Message)
		}
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return unreachable("等了 15 秒没有回应")
	}
	return err
}

// offlineError is agent_offline with the machine name, so the page can say
// which machine is down.
func (m *Module) offlineError(ctx context.Context, agentID string) error {
	name := ""
	if a, err := m.d.Agents.Get(context.WithoutCancel(ctx), agentID); err == nil {
		name = a.Name
	}
	e := *httpx.ErrAgentOffline
	e.Message = "虚拟机不在线"
	if name != "" {
		e.Message += "：" + name
	}
	e.Details = map[string]any{"agentId": agentID, "agentName": name}
	return &e
}
