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
	hash = ""
	if cfg.mode() == api.RouterModePush {
		hash = cfg.PushHash
	}
	m.mu.Lock()
	m.pushLoaded, m.pushHash = true, hash
	m.mu.Unlock()
	return hash != "", nil
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
	m.applyReport(ctx, rep, m.now())
	w.WriteHeader(http.StatusNoContent)
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

// errPushReadOnly is the answer to restart requests in push mode.
var errPushReadOnly = httpx.NewError(http.StatusConflict, "router_push_mode", "路由器是主动上报的，面板连不到它，不能从这里重启。请在路由器上操作")
