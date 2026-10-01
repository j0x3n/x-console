package router

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router/api"
)

func configToAPI(cfg config) api.RouterConfig {
	out := api.RouterConfig{Url: cfg.URL, Username: cfg.Username, Mode: cfg.mode(), HasPassword: cfg.Password != ""}
	if cfg.AgentID != "" {
		id := cfg.AgentID
		out.AgentId = &id
	}
	return out
}

func (m *Module) GetRouterConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := m.loadConfig(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, configToAPI(cfg))
}

// PutRouterConfig logs in with the new values before saving. It can send the
// stored password to a new address, so it needs elevation.
func (m *Module) PutRouterConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.RouterConfigInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cfg, err := m.saveConfig(ctx, in)
	m.d.Audit.Record(ctx, "router.config", cfg.URL, map[string]any{"mode": string(cfg.mode()), "passwordChanged": in.Password != nil && *in.Password != ""}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.reset()
	httpx.JSON(w, http.StatusOK, configToAPI(cfg))
}

func (m *Module) saveConfig(ctx context.Context, in api.RouterConfigInput) (config, error) {
	if strings.TrimSpace(in.Url) == "" {
		for _, k := range []string{keyURL, keyUsername, keyPassword, keyAgentID} {
			if err := m.d.Settings.Delete(ctx, k); err != nil {
				return config{}, err
			}
		}
		return config{}, nil
	}
	old, err := m.loadConfig(ctx)
	if err != nil {
		return config{}, err
	}
	u, err := normalizeURL(in.Url)
	if err != nil {
		return config{}, err
	}
	cfg := config{URL: u, Username: "root", Password: old.Password}
	if in.Username != nil && strings.TrimSpace(*in.Username) != "" {
		cfg.Username = strings.TrimSpace(*in.Username)
	}
	if in.Password != nil && *in.Password != "" {
		cfg.Password = *in.Password
	}
	if in.Mode != nil && *in.Mode == api.Agent {
		id := ""
		if in.AgentId != nil {
			id = strings.TrimSpace(*in.AgentId)
		}
		if err := m.checkAgent(ctx, id); err != nil {
			return config{}, err
		}
		cfg.AgentID = id
	} else if in.Mode != nil && *in.Mode != api.Direct {
		return config{}, httpx.Invalid("连接方式只能是 direct 或 agent")
	}
	tctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var board boardInfo
	if err := m.newClient(cfg).call(tctx, "system", "board", nil, &board); err != nil {
		return config{}, httpx.Invalid(err.Error())
	}
	for key, v := range map[string]string{keyURL: cfg.URL, keyUsername: cfg.Username, keyAgentID: cfg.AgentID} {
		if err := m.d.Settings.Set(ctx, key, v); err != nil {
			return config{}, err
		}
	}
	if err := m.d.Settings.SetSecret(ctx, keyPassword, cfg.Password); err != nil {
		return config{}, err
	}
	return cfg, nil
}

// fail maps ubus errors: an unreachable router is 502, the rest are shown as is.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	if unreachable(err) {
		httpx.Fail(w, r, &httpx.Error{Status: http.StatusBadGateway, Code: "router_unreachable", Message: err.Error()})
		return
	}
	if _, ok := err.(*ubusError); ok {
		httpx.Fail(w, r, &httpx.Error{Status: http.StatusBadGateway, Code: "router_error", Message: err.Error()})
		return
	}
	httpx.Fail(w, r, err)
}

func (m *Module) GetRouterStatus(w http.ResponseWriter, r *http.Request) {
	u, err := m.ubus(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	st, err := m.status(r.Context(), u)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (m *Module) GetRouterClients(w http.ResponseWriter, r *http.Request) {
	u, err := m.ubus(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := m.clients(r.Context(), u, false)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// GetRouterTraffic sums the minute samples into 5 minute (24h) or hour (7d) points.
func (m *Module) GetRouterTraffic(w http.ResponseWriter, r *http.Request, params api.GetRouterTrafficParams) {
	rng, span, step := "24h", 24*time.Hour, 5*time.Minute
	if params.Range != nil && *params.Range == "7d" {
		rng, span, step = "7d", 7*24*time.Hour, time.Hour
	}
	now := m.now()
	start := now.Add(-span).Truncate(step)
	rows, err := m.q.ListTraffic(r.Context(), start.Unix())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := api.RouterTraffic{Range: rng, StepSeconds: int(step / time.Second), Points: []api.RouterTrafficPoint{}}
	type bucket struct{ rx, tx, secs int64 }
	buckets := map[int64]*bucket{}
	for _, row := range rows {
		key := time.Unix(row.At, 0).Truncate(step).Unix()
		b := buckets[key]
		if b == nil {
			b = &bucket{}
			buckets[key] = b
		}
		b.rx += row.Rx
		b.tx += row.Tx
		b.secs += row.Seconds
		out.RxBytes += row.Rx
		out.TxBytes += row.Tx
	}
	// Every bucket in the range, so a gap shows as a gap in the chart.
	for at := start; !at.After(now); at = at.Add(step) {
		b := buckets[at.Unix()]
		if b == nil || b.secs == 0 {
			continue
		}
		out.Points = append(out.Points, api.RouterTrafficPoint{
			At: at.UTC(), RxRate: float64(b.rx) / float64(b.secs), TxRate: float64(b.tx) / float64(b.secs),
		})
	}
	httpx.JSON(w, http.StatusOK, out)
}

var ifaceName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,32}$`)

// RestartRouterInterface takes one interface down and up again.
func (m *Module) RestartRouterInterface(w http.ResponseWriter, r *http.Request, name string) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !ifaceName.MatchString(name) {
		httpx.Fail(w, r, httpx.Invalid("接口名不对"))
		return
	}
	u, err := m.ubus(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	obj := "network.interface." + name
	err = u.call(ctx, obj, "down", nil, nil)
	if err == nil {
		err = u.call(ctx, obj, "up", nil, nil)
	}
	m.d.Audit.Record(ctx, "router.interface.restart", name, nil, err)
	if err != nil {
		fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RebootRouter restarts the router after a typed confirmation.
func (m *Module) RebootRouter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.RebootRouterJSONBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if strings.TrimSpace(body.Confirm) != "重启" {
		httpx.Fail(w, r, httpx.Invalid("请输入“重启”确认"))
		return
	}
	u, err := m.ubus(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err = u.call(ctx, "system", "reboot", nil, nil)
	m.d.Audit.Record(ctx, "router.reboot", "", nil, err)
	if err != nil {
		fail(w, r, err)
		return
	}
	m.reset()
	m.mu.Lock()
	m.quiet = m.now().Add(5 * time.Minute) // a reboot takes a minute or two
	m.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
}
