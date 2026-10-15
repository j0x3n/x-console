package watchers

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// B208: the browser of a watcher can reach the web through a connected
// agent. The browser talks to the HTTP proxy in this file (on the Docker
// network, never published), and the proxy opens one net.dial stream on the
// agent per connection.

const (
	// EnvEgressListen is the address of the proxy, for example ":8091".
	// Without it the proxy does not start.
	EnvEgressListen = "XC_BROWSER_EGRESS_LISTEN"
	// EnvEgressURL is how the browser container reaches the proxy, for
	// example "http://x-console:8091".
	EnvEgressURL = "XC_BROWSER_EGRESS_URL"

	// maxEgressConns is how many connections one watcher may have open.
	maxEgressConns = 64
	// egressDialWait covers the agent's own 10 second dial timeout.
	egressDialWait = 15 * time.Second
	egressChunk    = 32 << 10
)

var (
	errNoEgress = errors.New("服务器没有开出口转发，请管理员设置 XC_BROWSER_EGRESS_LISTEN 和 XC_BROWSER_EGRESS_URL")
	msgOldAgent = "这台机器的代理太旧，先更新代理"
)

// egressProxy is the HTTP proxy for one Module.
type egressProxy struct {
	m         *Module
	publicURL string // from EnvEgressURL

	mu        sync.Mutex
	srv       *http.Server
	base      context.Context // canceled when the server stops
	passwords map[int64]string
	open      map[int64]int
	lastErr   map[int64]egressFailure
}

type egressFailure struct {
	msg string
	at  time.Time
}

func newEgress(m *Module, publicURL string) *egressProxy {
	return &egressProxy{m: m, publicURL: strings.TrimSpace(publicURL), base: context.Background(),
		passwords: map[int64]string{}, open: map[int64]int{}, lastErr: map[int64]egressFailure{}}
}

// start listens on addr until ctx ends. It returns the address in use.
func (e *egressProxy) start(ctx context.Context, addr string) (net.Addr, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: e, ReadHeaderTimeout: 30 * time.Second}
	bctx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.srv, e.base = srv, bctx
	e.mu.Unlock()
	go func() { _ = srv.Serve(ln) }()
	go func() {
		<-bctx.Done()
		_ = srv.Close()
		cancel()
	}()
	return ln.Addr(), nil
}

func (e *egressProxy) running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.srv != nil
}

// proxyURL is the address given to the browser of watcher id.
func (e *egressProxy) proxyURL(id int64) (string, error) {
	if !e.running() || e.publicURL == "" {
		return "", errNoEgress
	}
	base, err := url.Parse(e.publicURL)
	if err != nil || base.Host == "" {
		return "", errNoEgress
	}
	u := url.URL{Scheme: "http", Host: base.Host, User: url.UserPassword(egressUser(id), e.password(id))}
	return u.String(), nil
}

func egressUser(id int64) string { return "w" + strconv.FormatInt(id, 10) }

// password returns the secret of a watcher, making one the first time. It is
// only kept in memory: after a restart the next run gets a new one.
func (e *egressProxy) password(id int64) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if p, ok := e.passwords[id]; ok {
		return p
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	p := base64.RawURLEncoding.EncodeToString(b)
	e.passwords[id] = p
	return p
}

func (e *egressProxy) forget(id int64) {
	e.mu.Lock()
	delete(e.passwords, id)
	delete(e.lastErr, id)
	e.mu.Unlock()
}

// authorize returns the watcher a request belongs to.
func (e *egressProxy) authorize(r *http.Request) (int64, bool) {
	raw := r.Header.Get("Proxy-Authorization")
	scheme, rest, _ := strings.Cut(raw, " ")
	dec, err := base64.StdEncoding.DecodeString(strings.TrimSpace(rest))
	user, pass, ok := strings.Cut(string(dec), ":")
	id, perr := strconv.ParseInt(strings.TrimPrefix(user, "w"), 10, 64)
	valid := strings.EqualFold(scheme, "Basic") && err == nil && ok && strings.HasPrefix(user, "w") && perr == nil
	e.mu.Lock()
	want, has := e.passwords[id]
	e.mu.Unlock()
	if !valid || !has {
		want = "" // still compare, so the time does not tell which part was wrong
		pass = ""
	}
	a, b := sha256.Sum256([]byte(pass)), sha256.Sum256([]byte(want))
	same := subtle.ConstantTimeCompare(a[:], b[:]) == 1
	return id, valid && has && same
}

// acquire counts one more connection of the watcher.
func (e *egressProxy) acquire(id int64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.open[id] >= maxEgressConns {
		return false
	}
	e.open[id]++
	return true
}

func (e *egressProxy) release(id int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.open[id] <= 1 {
		delete(e.open, id)
		return
	}
	e.open[id]--
}

func (e *egressProxy) fail(id int64, msg string) {
	e.mu.Lock()
	e.lastErr[id] = egressFailure{msg: msg, at: time.Now()}
	e.mu.Unlock()
}

// failedSince returns the last connection failure of the watcher after t.
func (e *egressProxy) failedSince(id int64, t time.Time) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if f, ok := e.lastErr[id]; ok && f.at.After(t) {
		return f.msg
	}
	return ""
}

// ServeHTTP implements http.Handler.
func (e *egressProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := e.authorize(r)
	if !ok {
		w.Header().Set("Proxy-Authenticate", `Basic realm="x-console"`)
		http.Error(w, "需要代理密码", http.StatusProxyAuthRequired)
		return
	}
	row, err := e.m.q.GetWatcher(r.Context(), id)
	if err != nil || row.EgressAgentID == "" {
		http.Error(w, "这个任务没有选出口机器", http.StatusForbidden)
		return
	}
	if !e.acquire(id) {
		http.Error(w, "这个任务的连接太多了", http.StatusServiceUnavailable)
		return
	}
	defer e.release(id)
	if r.Method == http.MethodConnect {
		e.connect(w, r, id, row.EgressAgentID)
		return
	}
	e.plain(w, r, id, row.EgressAgentID)
}

// dial opens a net.dial stream and waits until the agent has connected. The
// first frame is returned too, because it could carry data.
func (e *egressProxy) dial(ctx context.Context, id int64, agentID, host string, port int) (*rpc.Stream, []byte, int, string) {
	ctx, cancel := context.WithTimeout(ctx, egressDialWait)
	defer cancel()
	s, err := e.m.d.Agents.Open(ctx, agentID, protocol.MethodNetDial, protocol.NetDialParams{Host: host, Port: port})
	if err == nil {
		var first []byte
		if first, err = s.Recv(ctx); err == nil {
			return s, first, 0, ""
		}
		s.Close(nil)
	}
	msg := egressMessage(err, host, port)
	e.m.d.Log.Warn("watcher egress", "watcher", id, "agent", agentID, "target", host, "err", err)
	e.fail(id, msg)
	return nil, nil, http.StatusBadGateway, msg
}

// egressMessage turns a failure into a sentence for the user.
func egressMessage(err error, host string, port int) string {
	var pe *protocol.Error
	var he *httpx.Error
	switch {
	case errors.Is(err, httpx.ErrAgentOffline), errors.Is(err, rpc.ErrClosed):
		return "出口机器不在线"
	case errors.As(err, &pe):
		switch pe.Code {
		case protocol.CodeUnknownMethod:
			return msgOldAgent
		case protocol.CodeUnsupported, protocol.CodeForbiddenTarget:
			return pe.Message
		}
		return fmt.Sprintf("出口机器连不上 %s:%d", host, port)
	case errors.As(err, &he):
		if he.Code == "agent_unknown_method" || he.Code == "agent_unsupported" {
			return msgOldAgent
		}
		return he.Message
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("出口机器连 %s:%d 超时", host, port)
	}
	return fmt.Sprintf("出口机器连不上 %s:%d", host, port)
}

// connect handles CONNECT host:port: a tunnel for HTTPS and WebSocket.
func (e *egressProxy) connect(w http.ResponseWriter, r *http.Request, id int64, agentID string) {
	host, port, err := splitTarget(r.Host, 0)
	if err != nil {
		http.Error(w, "目标地址不对", http.StatusBadRequest)
		return
	}
	s, first, status, msg := e.dial(r.Context(), id, agentID, host, port)
	if s == nil {
		http.Error(w, msg, status)
		return
	}
	defer s.Close(nil)
	c, brw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		return
	}
	defer c.Close()
	if _, err := io.WriteString(c, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if len(first) > 0 {
		if _, err := c.Write(first); err != nil {
			return
		}
	}
	e.tunnel(c, brw.Reader, s)
}

// tunnel copies between the browser and the stream until one side ends.
func (e *egressProxy) tunnel(c net.Conn, in io.Reader, s *rpc.Stream) {
	e.mu.Lock()
	base := e.base
	e.mu.Unlock()
	ctx, cancel := context.WithCancel(base)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	done := make(chan struct{}, 2)
	go func() { // browser -> agent
		defer func() { done <- struct{}{} }()
		buf := make([]byte, egressChunk)
		for {
			n, err := in.Read(buf)
			if n > 0 && s.Send(ctx, append([]byte(nil), buf[:n]...)) != nil {
				return
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { // agent -> browser
		defer func() { done <- struct{}{} }()
		for {
			b, err := s.Recv(ctx)
			if err != nil {
				return
			}
			if len(b) > 0 {
				if _, err := c.Write(b); err != nil {
					return
				}
			}
		}
	}()
	<-done
	cancel()
	<-done
}

// splitTarget reads host:port. defaultPort is used when the port is missing;
// 0 means a port is required.
func splitTarget(hostport string, defaultPort int) (string, int, error) {
	host, p, err := net.SplitHostPort(hostport)
	if err != nil {
		if defaultPort == 0 {
			return "", 0, err
		}
		host, p = strings.Trim(hostport, "[]"), strconv.Itoa(defaultPort)
	}
	port, err := strconv.Atoi(p)
	if err != nil || host == "" || port < 1 || port > 65535 {
		return "", 0, errors.New("bad target")
	}
	return host, port, nil
}

// hopHeaders are not passed on by a proxy.
var hopHeaders = []string{"Connection", "Proxy-Connection", "Proxy-Authorization", "Proxy-Authenticate", "Keep-Alive", "Te", "Trailer", "Transfer-Encoding", "Upgrade"}

// plain handles a request with an absolute address, such as
// GET http://example.com/ HTTP/1.1. One request per connection to the target.
func (e *egressProxy) plain(w http.ResponseWriter, r *http.Request, id int64, agentID string) {
	if r.URL.Scheme != "http" || r.URL.Host == "" {
		http.Error(w, "代理请求要写完整的 http:// 地址", http.StatusBadRequest)
		return
	}
	host, port, err := splitTarget(r.URL.Host, 80)
	if err != nil {
		http.Error(w, "目标地址不对", http.StatusBadRequest)
		return
	}
	s, first, status, msg := e.dial(r.Context(), id, agentID, host, port)
	if s == nil {
		http.Error(w, msg, status)
		return
	}
	defer s.Close(nil)

	// Relative address, no proxy headers, one request per connection.
	r.RequestURI = ""
	for _, h := range []string{"Connection", "Proxy-Connection", "Proxy-Authorization", "Keep-Alive"} {
		r.Header.Del(h)
	}
	r.Close = true
	ctx := r.Context()
	if err := r.Write(&streamWriter{ctx: ctx, s: s}); err != nil {
		e.fail(id, "出口机器上传请求时断了")
		http.Error(w, "出口机器上传请求时断了", http.StatusBadGateway)
		return
	}
	resp, err := http.ReadResponse(bufio.NewReader(&streamReader{ctx: ctx, s: s, buf: first}), r)
	if err != nil {
		e.fail(id, fmt.Sprintf("%s:%d 没有回应", host, port))
		http.Error(w, fmt.Sprintf("%s:%d 没有回应", host, port), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	for _, h := range hopHeaders {
		w.Header().Del(h)
	}
	w.WriteHeader(resp.StatusCode)
	fl, _ := w.(http.Flusher)
	buf := make([]byte, egressChunk)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if fl != nil {
				fl.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

// streamWriter and streamReader make a stream an io.Writer and io.Reader.
type streamWriter struct {
	ctx context.Context
	s   *rpc.Stream
}

func (w *streamWriter) Write(p []byte) (int, error) {
	sent := 0
	for len(p) > 0 {
		n := min(len(p), egressChunk)
		if err := w.s.Send(w.ctx, append([]byte(nil), p[:n]...)); err != nil {
			return sent, err
		}
		sent += n
		p = p[n:]
	}
	return sent, nil
}

type streamReader struct {
	ctx context.Context
	s   *rpc.Stream
	buf []byte
}

func (r *streamReader) Read(p []byte) (int, error) {
	for len(r.buf) == 0 {
		b, err := r.s.Recv(r.ctx)
		if err != nil {
			return 0, err
		}
		r.buf = b
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}
