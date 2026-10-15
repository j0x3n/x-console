package watchers_test

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/egress"
	"github.com/j0x3n/x-console/backend/internal/server/browser"
	"github.com/j0x3n/x-console/backend/internal/server/browser/browsertest"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/api"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// B208: the browser goes out through a connected agent.

// anyIP lets the test agents dial the test servers on this machine.
func anyIP(net.IP) bool { return true }

// egressFixture has the proxy running and three agents: one that connects to
// anything, one with the real public-only rule, and one too old to know
// net.dial.
type egressFixture struct {
	*fixture
	proxy string       // listen address of the egress proxy
	open  string       // agent that dials anything
	real  string       // agent with the real rule
	old   string       // agent without net.dial
	dials atomic.Int64 // connections the open agent made
}

func setupEgress(t *testing.T) *egressFixture {
	t.Helper()
	f := setup(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	addr, err := watchers.StartEgress(f.m, ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	e := &egressFixture{fixture: f, proxy: addr}
	e.open = f.env.Agent("server", []string{protocol.CapEgress}, func(c *conn.Client) {
		dial := egress.New(anyIP).Handle
		c.HandleStream(protocol.MethodNetDial, func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
			e.dials.Add(1)
			return dial(ctx, raw, s)
		})
	})
	e.real = f.env.Agent("server", []string{protocol.CapEgress}, func(c *conn.Client) {
		c.HandleStream(protocol.MethodNetDial, egress.Handle)
	})
	e.old = f.env.Agent("server", nil, nil)
	return e
}

// watcher makes a watcher that goes out through agent ("" for none) and
// returns its id and the proxy login the browser would get.
func (e *egressFixture) watcher(t *testing.T, agent string) (id int64, user, pass string) {
	t.Helper()
	body := map[string]any{"name": "w", "url": "https://www.example.gov.cn/a", "goal": "盯"}
	if agent != "" {
		body["egressAgentId"] = agent
	}
	w := e.create(t, body)
	raw, err := watchers.EgressProxyURL(e.m, w.Id)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	pass, _ = u.User.Password()
	return w.Id, u.User.Username(), pass
}

func basic(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

// connectVia sends CONNECT to the proxy and returns the answer and the
// connection, which carries the tunnel when the answer is 200.
func connectVia(t *testing.T, proxy, auth, target string) (*http.Response, net.Conn, *bufio.Reader) {
	t.Helper()
	c, err := net.DialTimeout("tcp", proxy, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target)
	if auth != "" {
		req += "Proxy-Authorization: " + auth + "\r\n"
	}
	if _, err := io.WriteString(c, req+"\r\n"); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatal(err)
	}
	return resp, c, br
}

// viaProxy is an HTTP client that sends everything to the proxy with the
// given login (none when user is empty).
func viaProxy(proxy, user, pass string) *http.Client {
	u := &url.URL{Scheme: "http", Host: proxy}
	if user != "" {
		u.User = url.UserPassword(user, pass)
	}
	return &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(u), DisableKeepAlives: true}}
}

// echoServer answers every byte it gets with the same byte.
func echoServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().String()
}

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEgressProxyAuth(t *testing.T) {
	e := setupEgress(t)
	target := echoServer(t)
	id, user, pass := e.watcher(t, e.open)
	if user != fmt.Sprintf("w%d", id) || len(pass) < 32 {
		t.Fatalf("login %q %q", user, pass)
	}

	// No password, a wrong password, a wrong user and a made-up task: 407
	// with a challenge, so the browser sends the login.
	for name, auth := range map[string]string{
		"none": "", "wrong password": basic(user, pass+"x"), "wrong user": basic("w999", pass),
		"not a task": basic("admin", pass), "bad scheme": "Bearer " + pass, "bad base64": "Basic ???",
	} {
		resp, c, _ := connectVia(t, e.proxy, auth, target)
		if resp.StatusCode != http.StatusProxyAuthRequired || !strings.HasPrefix(resp.Header.Get("Proxy-Authenticate"), "Basic") {
			t.Fatalf("connect, %s: %d %v", name, resp.StatusCode, resp.Header)
		}
		c.Close()
	}
	u, p := user, pass+"x"
	if name := "plain"; true {
		resp, err := viaProxy(e.proxy, u, p).Get("http://" + target + "/")
		if err != nil || resp.StatusCode != http.StatusProxyAuthRequired {
			t.Fatalf("%s, wrong password: %v %v", name, resp, err)
		}
		resp.Body.Close()
		resp, err = viaProxy(e.proxy, "", "").Get("http://" + target + "/")
		if err != nil || resp.StatusCode != http.StatusProxyAuthRequired || resp.Header.Get("Proxy-Authenticate") != `Basic realm="x-console"` {
			t.Fatalf("%s, no login: %v %v", name, resp, err)
		}
		resp.Body.Close()
	}

	// The right login works while the task has a machine ...
	resp, c, _ := connectVia(t, e.proxy, basic(user, pass), target)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("right login: %d", resp.StatusCode)
	}
	c.Close()

	// ... and is refused with 403 once it has none: the machine is read from
	// the database on every connection.
	e.env.Elevate()
	e.env.MustDo(http.MethodPatch, fmt.Sprintf("/watchers/%d", id), map[string]any{"egressAgentId": ""}, nil)
	resp, c, _ = connectVia(t, e.proxy, basic(user, pass), target)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("no machine, connect: %d", resp.StatusCode)
	}
	c.Close()
	r2, err := viaProxy(e.proxy, user, pass).Get("http://" + target + "/")
	if err != nil || r2.StatusCode != http.StatusForbidden {
		t.Fatalf("no machine, plain: %v %v", r2, err)
	}
	r2.Body.Close()

	// A task that never had a machine: 403 as well.
	_, user2, pass2 := e.watcher(t, "")
	resp, c, _ = connectVia(t, e.proxy, basic(user2, pass2), target)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("task without machine: %d", resp.StatusCode)
	}
	c.Close()
	// Every task has its own password.
	if pass == pass2 {
		t.Fatal("two tasks share a password")
	}
	resp, c, _ = connectVia(t, e.proxy, basic(user2, pass), target)
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("password of another task: %d", resp.StatusCode)
	}
	c.Close()
	// The password is not in the API.
	status, raw := e.env.Do(http.MethodGet, fmt.Sprintf("/watchers/%d", id), nil, nil)
	if status != 200 || strings.Contains(string(raw), pass) {
		t.Fatalf("the password leaks: %d", status)
	}
}

func TestEgressConnectTunnel(t *testing.T) {
	e := setupEgress(t)
	target := echoServer(t)
	_, user, pass := e.watcher(t, e.open)

	resp, c, br := connectVia(t, e.proxy, basic(user, pass), target)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("connect: %d", resp.StatusCode)
	}
	if _, err := io.WriteString(c, "hello\n"); err != nil {
		t.Fatal(err)
	}
	line, err := br.ReadString('\n')
	if err != nil || line != "hello\n" {
		t.Fatalf("echo %q %v", line, err)
	}
	// A body bigger than one frame comes back whole and in order.
	big := make([]byte, 700<<10)
	for i := range big {
		big[i] = byte(i*31 + i>>8)
	}
	go func() { _, _ = c.Write(big) }()
	got := make([]byte, len(big))
	if _, err := io.ReadFull(br, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(big) {
		t.Fatal("the data came back different")
	}
	// Closing the browser side ends the stream on the agent.
	c.Close()

	// A target nobody listens on: 502 with a reason.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := ln.Addr().String()
	ln.Close()
	resp, c2, _ := connectVia(t, e.proxy, basic(user, pass), dead)
	if resp.StatusCode != http.StatusBadGateway || !strings.Contains(readAll(t, resp.Body), "出口机器连不上") {
		t.Fatalf("dead target: %d", resp.StatusCode)
	}
	c2.Close()
	// A target that is not host:port.
	resp, c3, _ := connectVia(t, e.proxy, basic(user, pass), "nothing")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad target: %d", resp.StatusCode)
	}
	c3.Close()
}

func TestEgressPlainHTTP(t *testing.T) {
	e := setupEgress(t)
	big := strings.Repeat("0123456789abcdef", 40<<10) // 640 KB
	var mu sync.Mutex
	var seen []*http.Request
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen, bodies = append(seen, r), append(bodies, string(b))
		mu.Unlock()
		switch r.URL.Path {
		case "/big":
			_, _ = io.WriteString(w, big)
		case "/teapot":
			w.Header().Set("X-Test", "1")
			w.WriteHeader(http.StatusTeapot)
			_, _ = io.WriteString(w, "短而粗")
		default:
			_, _ = io.WriteString(w, "你好 "+r.Method)
		}
	}))
	t.Cleanup(srv.Close)
	_, user, pass := e.watcher(t, e.open)
	client := viaProxy(e.proxy, user, pass)

	resp, err := client.Get(srv.URL + "/page?q=1")
	if err != nil || resp.StatusCode != 200 || readAll(t, resp.Body) != "你好 GET" {
		t.Fatalf("get: %v %v", resp, err)
	}
	resp, err = client.Post(srv.URL+"/submit", "text/plain", strings.NewReader("name=张三"))
	if err != nil || readAll(t, resp.Body) != "你好 POST" {
		t.Fatalf("post: %v %v", resp, err)
	}
	resp, err = client.Get(srv.URL + "/teapot")
	if err != nil || resp.StatusCode != http.StatusTeapot || resp.Header.Get("X-Test") != "1" || readAll(t, resp.Body) != "短而粗" {
		t.Fatalf("status and headers: %v %v", resp, err)
	}
	resp, err = client.Get(srv.URL + "/big")
	if err != nil || readAll(t, resp.Body) != big {
		t.Fatalf("big body: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 4 {
		t.Fatalf("the target saw %d requests", len(seen))
	}
	r := seen[0]
	if r.RequestURI != "/page?q=1" || r.Header.Get("Proxy-Authorization") != "" || r.Header.Get("Proxy-Connection") != "" || r.Host != srv.Listener.Addr().String() {
		t.Fatalf("request at the target: %q %v host %q", r.RequestURI, r.Header, r.Host)
	}
	if bodies[1] != "name=张三" {
		t.Fatalf("post body %q", bodies[1])
	}

	// A request without an absolute address is not a proxy request.
	c, err := net.Dial("tcp", e.proxy)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = io.WriteString(c, "GET /page HTTP/1.1\r\nHost: x\r\nProxy-Authorization: "+basic(user, pass)+"\r\n\r\n")
	r3, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil || r3.StatusCode != http.StatusBadRequest {
		t.Fatalf("relative address: %v %v", r3, err)
	}
}

func TestEgressRefusesLocalTargetsAndOldAgents(t *testing.T) {
	e := setupEgress(t)
	target := echoServer(t)

	// The agent with the real rule only connects to public addresses.
	_, user, pass := e.watcher(t, e.real)
	resp, c, _ := connectVia(t, e.proxy, basic(user, pass), target)
	body := readAll(t, resp.Body)
	if resp.StatusCode != http.StatusBadGateway || !strings.Contains(body, "不是公网地址") {
		t.Fatalf("local target: %d %q", resp.StatusCode, body)
	}
	c.Close()
	r2, err := viaProxy(e.proxy, user, pass).Get("http://" + target + "/")
	if err != nil || r2.StatusCode != http.StatusBadGateway || !strings.Contains(readAll(t, r2.Body), "不是公网地址") {
		t.Fatalf("local target, plain: %v %v", r2, err)
	}

	// An agent that does not know net.dial.
	_, user, pass = e.watcher(t, e.old)
	resp, c, _ = connectVia(t, e.proxy, basic(user, pass), target)
	body = readAll(t, resp.Body)
	if resp.StatusCode != http.StatusBadGateway || !strings.Contains(body, "这台机器的代理太旧，先更新代理") {
		t.Fatalf("old agent: %d %q", resp.StatusCode, body)
	}
	c.Close()
}

func TestEgressLimitsConnectionsPerTask(t *testing.T) {
	e := setupEgress(t)
	target := echoServer(t)
	_, user, pass := e.watcher(t, e.open)
	_, user2, pass2 := e.watcher(t, e.open)
	var conns []net.Conn
	for i := 0; i < 64; i++ {
		resp, c, _ := connectVia(t, e.proxy, basic(user, pass), target)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("connection %d: %d", i+1, resp.StatusCode)
		}
		conns = append(conns, c)
	}
	resp, c, _ := connectVia(t, e.proxy, basic(user, pass), target)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("connection 65: %d", resp.StatusCode)
	}
	c.Close()
	// Another task is not affected.
	resp, c, _ = connectVia(t, e.proxy, basic(user2, pass2), target)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("other task: %d", resp.StatusCode)
	}
	c.Close()
	// A closed connection frees its place.
	conns[0].Close()
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, c, _ := connectVia(t, e.proxy, basic(user, pass), target)
		c.Close()
		if resp.StatusCode == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the place was not freed: %d", resp.StatusCode)
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, c := range conns[1:] {
		c.Close()
	}
}

func TestEgressSettingsRules(t *testing.T) {
	e := setupEgress(t)
	e.env.Elevate()
	post := func(body map[string]any) (int, string) {
		status, raw := e.env.Do(http.MethodPost, "/watchers", body, nil)
		return status, string(raw)
	}
	base := func(extra map[string]any) map[string]any {
		b := map[string]any{"name": "w", "url": "https://www.example.gov.cn/a", "goal": "盯"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	if status, raw := post(base(map[string]any{"egressAgentId": e.open, "proxy": "http://10.0.0.1:3128"})); status != http.StatusBadRequest || !strings.Contains(raw, "出口代理和出口机器只能选一个") {
		t.Fatalf("both: %d %s", status, raw)
	}
	if status, _ := post(base(map[string]any{"egressAgentId": "no-such-agent"})); status != http.StatusBadRequest {
		t.Fatalf("unknown machine: %d", status)
	}
	if status, _ := post(map[string]any{"kind": "x", "name": "y", "url": "@bob", "goal": "", "egressAgentId": e.open}); status != http.StatusBadRequest {
		t.Fatalf("x with a machine: %d", status)
	}

	var w api.Watcher
	e.env.MustDo(http.MethodPost, "/watchers", base(map[string]any{"egressAgentId": e.open}), &w)
	if w.EgressAgentId == nil || *w.EgressAgentId != e.open || w.HasProxy {
		t.Fatalf("created %+v", w)
	}
	path := fmt.Sprintf("/watchers/%d", w.Id)
	var list struct{ Items []api.Watcher }
	e.env.MustDo(http.MethodGet, "/watchers", nil, &list)
	if len(list.Items) != 1 || list.Items[0].EgressAgentId == nil || *list.Items[0].EgressAgentId != e.open {
		t.Fatalf("list %+v", list)
	}

	var p2 api.Watcher
	// Picking a proxy drops the machine; picking a machine drops the proxy.
	p2 = api.Watcher{}
	e.env.MustDo(http.MethodPatch, path, map[string]any{"proxy": "http://10.0.0.1:3128"}, &p2)
	if p2.EgressAgentId != nil || !p2.HasProxy {
		t.Fatalf("after proxy: %+v", p2)
	}
	p2 = api.Watcher{}
	e.env.MustDo(http.MethodPatch, path, map[string]any{"egressAgentId": e.real}, &p2)
	if p2.EgressAgentId == nil || *p2.EgressAgentId != e.real || p2.HasProxy {
		t.Fatalf("after machine: %+v", p2)
	}
	if status, raw := e.env.Do(http.MethodPatch, path, map[string]any{"egressAgentId": e.open, "proxy": "http://10.0.0.1:3128"}, nil); status != http.StatusBadRequest || !strings.Contains(string(raw), "出口代理和出口机器只能选一个") {
		t.Fatalf("patch both: %d %s", status, raw)
	}
	if status, _ := e.env.Do(http.MethodPatch, path, map[string]any{"egressAgentId": "no-such-agent"}, nil); status != http.StatusBadRequest {
		t.Fatalf("patch unknown machine: %d", status)
	}
	// Other changes leave the machine alone; an empty id removes it.
	p2 = api.Watcher{}
	e.env.MustDo(http.MethodPatch, path, map[string]any{"name": "新名字"}, &p2)
	if p2.EgressAgentId == nil || *p2.EgressAgentId != e.real {
		t.Fatalf("after rename: %+v", p2)
	}
	p2 = api.Watcher{}
	e.env.MustDo(http.MethodPatch, path, map[string]any{"egressAgentId": ""}, &p2)
	if p2.EgressAgentId != nil {
		t.Fatalf("after removal: %+v", p2)
	}
	var x api.Watcher
	e.env.MustDo(http.MethodPost, "/watchers", map[string]any{"kind": "x", "name": "y", "url": "@bob", "goal": ""}, &x)
	if status, _ := e.env.Do(http.MethodPatch, fmt.Sprintf("/watchers/%d", x.Id), map[string]any{"egressAgentId": e.open}, nil); status != http.StatusBadRequest {
		t.Fatalf("patch x with a machine: %d", status)
	}
	watchers.WaitRuns(e.m)

	// The changes are in the audit log.
	var n int
	row := e.env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'watcher.update' AND detail LIKE '%"egressChanged":true%' AND detail LIKE ?`, "%"+e.real+"%")
	if err := row.Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit entries for the machine: %d %v", n, err)
	}
	row = e.env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'watcher.create' AND result = 'ok' AND detail LIKE ?`, "%"+e.open+"%")
	if err := row.Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit entries for create: %d %v", n, err)
	}
}

func TestEgressNotConfigured(t *testing.T) {
	f := setup(t, false) // the proxy is not started
	agent := f.env.Agent("server", []string{protocol.CapEgress}, nil)
	w := f.create(t, map[string]any{"name": "w", "url": "https://www.example.gov.cn/a", "goal": "盯", "egressAgentId": agent})
	got := f.until(t, w.Id, api.WatcherStatusError)
	if !strings.Contains(got.StatusDetail, "服务器没有开出口转发") {
		t.Fatalf("detail %q", got.StatusDetail)
	}
	watchers.WaitRuns(f.m)
}

func TestEgressMachineOffline(t *testing.T) {
	e := setupEgress(t)
	ctx := context.Background()
	hub := e.env.App.Deps.Agents
	code, _, err := hub.CreatePairingCode(ctx, "Unraid 虚拟机", "server")
	if err != nil {
		t.Fatal(err)
	}
	away, _, err := hub.Pair(ctx, code, protocol.Hello{ProtocolVersion: protocol.Version, OS: "linux", Capabilities: []string{protocol.CapEgress}})
	if err != nil {
		t.Fatal(err)
	}
	if hub.Online(away) {
		t.Fatal("the agent is online")
	}
	e.chat.script() // a run that does start would fail loudly
	w := e.create(t, map[string]any{"name": "w", "url": "https://www.example.gov.cn/a", "goal": "盯", "egressAgentId": away, "intervalMinutes": 30})
	watchers.WaitRuns(e.m)

	got := e.get(t, w.Id)
	if got.Status != api.WatcherStatusNew || got.StatusDetail != "出口机器 Unraid 虚拟机 不在线，等它上线再检查" {
		t.Fatalf("after the first try: %s %q", got.Status, got.StatusDetail)
	}
	if len(got.Runs) != 0 || e.chat.count() != 0 || got.NextCheckAt == nil || got.NextCheckAt.Before(time.Now().Add(25*time.Minute)) {
		t.Fatalf("a run started or the next check is too soon: runs %d, chat %d, next %v", len(got.Runs), e.chat.count(), got.NextCheckAt)
	}

	// A run started by hand is refused at once, and it is not an error.
	if status, _ := e.env.Do(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "manual"}, nil); status != http.StatusConflict {
		t.Fatalf("a manual run with the machine away: %d", status)
	}
	watchers.WaitRuns(e.m)
	got = e.get(t, w.Id)
	if got.Status == api.WatcherStatusError || len(got.Runs) != 0 || !strings.Contains(got.StatusDetail, "不在线，等它上线再检查") {
		t.Fatalf("after a check: %s %q runs %d", got.Status, got.StatusDetail, len(got.Runs))
	}
	if e.notified(t, "watcher.failed") != 0 {
		t.Fatal("an error notice was sent")
	}
}

// localAddr is an address of this machine that is not loopback. Chrome never
// sends loopback addresses to a proxy.
func localAddr(t *testing.T) string {
	t.Helper()
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLoopback() {
			return ipn.IP.String()
		}
	}
	t.Skip("这台机器没有非回环的网络地址")
	return ""
}

func TestEgressRealBrowser(t *testing.T) {
	if browsertest.Path() == "" {
		t.Skip("没有找到 Chrome，设置 XC_TEST_BROWSER")
	}
	ip := localAddr(t)
	e := setupEgress(t)
	var mu sync.Mutex
	var hits []string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, r.Header.Get("Proxy-Authorization")+"|"+r.RequestURI)
		mu.Unlock()
		fmt.Fprint(w, `<html><body><h1>经过出口</h1><p id="v">办理状态：已受理</p></body></html>`)
	}))
	l, err := net.Listen("tcp", ip+":0")
	if err != nil {
		t.Skip("监听不了本机地址：", err)
	}
	srv.Listener = l
	srv.Start()
	t.Cleanup(srv.Close)

	id, _, _ := e.watcher(t, e.open)
	proxy, err := watchers.EgressProxyURL(e.m, id)
	if err != nil {
		t.Fatal(err)
	}
	svc := browsertest.Service(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sess, err := svc.Open(ctx, browser.Options{Key: "egress-test", Proxy: proxy, AllowedHosts: []string{ip}})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.Navigate(ctx, srv.URL+"/status"); err != nil {
		t.Fatal(err)
	}
	page, err := sess.Snapshot(ctx, 2000)
	if err != nil || !strings.Contains(page.Text, "办理状态：已受理") {
		t.Fatalf("page %+v %v", page, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(hits) == 0 || strings.HasPrefix(hits[0], "Basic") || !strings.HasSuffix(hits[0], "|/status") {
		t.Fatalf("the site saw %v", hits)
	}
	// The page came through the machine, not straight from the browser.
	if e.dials.Load() == 0 {
		t.Fatal("the agent made no connection")
	}
}
