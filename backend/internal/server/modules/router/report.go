package router

// B114: the router reports by itself. A shell script on the router runs from
// cron once a minute, collects what the panel would read over ubus and POSTs
// it here. The panel never connects to the router, so no agent is needed.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router/api"
)

const (
	reportPath = "/router/report"
	// pushStaleAfter is how long the panel waits for the next report before
	// it calls the router offline. The script reports every minute.
	pushStaleAfter = 3 * time.Minute
	// defaultPushInterval is the seconds between reports until the user picks another.
	defaultPushInterval = 60
	// commandTTL is how long a queued command waits for the router. After that
	// the router is probably offline and the command would surprise someone later.
	commandTTL = 5 * time.Minute
	// maxCommands bounds the queue.
	maxCommands = 5
	// maxReport bounds one report. A few hundred devices make about 100 KB.
	maxReport = 4 << 20
)

//go:embed report.sh
var reportScript string

// PublicPaths implements module.PublicPather: the router has no session, the
// report is checked against the token instead.
func (m *Module) PublicPaths() []string { return []string{reportPath} }

// pushSnapshot is the latest report, ready to serve.
type pushSnapshot struct {
	at      time.Time
	status  api.RouterStatus
	clients []api.RouterClient
}

// errNoReport is what the pages get before the first report and after the
// reports stop.
func errNoReport(snap *pushSnapshot, now time.Time) error {
	if snap == nil {
		return &httpx.Error{Status: http.StatusBadGateway, Code: "router_unreachable", Message: "还没有收到路由器的上报。在路由器上执行设置页里的安装命令，等一分钟再看"}
	}
	return &httpx.Error{Status: http.StatusBadGateway, Code: "router_unreachable",
		Message: "已经 " + humanDuration(now.Sub(snap.at)) + "没收到路由器的上报，上一次在 " + snap.at.Local().Format("15:04")}
}

// pushMode reports whether the router reports by itself. The answer is kept
// until the config changes.
func (m *Module) pushMode(ctx context.Context) (bool, error) {
	m.mu.Lock()
	loaded, hash := m.pushLoaded, m.pushHash
	m.mu.Unlock()
	if loaded {
		return hash != "", nil
	}
	cfg, err := m.loadConfig(ctx)
	if err != nil {
		return false, err
	}
	hash, interval := "", 0
	if cfg.mode() == api.RouterModePush {
		hash, interval = cfg.PushHash, cfg.PushInterval
	}
	m.mu.Lock()
	m.pushLoaded, m.pushHash, m.pushInterval = true, hash, interval
	m.mu.Unlock()
	return hash != "", nil
}

// pushIntervals are the report intervals the user can pick, in seconds.
var pushIntervals = []int{3, 5, 10, 30, 60}

func validInterval(n int) bool { return slices.Contains(pushIntervals, n) }

// routerCommand is something the router is asked to do on its next report.
type routerCommand struct {
	id     int64
	action string // "restart_interface" or "reboot"
	arg    string
	at     time.Time
}

// queueCommand adds a command for the router. The same command twice in a row
// is kept once. It reports false when the queue is full.
func (m *Module) queueCommand(action, arg string) bool {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropExpiredLocked(now)
	for _, c := range m.cmds {
		if c.action == action && c.arg == arg {
			return true
		}
	}
	if len(m.cmds) >= maxCommands {
		return false
	}
	m.cmdSeq++
	m.cmds = append(m.cmds, routerCommand{id: m.cmdSeq, action: action, arg: arg, at: now})
	return true
}

func (m *Module) dropExpiredLocked(now time.Time) {
	m.cmds = slices.DeleteFunc(m.cmds, func(c routerCommand) bool { return now.Sub(c.at) > commandTTL })
}

// takeReply builds the answer to a report: the interval and the commands.
// Each command is handed out once.
func (m *Module) takeReply(now time.Time) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropExpiredLocked(now)
	interval := m.pushInterval
	if !validInterval(interval) {
		interval = defaultPushInterval
	}
	var b strings.Builder
	fmt.Fprintf(&b, "interval=%d\n", interval)
	for _, c := range m.cmds {
		fmt.Fprintf(&b, "cmd=%d %s %s\n", c.id, c.action, c.arg)
		if c.action == "reboot" {
			m.quiet = now.Add(5 * time.Minute) // the router is about to go quiet
		}
	}
	m.cmds = nil
	return b.String()
}

// PutRouterPushInterval changes how often the router script reports.
func (m *Module) PutRouterPushInterval(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Seconds int `json:"seconds"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !validInterval(in.Seconds) {
		httpx.Fail(w, r, httpx.Invalid("上报间隔只能是 3、5、10、30、60 秒"))
		return
	}
	push, err := m.pushMode(ctx)
	if err == nil && !push {
		err = httpx.NewError(http.StatusConflict, "router_not_push", "路由器不是主动上报的，没有上报间隔")
	}
	if err == nil {
		err = m.d.Settings.Set(ctx, keyPushInterval, in.Seconds)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.mu.Lock()
	m.pushInterval = in.Seconds
	m.mu.Unlock()
	cfg, err := m.loadConfig(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m.configToAPI(r, cfg))
}

// currentReport returns the latest report, or the error the pages show.
func (m *Module) currentReport() (*pushSnapshot, error) {
	now := m.now()
	m.mu.Lock()
	snap := m.report
	m.mu.Unlock()
	if snap == nil || now.Sub(snap.at) > pushStaleAfter {
		return nil, errNoReport(snap, now)
	}
	return snap, nil
}

// pollPush runs once a minute in push mode: no report for a while means the
// router, its power or the home network is down.
func (m *Module) pollPush(ctx context.Context, now time.Time) {
	m.mu.Lock()
	if m.lastReport.IsZero() {
		m.lastReport = now // after a panel restart, give the router one full period
	}
	last := m.lastReport
	stale := now.Sub(last) >= pushStaleAfter
	if stale && m.watch.since.IsZero() {
		m.watch = wanWatch{since: last, reason: "unreachable", push: true}
	}
	m.mu.Unlock()
	if stale {
		m.wanState(ctx, now, false, "unreachable", "")
	}
	m.prune(ctx, now)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// reportURL is the address the router posts to.
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

// installScript fills the router script in and wraps it in the commands that
// put it on the router and schedule it.
func installScript(url, token string) string {
	quote := func(s string) string { return strings.ReplaceAll(s, "'", `'\''`) }
	script := strings.NewReplacer("@URL@", quote(url), "@TOKEN@", quote(token)).Replace(reportScript)
	return "cat > /usr/bin/xc-report.sh <<'XC_EOF'\n" + script + "XC_EOF\n" +
		"chmod 700 /usr/bin/xc-report.sh\n" +
		"(crontab -l 2>/dev/null | grep -v xc-report.sh; echo '* * * * * /usr/bin/xc-report.sh') | crontab -\n" +
		"/etc/init.d/cron enable\n" +
		"/etc/init.d/cron restart\n" +
		"/usr/bin/xc-report.sh && echo '已上报一次'\n"
}

// CreateRouterPushToken switches to push mode and returns a new token. The
// old ubus login and the old token stop working at once.
func (m *Module) CreateRouterPushToken(w http.ResponseWriter, r *http.Request) {
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
	err := m.d.Settings.Set(ctx, keyPushHash, hashToken(token))
	if err == nil {
		for _, k := range []string{keyURL, keyUsername, keyPassword, keyAgentID} {
			if err = m.d.Settings.Delete(ctx, k); err != nil {
				break
			}
		}
	}
	m.d.Audit.Record(ctx, "router.push.token", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.reset()
	url := m.reportURL(r)
	httpx.JSON(w, http.StatusOK, api.RouterPushToken{Token: token, ReportUrl: url, Script: installScript(url, token)})
}

// ReportRouter takes one report from the router script.
func (m *Module) ReportRouter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	push, err := m.pushMode(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.mu.Lock()
	hash := m.pushHash
	m.mu.Unlock()
	token := bearerToken(r)
	if !push || token == "" || subtle.ConstantTimeCompare([]byte(hashToken(token)), []byte(hash)) != 1 {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxReport))
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid("上报内容读不出来或者太大"))
		return
	}
	rep, err := parseReport(body)
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid(err.Error()))
		return
	}
	now := m.now()
	m.applyReport(ctx, rep, now)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, m.takeReply(now))
}

// report is one parsed upload.
type report struct {
	board      boardInfo
	info       systemInfo
	interfaces []ifaceInfo
	devices    map[string]deviceStat
	leases     string
	arp        string
	routerTime time.Time // the router's clock, to turn lease expiry times into seconds left
}

type deviceStat struct {
	Statistics struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"statistics"`
}

// sections splits the body at lines like "#xc:board".
func sections(body []byte) map[string]string {
	out := map[string]string{}
	var name string
	var b strings.Builder
	flush := func() {
		if name != "" {
			out[name] = b.String()
		}
		b.Reset()
	}
	for _, line := range strings.Split(string(body), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "#xc:"); ok {
			flush()
			name = strings.TrimSpace(rest)
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	flush()
	return out
}

func parseReport(body []byte) (report, error) {
	sec := sections(body)
	var rep report
	read := func(name string, v any) error {
		text := strings.TrimSpace(sec[name])
		if text == "" {
			return fmt.Errorf("上报里缺少 %s", name)
		}
		if err := json.Unmarshal([]byte(text), v); err != nil {
			return fmt.Errorf("上报里的 %s 不是 JSON", name)
		}
		return nil
	}
	if err := read("board", &rep.board); err != nil {
		return rep, err
	}
	if err := read("info", &rep.info); err != nil {
		return rep, err
	}
	var dump struct {
		Interface []ifaceInfo `json:"interface"`
	}
	if err := read("interfaces", &dump); err != nil {
		return rep, err
	}
	rep.interfaces = withoutLoopback(dump.Interface)
	// The counters are optional: without them there is no speed or traffic.
	if strings.TrimSpace(sec["devices"]) != "" {
		if err := read("devices", &rep.devices); err != nil {
			return rep, err
		}
	}
	rep.leases, rep.arp = sec["leases"], sec["arp"]
	if secs, err := strconv.ParseInt(strings.TrimSpace(sec["time"]), 10, 64); err == nil && secs > 0 {
		rep.routerTime = time.Unix(secs, 0)
	}
	return rep, nil
}

// applyReport stores a report: traffic sample, WAN state, status and devices.
func (m *Module) applyReport(ctx context.Context, rep report, now time.Time) {
	st := buildStatus(rep.board, rep.info, rep.interfaces, now)
	st.Source = api.RouterStatusSourcePush
	if wan, ok := pickWAN(rep.interfaces); ok {
		if !wan.Up {
			m.wanState(ctx, now, false, "wan", "")
		} else {
			m.wanState(ctx, now, true, "", firstIP(wan))
			if dev := wan.dev(); dev != "" {
				if stat, ok := rep.devices[dev]; ok {
					st.RxRate, st.TxRate = m.addSample(ctx, sample{dev: dev, rx: stat.Statistics.RxBytes, tx: stat.Statistics.TxBytes, at: now})
				}
			}
		}
	}
	// Lease times are Unix times on the router's clock, so the seconds left
	// are worked out against that clock, not the panel's.
	leaseNow := rep.routerTime
	if leaseNow.IsZero() {
		leaseNow = now
	}
	clients := m.mergeClients(now, parseLeaseFile(rep.leases, leaseNow), parseARP(rep.arp))
	st.ClientCount = len(clients)
	m.mu.Lock()
	m.report = &pushSnapshot{at: now, status: st, clients: clients}
	m.lastReport = now
	m.mu.Unlock()
}
