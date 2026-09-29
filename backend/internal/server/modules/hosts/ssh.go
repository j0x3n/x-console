package hosts

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	sshDialTimeout  = 10 * time.Second
	sshOnlineWindow = 150 * time.Second // polled every minute
	sshMaxOutput    = protocol.ExecMaxOutput
)

var errHostKeyChanged = errors.New("主机指纹和第一次连接时不一样，可能被冒充。确认无误后请在设置里重新保存这台主机")

// sshSecret is what ssh_hosts.secret holds after decryption.
type sshSecret struct {
	Password   string `json:"password,omitempty"`
	Key        string `json:"key,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
}

// sshPool keeps one client connection per SSH host.
type sshPool struct {
	mu      sync.Mutex
	clients map[int64]*ssh.Client
	ok      map[int64]time.Time
}

func newSSHPool() *sshPool {
	return &sshPool{clients: map[int64]*ssh.Client{}, ok: map[int64]time.Time{}}
}

func (p *sshPool) lastOK(id int64) (time.Time, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	t, ok := p.ok[id]
	return t, ok
}

func (p *sshPool) markOK(id int64, t time.Time) {
	p.mu.Lock()
	p.ok[id] = t
	p.mu.Unlock()
}

func (p *sshPool) drop(id int64) {
	p.mu.Lock()
	c := p.clients[id]
	delete(p.clients, id)
	p.mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

func (p *sshPool) forget(id int64) {
	p.drop(id)
	p.mu.Lock()
	delete(p.ok, id)
	p.mu.Unlock()
}

func (p *sshPool) closeAll() {
	p.mu.Lock()
	clients := p.clients
	p.clients = map[int64]*ssh.Client{}
	p.mu.Unlock()
	for _, c := range clients {
		_ = c.Close()
	}
}

// sshConfig builds the client config. The host key callback records the
// key it saw into *seen; a stored key must match (trust on first use).
func sshConfig(username string, secret sshSecret, auth string, knownKey string, seen *ssh.PublicKey) (*ssh.ClientConfig, error) {
	var methods []ssh.AuthMethod
	switch auth {
	case "password":
		methods = append(methods, ssh.Password(secret.Password),
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = secret.Password
				}
				return answers, nil
			}))
	case "key":
		var signer ssh.Signer
		var err error
		if secret.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(secret.Key), []byte(secret.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(secret.Key))
		}
		if err != nil {
			return nil, httpx.Invalid("私钥无法解析: " + err.Error())
		}
		methods = append(methods, ssh.PublicKeys(signer))
	default:
		return nil, httpx.Invalid("auth 只能是 password 或 key")
	}
	return &ssh.ClientConfig{
		User: username,
		Auth: methods,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			*seen = key
			if knownKey == "" {
				return nil
			}
			want, _, _, _, err := ssh.ParseAuthorizedKey([]byte(knownKey))
			if err != nil || !bytes.Equal(want.Marshal(), key.Marshal()) {
				return errHostKeyChanged
			}
			return nil
		},
		Timeout: sshDialTimeout,
	}, nil
}

// dialSSH connects and authenticates. It returns the host key it saw.
func dialSSH(ctx context.Context, address string, port int64, username, authKind string, secret sshSecret, knownKey string) (*ssh.Client, ssh.PublicKey, error) {
	var seen ssh.PublicKey
	cfg, err := sshConfig(username, secret, authKind, knownKey, &seen)
	if err != nil {
		return nil, nil, err
	}
	addr := net.JoinHostPort(address, strconv.FormatInt(port, 10))
	d := net.Dialer{Timeout: sshDialTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, httpx.NewError(http.StatusBadGateway, "ssh_failed", "连不上 "+addr+": "+err.Error())
	}
	_ = conn.SetDeadline(time.Now().Add(2 * sshDialTimeout))
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		if errors.Is(err, errHostKeyChanged) {
			return nil, seen, httpx.NewError(http.StatusBadGateway, "ssh_host_key_changed", errHostKeyChanged.Error())
		}
		return nil, seen, httpx.NewError(http.StatusBadGateway, "ssh_failed", "SSH 登录失败: "+err.Error())
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), seen, nil
}

func (m *Module) openSecret(row db.SshHost) (sshSecret, error) {
	var s sshSecret
	plain, err := m.d.Secrets.Open(row.Secret)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal([]byte(plain), &s)
	return s, err
}

// sshClient returns the pooled client of a host, dialing when needed. The
// first successful connection stores the host key.
func (m *Module) sshClient(ctx context.Context, row db.SshHost) (*ssh.Client, error) {
	m.ssh.mu.Lock()
	c := m.ssh.clients[row.ID]
	m.ssh.mu.Unlock()
	if c != nil {
		return c, nil
	}
	secret, err := m.openSecret(row)
	if err != nil {
		return nil, err
	}
	c, key, err := dialSSH(ctx, row.Address, row.Port, row.Username, row.Auth, secret, row.HostKey)
	if err != nil {
		return nil, err
	}
	if row.HostKey == "" && key != nil {
		if err := m.q.SetSSHHostKey(ctx, db.SetSSHHostKeyParams{HostKey: string(ssh.MarshalAuthorizedKey(key)), ID: row.ID}); err != nil {
			c.Close()
			return nil, err
		}
	}
	m.ssh.mu.Lock()
	if old := m.ssh.clients[row.ID]; old != nil {
		m.ssh.mu.Unlock()
		c.Close()
		return old, nil
	}
	m.ssh.clients[row.ID] = c
	m.ssh.mu.Unlock()
	return c, nil
}

// sshSession opens a session, redialing once if the pooled connection died.
func (m *Module) sshSession(ctx context.Context, row db.SshHost) (*ssh.Session, error) {
	for attempt := 0; ; attempt++ {
		c, err := m.sshClient(ctx, row)
		if err != nil {
			return nil, err
		}
		s, err := c.NewSession()
		if err == nil {
			m.ssh.markOK(row.ID, m.now())
			return s, nil
		}
		m.ssh.drop(row.ID)
		if attempt > 0 {
			return nil, httpx.NewError(http.StatusBadGateway, "ssh_failed", "SSH 会话打不开: "+err.Error())
		}
	}
}

// cappedBuffer keeps the first n bytes.
type cappedBuffer struct {
	mu        sync.Mutex
	b         bytes.Buffer
	n         int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	room := c.n - c.b.Len()
	if room < len(p) {
		c.truncated = true
		if room > 0 {
			c.b.Write(p[:room])
		}
		return len(p), nil
	}
	return c.b.Write(p)
}

func (c *cappedBuffer) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.ToValidUTF8(c.b.String(), "�")
}

func (m *Module) sshExec(ctx context.Context, row db.SshHost, command string, timeout time.Duration) (api.ExecResult, error) {
	s, err := m.sshSession(ctx, row)
	if err != nil {
		return api.ExecResult{}, err
	}
	defer s.Close()
	stdout, stderr := &cappedBuffer{n: sshMaxOutput}, &cappedBuffer{n: sshMaxOutput}
	s.Stdout, s.Stderr = stdout, stderr
	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- s.Run(command) }()
	res := api.ExecResult{}
	select {
	case err = <-done:
	case <-time.After(timeout):
		res.TimedOut = true
	case <-ctx.Done():
		err = ctx.Err()
	}
	if res.TimedOut || ctx.Err() != nil {
		_ = s.Signal(ssh.SIGKILL)
		_ = s.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}
	res.Stdout, res.Stderr = stdout.String(), stderr.String()
	res.Truncated = stdout.truncated || stderr.truncated
	res.DurationMs = time.Since(start).Milliseconds()
	var exitErr *ssh.ExitError
	switch {
	case res.TimedOut:
		res.ExitCode = -1
		return res, nil
	case err == nil:
		return res, nil
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitStatus()
		return res, nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return res, err
	}
	var missing *ssh.ExitMissingError
	if errors.As(err, &missing) {
		res.ExitCode = -1
		return res, nil
	}
	m.ssh.drop(row.ID)
	return res, httpx.NewError(http.StatusBadGateway, "ssh_failed", err.Error())
}

// sshTerm is an interactive shell over SSH.
type sshTerm struct {
	s     *ssh.Session
	in    io.WriteCloser
	out   io.Reader
	close sync.Once
}

func (m *Module) sshTerminal(ctx context.Context, row db.SshHost, cols, rows int) (termSession, error) {
	s, err := m.sshSession(ctx, row)
	if err != nil {
		return nil, err
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := s.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		s.Close()
		return nil, httpx.NewError(http.StatusBadGateway, "ssh_failed", "申请终端失败: "+err.Error())
	}
	in, err := s.StdinPipe()
	if err != nil {
		s.Close()
		return nil, err
	}
	out, err := s.StdoutPipe()
	if err != nil {
		s.Close()
		return nil, err
	}
	s.Stderr = io.Discard // merged into stdout by the pty
	if err := s.Shell(); err != nil {
		s.Close()
		return nil, httpx.NewError(http.StatusBadGateway, "ssh_failed", "启动 shell 失败: "+err.Error())
	}
	return &sshTerm{s: s, in: in, out: out}, nil
}

func (t *sshTerm) Input(_ context.Context, b []byte) error {
	_, err := t.in.Write(b)
	return err
}

func (t *sshTerm) Resize(_ context.Context, cols, rows int) error {
	return t.s.WindowChange(rows, cols)
}

func (t *sshTerm) Output(_ context.Context) ([]byte, error) {
	buf := make([]byte, 32<<10)
	n, err := t.out.Read(buf)
	if n > 0 {
		return buf[:n], nil
	}
	if err == nil {
		err = io.EOF
	}
	return nil, err
}

func (t *sshTerm) Close() {
	t.close.Do(func() {
		_ = t.s.Signal(ssh.SIGHUP)
		_ = t.s.Close()
	})
}

// ---- metrics over SSH ----

// sshMetricsScript prints /proc snapshots one second apart.
const sshMetricsScript = `export LC_ALL=C
echo @@stat1; head -n1 /proc/stat
echo @@net1; cat /proc/net/dev
sleep 1
echo @@stat2; head -n1 /proc/stat
echo @@net2; cat /proc/net/dev
echo @@mem; cat /proc/meminfo
echo @@load; cat /proc/loadavg
echo @@uptime; cat /proc/uptime
echo @@df; df -kP 2>/dev/null
echo @@end`

// pollSSH samples every SSH host once; the scheduler runs it every minute.
func (m *Module) pollSSH(ctx context.Context) error {
	rows, err := m.q.ListSSHHosts(ctx)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	for _, row := range rows {
		wg.Add(1)
		go func(row db.SshHost) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, 45*time.Second)
			defer cancel()
			res, err := m.sshExec(pctx, row, sshMetricsScript, 30*time.Second)
			if err != nil || res.ExitCode != 0 {
				m.d.Log.Debug("ssh metrics failed", "host", row.Name, "err", err, "stderr", res.Stderr)
				return
			}
			x, err := parseProcSnapshot(res.Stdout)
			if err != nil {
				m.d.Log.Debug("ssh metrics parse failed", "host", row.Name, "err", err)
				return
			}
			x.At = m.now()
			m.recordSample(sshHostID(row.ID), x)
		}(row)
	}
	wg.Wait()
	return nil
}

// parseProcSnapshot turns the output of sshMetricsScript into a sample.
func parseProcSnapshot(out string) (protocol.MetricsSample, error) {
	sections := map[string][]string{}
	cur := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "@@") {
			cur = strings.TrimPrefix(line, "@@")
			continue
		}
		if cur != "" && line != "" {
			sections[cur] = append(sections[cur], line)
		}
	}
	if _, ok := sections["mem"]; !ok {
		return protocol.MetricsSample{}, errors.New("no /proc/meminfo in output")
	}
	x := protocol.MetricsSample{CPUPerCore: []float64{}, Disks: []protocol.DiskUsage{}}
	if a, ok := cpuLine(sections["stat1"]); ok {
		if b, ok := cpuLine(sections["stat2"]); ok {
			x.CPU = round1(busyBetween(a, b))
		}
	}
	rx1, tx1 := netDev(sections["net1"])
	rx2, tx2 := netDev(sections["net2"])
	x.NetRxTotal, x.NetTxTotal = netDevCounted(sections["net2"])
	if rx2 >= rx1 {
		x.NetRxRate = float64(rx2 - rx1)
	}
	if tx2 >= tx1 {
		x.NetTxRate = float64(tx2 - tx1)
	}
	mem := map[string]uint64{}
	for _, l := range sections["mem"] {
		f := strings.Fields(l)
		if len(f) >= 2 {
			v, _ := strconv.ParseUint(f[1], 10, 64)
			mem[strings.TrimSuffix(f[0], ":")] = v * 1024
		}
	}
	x.MemTotal = mem["MemTotal"]
	if avail, ok := mem["MemAvailable"]; ok && avail <= x.MemTotal {
		x.MemUsed = x.MemTotal - avail
	} else {
		x.MemUsed = x.MemTotal - mem["MemFree"] - mem["Buffers"] - mem["Cached"]
	}
	x.SwapTotal = mem["SwapTotal"]
	if mem["SwapFree"] <= x.SwapTotal {
		x.SwapUsed = x.SwapTotal - mem["SwapFree"]
	}
	if l := sections["load"]; len(l) > 0 {
		f := strings.Fields(l[0])
		if len(f) >= 4 {
			x.Load1, _ = strconv.ParseFloat(f[0], 64)
			x.Load5, _ = strconv.ParseFloat(f[1], 64)
			x.Load15, _ = strconv.ParseFloat(f[2], 64)
			if _, total, ok := strings.Cut(f[3], "/"); ok {
				x.Procs, _ = strconv.Atoi(total)
			}
		}
	}
	if l := sections["uptime"]; len(l) > 0 {
		if f := strings.Fields(l[0]); len(f) > 0 {
			up, _ := strconv.ParseFloat(f[0], 64)
			x.UptimeSeconds = uint64(up)
		}
	}
	seen := map[string]bool{}
	for _, l := range sections["df"] {
		f := strings.Fields(l)
		if len(f) < 6 || !strings.HasPrefix(f[0], "/dev/") || strings.HasPrefix(f[0], "/dev/loop") || seen[f[0]] {
			continue
		}
		total, err1 := strconv.ParseUint(f[1], 10, 64)
		used, err2 := strconv.ParseUint(f[2], 10, 64)
		if err1 != nil || err2 != nil || total == 0 {
			continue
		}
		seen[f[0]] = true
		x.Disks = append(x.Disks, protocol.DiskUsage{Mount: strings.Join(f[5:], " "), Used: used * 1024, Total: total * 1024})
	}
	return x, nil
}

// cpuLine parses the aggregate "cpu" line of /proc/stat into (total, idle).
func cpuLine(lines []string) ([2]float64, bool) {
	if len(lines) == 0 {
		return [2]float64{}, false
	}
	f := strings.Fields(lines[0])
	if len(f) < 5 || f[0] != "cpu" {
		return [2]float64{}, false
	}
	var total, idle float64
	for i, s := range f[1:] {
		if i >= 8 { // guest time is already part of user time
			break
		}
		v, _ := strconv.ParseFloat(s, 64)
		total += v
		if i == 3 || i == 4 { // idle, iowait
			idle += v
		}
	}
	return [2]float64{total, idle}, true
}

func busyBetween(a, b [2]float64) float64 {
	dt := b[0] - a[0]
	if dt <= 0 {
		return 0
	}
	return min(100, max(0, (dt-(b[1]-a[1]))/dt*100))
}

// netDev sums received and sent bytes of /proc/net/dev except loopback.
func netDev(lines []string) (rx, tx uint64) {
	for _, l := range lines {
		name, rest, ok := strings.Cut(l, ":")
		if !ok || strings.TrimSpace(name) == "lo" {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(f[0], 10, 64)
		t, _ := strconv.ParseUint(f[8], 10, 64)
		rx += r
		tx += t
	}
	return rx, tx
}

// netDevCounted sums the bytes of the interfaces that count as traffic.
func netDevCounted(lines []string) (rx, tx uint64) {
	for _, l := range lines {
		name, rest, ok := strings.Cut(l, ":")
		if !ok || !protocol.CountedInterface(name) {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(f[0], 10, 64)
		t, _ := strconv.ParseUint(f[8], 10, 64)
		rx += r
		tx += t
	}
	return rx, tx
}

// ---- SSH host CRUD ----

func toAPISSH(row db.SshHost) api.SshHost {
	fp := ""
	if row.HostKey != "" {
		if k, _, _, _, err := ssh.ParseAuthorizedKey([]byte(row.HostKey)); err == nil {
			fp = ssh.FingerprintSHA256(k)
		}
	}
	return api.SshHost{Id: row.ID, HostId: sshHostID(row.ID), Name: row.Name, Address: row.Address, Port: int(row.Port),
		Username: row.Username, Auth: api.SshAuth(row.Auth), HostKeyFingerprint: fp, CreatedAt: row.CreatedAt}
}

type sshInput struct {
	name, address, username, auth string
	port                          int64
	secret                        *sshSecret // nil keeps the stored secret
}

func parseSSHInput(body api.SshHostInput, requireSecret bool) (sshInput, error) {
	in := sshInput{name: strings.TrimSpace(body.Name), address: strings.TrimSpace(body.Address),
		username: strings.TrimSpace(body.Username), auth: string(body.Auth), port: 22}
	if in.name == "" || in.address == "" || in.username == "" {
		return in, httpx.Invalid("名称、地址和用户名都要填")
	}
	if strings.ContainsAny(in.address, " /") {
		return in, httpx.Invalid("地址格式不对")
	}
	if body.Port != nil {
		if *body.Port < 1 || *body.Port > 65535 {
			return in, httpx.Invalid("端口要在 1 到 65535 之间")
		}
		in.port = int64(*body.Port)
	}
	if in.auth != "password" && in.auth != "key" {
		return in, httpx.Invalid("auth 只能是 password 或 key")
	}
	secret := ""
	if body.Secret != nil {
		secret = *body.Secret
	}
	if secret == "" {
		if requireSecret {
			return in, httpx.Invalid("请填写密码或私钥")
		}
		return in, nil
	}
	s := &sshSecret{}
	if in.auth == "password" {
		s.Password = secret
	} else {
		s.Key = secret
		if body.Passphrase != nil {
			s.Passphrase = *body.Passphrase
		}
		var err error
		if s.Passphrase != "" {
			_, err = ssh.ParsePrivateKeyWithPassphrase([]byte(s.Key), []byte(s.Passphrase))
		} else {
			_, err = ssh.ParsePrivateKey([]byte(s.Key))
		}
		if err != nil {
			return in, httpx.Invalid("私钥无法解析: " + err.Error())
		}
	}
	in.secret = s
	return in, nil
}

func (m *Module) sealSecret(s sshSecret) (string, error) {
	raw, _ := json.Marshal(s)
	return m.d.Secrets.Seal(string(raw))
}

// ListSshHosts is GET /ssh-hosts.
func (m *Module) ListSshHosts(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListSSHHosts(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.SshHost, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAPISSH(row))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) getSSH(ctx context.Context, id int64) (db.SshHost, error) {
	row, err := m.q.GetSSHHost(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, httpx.ErrNotFound
	}
	return row, err
}

// GetSshHost is GET /ssh-hosts/{sshId}.
func (m *Module) GetSshHost(w http.ResponseWriter, r *http.Request, id int64) {
	row, err := m.getSSH(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPISSH(row))
}

// CreateSshHost is POST /ssh-hosts.
func (m *Module) CreateSshHost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.CreateSshHostJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in, err := parseSSHInput(body, true)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sealed, err := m.sealSecret(*in.secret)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.q.CreateSSHHost(ctx, db.CreateSSHHostParams{Name: in.name, Address: in.address, Port: in.port,
		Username: in.username, Auth: in.auth, Secret: sealed, CreatedAt: m.now()})
	m.d.Audit.Record(ctx, "host.ssh.create", in.name, map[string]any{"address": in.address, "username": in.username}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("host.ssh.created", toAPISSH(row))
	httpx.JSON(w, http.StatusCreated, toAPISSH(row))
}

// UpdateSshHost is PUT /ssh-hosts/{sshId}.
func (m *Module) UpdateSshHost(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.UpdateSshHostJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	old, err := m.getSSH(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in, err := parseSSHInput(body, old.Auth != string(body.Auth))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sealed := old.Secret
	if in.secret != nil {
		if sealed, err = m.sealSecret(*in.secret); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	hostKey := old.HostKey
	if in.address != old.Address || in.port != old.Port {
		hostKey = "" // a different machine: learn its key again
	}
	row, err := m.q.UpdateSSHHost(ctx, db.UpdateSSHHostParams{Name: in.name, Address: in.address, Port: in.port,
		Username: in.username, Auth: in.auth, Secret: sealed, HostKey: hostKey, ID: id})
	m.d.Audit.Record(ctx, "host.ssh.update", in.name, map[string]any{"address": in.address, "secretChanged": in.secret != nil}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.ssh.drop(id)
	m.d.Bus.Publish("host.ssh.updated", toAPISSH(row))
	httpx.JSON(w, http.StatusOK, toAPISSH(row))
}

// DeleteSshHost is DELETE /ssh-hosts/{sshId}.
func (m *Module) DeleteSshHost(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	n, err := m.q.DeleteSSHHost(ctx, id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(ctx, "host.ssh.delete", sshHostID(id), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.ssh.forget(id)
	m.metrics.drop(sshHostID(id))
	m.dropTraffic(ctx, sshHostID(id))
	m.d.Bus.Publish("host.ssh.deleted", map[string]string{"hostId": sshHostID(id)})
	httpx.NoContent(w)
}

// TestSshHost is POST /ssh-hosts/test: connect once without saving.
func (m *Module) TestSshHost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.TestSshHostJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in, err := parseSSHInput(body, true)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, testConnection(ctx, in.address, in.port, in.username, in.auth, *in.secret, ""))
}

// TestSavedSshHost is POST /ssh-hosts/{sshId}/test.
func (m *Module) TestSavedSshHost(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	row, err := m.getSSH(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	secret, err := m.openSecret(row)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	res := testConnection(ctx, row.Address, row.Port, row.Username, row.Auth, secret, row.HostKey)
	if res.Ok {
		m.ssh.markOK(id, m.now())
	}
	httpx.JSON(w, http.StatusOK, res)
}

func testConnection(ctx context.Context, address string, port int64, username, authKind string, secret sshSecret, knownKey string) api.SshTestResult {
	ctx, cancel := context.WithTimeout(ctx, 2*sshDialTimeout)
	defer cancel()
	c, key, err := dialSSH(ctx, address, port, username, authKind, secret, knownKey)
	res := api.SshTestResult{}
	if key != nil {
		fp := ssh.FingerprintSHA256(key)
		res.Fingerprint = &fp
	}
	if err != nil {
		var he *httpx.Error
		if errors.As(err, &he) {
			res.Message = he.Message
		} else {
			res.Message = err.Error()
		}
		return res
	}
	defer c.Close()
	res.Ok = true
	res.Message = fmt.Sprintf("已连上 %s", c.ServerVersion())
	return res
}
