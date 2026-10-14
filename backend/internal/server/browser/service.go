// Package browser drives a shared Chromium over the DevTools protocol (B205).
//
// The browser runs in its own container (deploy/docker-compose.yml, profile
// "browser"). XC_BROWSER says where it is: an http:// or ws:// address of the
// DevTools endpoint, or the path of a Chrome executable that the server starts
// itself (development and tests). Every Session is a separate browser
// context, so watchers never see each other's cookies. A session's login
// state is exported with SaveState and handed back on the next Open.
package browser

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// EnvAddr is the environment variable with the browser address.
const EnvAddr = "XC_BROWSER"

// ErrUnavailable means no browser is configured or it cannot be reached.
var ErrUnavailable = errors.New("浏览器没有启动")

// Defaults of a Service.
const (
	DefaultSlots  = 2
	DefaultMaxUse = 5 * time.Minute
)

// Service hands out sessions of one browser. The zero value is not usable;
// use New.
type Service struct {
	addr   string
	log    *slog.Logger
	slots  chan struct{}
	maxUse time.Duration

	mu       sync.Mutex
	c        *conn
	proc     *exec.Cmd
	tmp      string
	sessions map[string]*Session
}

// New returns a service for the browser at addr. An empty addr makes a
// service that is never available.
func New(addr string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{addr: strings.TrimSpace(addr), log: log, slots: make(chan struct{}, DefaultSlots),
		maxUse: DefaultMaxUse, sessions: map[string]*Session{}}
}

// FromEnv returns a service for the address in XC_BROWSER.
func FromEnv(log *slog.Logger) *Service { return New(os.Getenv(EnvAddr), log) }

// Configured reports whether an address is set.
func (s *Service) Configured() bool { return s != nil && s.addr != "" }

// SetLimits changes the number of pages open at once and the default time a
// session may stay open. Call it before the first Open.
func (s *Service) SetLimits(slots int, maxUse time.Duration) {
	if slots > 0 {
		s.slots = make(chan struct{}, slots)
	}
	if maxUse > 0 {
		s.maxUse = maxUse
	}
}

// Check connects to the browser and returns its version.
func (s *Service) Check(ctx context.Context) (string, error) {
	c, err := s.connect(ctx)
	if err != nil {
		return "", err
	}
	var v struct {
		Product string `json:"product"`
	}
	if err := c.call(ctx, "", "Browser.getVersion", nil, &v); err != nil {
		return "", err
	}
	return v.Product, nil
}

// Live returns the open session with this key, or nil.
func (s *Service) Live(key string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[key]
}

// Close ends all sessions and stops a browser the service started.
func (s *Service) Close() {
	s.mu.Lock()
	list := make([]*Session, 0, len(s.sessions))
	for _, x := range s.sessions {
		list = append(list, x)
	}
	s.mu.Unlock()
	for _, x := range list {
		x.Close()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.c != nil {
		s.c.close()
		s.c = nil
	}
	if s.proc != nil {
		_ = s.proc.Process.Kill()
		_ = s.proc.Wait()
		s.proc = nil
	}
	if s.tmp != "" {
		_ = os.RemoveAll(s.tmp)
		s.tmp = ""
	}
}

func (s *Service) connect(ctx context.Context) (*conn, error) {
	if !s.Configured() {
		return nil, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.c != nil && s.c.alive() {
		return s.c, nil
	}
	wsURL, err := s.endpoint(ctx)
	if err != nil {
		s.log.Warn("browser unavailable", "err", err)
		return nil, fmt.Errorf("%w：%v", ErrUnavailable, err)
	}
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c, err := dial(dctx, wsURL)
	if err != nil {
		return nil, fmt.Errorf("%w：%v", ErrUnavailable, err)
	}
	s.c = c
	return c, nil
}

// endpoint returns the browser websocket address. Called with s.mu held.
func (s *Service) endpoint(ctx context.Context) (string, error) {
	switch {
	case strings.HasPrefix(s.addr, "ws://") || strings.HasPrefix(s.addr, "wss://"):
		if strings.Contains(s.addr, "/devtools/browser/") {
			return s.addr, nil
		}
		return versionURL(ctx, s.addr)
	case strings.HasPrefix(s.addr, "http://") || strings.HasPrefix(s.addr, "https://"):
		return versionURL(ctx, s.addr)
	default:
		return s.launch(ctx)
	}
}

// versionURL asks /json/version for the websocket address. Chrome refuses
// a Host header that is not an IP or localhost, so the host is resolved
// first (the compose service name "browser" is not accepted as is).
func versionURL(ctx context.Context, addr string) (string, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return "", err
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return "", err
	}
	if net.ParseIP(host) == nil && host != "localhost" {
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(ips) == 0 {
			return "", fmt.Errorf("找不到 %s", host)
		}
		host = ips[0].IP.String()
	}
	scheme := "http"
	if u.Scheme == "https" || u.Scheme == "wss" {
		scheme = "https"
	}
	base := scheme + "://" + net.JoinHostPort(host, port)
	rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, base+"/json/version", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var v struct {
		WS string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&v); err != nil || v.WS == "" {
		return "", errors.New("浏览器没有返回调试地址")
	}
	ws, err := url.Parse(v.WS)
	if err != nil {
		return "", err
	}
	// The browser builds the address from the Host header; use the address
	// we reached it at, because inside a container it may say 0.0.0.0.
	ws.Host = net.JoinHostPort(host, port)
	if scheme == "https" {
		ws.Scheme = "wss"
	}
	return ws.String(), nil
}

var listening = regexp.MustCompile(`DevTools listening on (ws://\S+)`)

// launch starts the Chrome at s.addr. Called with s.mu held.
func (s *Service) launch(ctx context.Context) (string, error) {
	if s.proc != nil {
		_ = s.proc.Process.Kill()
		_ = s.proc.Wait()
		s.proc = nil
	}
	if s.tmp == "" {
		dir, err := os.MkdirTemp("", "xc-browser-")
		if err != nil {
			return "", err
		}
		s.tmp = dir
	}
	args := []string{"--headless=new", "--remote-debugging-port=0", "--user-data-dir=" + s.tmp,
		"--no-first-run", "--no-default-browser-check", "--disable-gpu", "--disable-dev-shm-usage",
		"--disable-background-networking", "--disable-sync", "--mute-audio", "about:blank"}
	if os.Geteuid() == 0 {
		args = append(args, "--no-sandbox")
	}
	// The process outlives the request that started it.
	cmd := exec.Command(s.addr, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	found := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		sent := false
		for sc.Scan() {
			if m := listening.FindStringSubmatch(sc.Text()); m != nil && !sent {
				found <- m[1]
				sent = true
			}
		}
	}()
	select {
	case ws := <-found:
		s.proc = cmd
		return ws, nil
	case <-time.After(20 * time.Second):
	case <-ctx.Done():
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return "", errors.New("浏览器启动超时")
}
