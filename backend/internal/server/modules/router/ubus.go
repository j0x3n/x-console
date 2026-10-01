package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/agenthub"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// maxBody bounds one ubus reply. A host hints dump on a busy network is a
// few hundred KB.
const maxBody = 8 << 20

// anonSession is the ubus session id used to log in.
const anonSession = "00000000000000000000000000000000"

// errAccessDenied is the JSON-RPC error uhttpd returns for an expired or
// unknown session.
const errAccessDenied = -32002

// transport sends one HTTP request to the router: from the server, or
// through a paired agent on the home network.
type transport interface {
	do(ctx context.Context, url string, body []byte) (int, []byte, error)
}

type direct struct{ client *http.Client }

func newDirect() *direct {
	return &direct{client: &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (d *direct) do(ctx context.Context, url string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return 0, nil, err
	}
	if len(raw) > maxBody {
		return 0, nil, errors.New("路由器返回的内容太大")
	}
	return resp.StatusCode, raw, nil
}

// viaAgent uses the agent's http.proxy, which only reaches private addresses.
type viaAgent struct {
	hub     *agenthub.Hub
	agentID string
}

func (v *viaAgent) do(ctx context.Context, url string, body []byte) (int, []byte, error) {
	var res protocol.HTTPProxyResult
	err := v.hub.Call(ctx, v.agentID, protocol.MethodHTTPProxy, protocol.HTTPProxyParams{
		Method: http.MethodPost, URL: url, Body: body,
		Header: map[string][]string{"Content-Type": {"application/json"}},
	}, &res)
	if err != nil {
		return 0, nil, err
	}
	return res.Status, res.Body, nil
}

// ubus talks JSON-RPC to uhttpd's /ubus endpoint. It logs in on first use
// and again when the session expires.
type ubus struct {
	url      string // http://router/ubus
	username string
	password string
	t        transport

	mu      sync.Mutex
	session string
	expires time.Time
	id      int
}

// ubusError is a non-zero ubus status code or a JSON-RPC error.
type ubusError struct {
	object, method string
	code           int
	rpc            bool // a JSON-RPC error, not a ubus status
	message        string
}

func (e *ubusError) Error() string {
	if e.rpc {
		if e.code == errAccessDenied {
			return fmt.Sprintf("路由器拒绝了 %s %s：没有权限，检查 rpcd 的 ACL", e.object, e.method)
		}
		return fmt.Sprintf("路由器返回错误 %s %s：%s", e.object, e.method, e.message)
	}
	return fmt.Sprintf("路由器返回错误 %s %s：%s", e.object, e.method, ubusStatus(e.code))
}

// ubusStatus names the ubus status codes (libubus UBUS_STATUS_*).
func ubusStatus(code int) string {
	switch code {
	case 1:
		return "命令无效"
	case 2:
		return "参数无效"
	case 3:
		return "没有这个方法"
	case 4:
		return "没有这个对象，可能没装对应的软件包"
	case 5:
		return "没有数据"
	case 6:
		return "没有权限，检查 rpcd 的 ACL"
	case 7:
		return "超时"
	case 8:
		return "不支持"
	case 10:
		return "连接失败"
	default:
		return fmt.Sprintf("错误码 %d", code)
	}
}

// netError marks a failure to reach the router at all.
type netError struct{ err error }

func (e *netError) Error() string { return "连不上路由器：" + e.err.Error() }
func (e *netError) Unwrap() error { return e.err }

// unreachable reports whether err means the router could not be reached.
func unreachable(err error) bool {
	var n *netError
	return errors.As(err, &n)
}

// call runs object.method with args and decodes the reply into out.
func (u *ubus) call(ctx context.Context, object, method string, args, out any) error {
	session, err := u.login(ctx, false)
	if err != nil {
		return err
	}
	err = u.raw(ctx, session, object, method, args, out)
	var ue *ubusError
	if errors.As(err, &ue) && ue.rpc && ue.code == errAccessDenied {
		// The session may have expired early, for example after rpcd restarted.
		if session, err = u.login(ctx, true); err != nil {
			return err
		}
		return u.raw(ctx, session, object, method, args, out)
	}
	return err
}

// login returns a valid session, logging in when needed.
func (u *ubus) login(ctx context.Context, force bool) (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !force && u.session != "" && time.Now().Before(u.expires) {
		return u.session, nil
	}
	var res struct {
		Session string `json:"ubus_rpc_session"`
		Expires int    `json:"expires"`
	}
	err := u.rawLocked(ctx, anonSession, "session", "login", map[string]string{"username": u.username, "password": u.password}, &res)
	var ue *ubusError
	if errors.As(err, &ue) {
		return "", errors.New("登录路由器失败：用户名或密码不对，或者 rpcd 里没有这个用户")
	}
	if err != nil {
		return "", err
	}
	if res.Session == "" {
		return "", errors.New("登录路由器失败：没有拿到会话")
	}
	ttl := time.Duration(res.Expires) * time.Second
	if ttl <= 0 {
		ttl = 300 * time.Second
	}
	u.session = res.Session
	// Renew a little early so a call never races the expiry.
	u.expires = time.Now().Add(ttl * 4 / 5)
	return u.session, nil
}

func (u *ubus) raw(ctx context.Context, session, object, method string, args, out any) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.rawLocked(ctx, session, object, method, args, out)
}

// rawLocked sends one request. It holds the lock to number requests, which
// also keeps at most one request in flight: routers are slow and small.
func (u *ubus) rawLocked(ctx context.Context, session, object, method string, args, out any) error {
	if args == nil {
		args = map[string]any{}
	}
	u.id++
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": u.id, "method": "call",
		"params": []any{session, object, method, args},
	})
	if err != nil {
		return err
	}
	status, raw, err := u.t.do(ctx, u.url, body)
	if err != nil {
		return &netError{err}
	}
	if status != http.StatusOK {
		return &netError{fmt.Errorf("HTTP %d，检查地址和 uhttpd-mod-ubus", status)}
	}
	var res struct {
		Result []json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return &netError{errors.New("返回的不是 ubus 的 JSON，检查地址和 uhttpd-mod-ubus")}
	}
	if res.Error != nil {
		return &ubusError{object: object, method: method, code: res.Error.Code, rpc: true, message: res.Error.Message}
	}
	if len(res.Result) == 0 {
		return &ubusError{object: object, method: method, code: 9}
	}
	var code int
	if err := json.Unmarshal(res.Result[0], &code); err != nil {
		return &ubusError{object: object, method: method, code: 9}
	}
	if code != 0 {
		return &ubusError{object: object, method: method, code: code}
	}
	if out == nil || len(res.Result) < 2 {
		return nil
	}
	if err := json.Unmarshal(res.Result[1], out); err != nil {
		return fmt.Errorf("%s %s 的返回看不懂：%w", object, method, err)
	}
	return nil
}
