package hosts

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/agenthub"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const sshPrefix = "ssh:"

// sshCapabilities are what an SSH-only host offers.
var sshCapabilities = []string{protocol.CapMetrics, protocol.CapPTY, protocol.CapExec}

// hostRef is a resolved host id: either an agent or an SSH host.
type hostRef struct {
	ID    string
	Name  string
	Kind  string
	agent *agenthub.Agent
	ssh   *db.SshHost
}

func sshHostID(id int64) string { return sshPrefix + strconv.FormatInt(id, 10) }

func parseSSHID(id string) (int64, bool) {
	if !strings.HasPrefix(id, sshPrefix) {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(id, sshPrefix), 10, 64)
	return n, err == nil && n > 0
}

func errUnsupported(msg string) error {
	return httpx.NewError(http.StatusNotImplemented, "feature_unavailable", msg)
}

func (m *Module) host(ctx context.Context, id string) (hostRef, error) {
	ref, err := m.lookupHost(ctx, id)
	if err != nil {
		return hostRef{}, err
	}
	if m.kindHidden(ctx, ref.Kind) {
		return hostRef{}, httpx.ErrNotFound
	}
	return ref, nil
}

func (m *Module) lookupHost(ctx context.Context, id string) (hostRef, error) {
	if n, ok := parseSSHID(id); ok {
		row, err := m.q.GetSSHHost(ctx, n)
		if errors.Is(err, sql.ErrNoRows) {
			return hostRef{}, httpx.ErrNotFound
		}
		if err != nil {
			return hostRef{}, err
		}
		return hostRef{ID: id, Name: row.Name, Kind: "server", ssh: &row}, nil
	}
	a, err := m.d.Agents.Get(ctx, id)
	if err != nil {
		return hostRef{}, err
	}
	return hostRef{ID: id, Name: a.Name, Kind: a.Kind, agent: &a}, nil
}

// agentFor resolves id to an online agent that announced capability c.
func (m *Module) agentFor(ctx context.Context, id, c string) (agenthub.Agent, error) {
	h, err := m.host(ctx, id)
	if err != nil {
		return agenthub.Agent{}, err
	}
	if h.ssh != nil {
		return agenthub.Agent{}, errUnsupported("SSH 主机不支持这个功能，请安装代理")
	}
	if !h.agent.Online {
		return agenthub.Agent{}, httpx.ErrAgentOffline
	}
	if c != "" && !h.agent.Has(c) {
		return agenthub.Agent{}, errUnsupported("这台机器不支持这个功能")
	}
	return *h.agent, nil
}

// resolveRef accepts a host id or a host name (for actions called by the
// assistant, which knows names better than ids).
func (m *Module) resolveRef(ctx context.Context, ref string) (string, error) {
	if _, err := m.host(ctx, ref); err == nil {
		return ref, nil
	}
	hosts, err := m.listHosts(ctx, "")
	if err != nil {
		return "", err
	}
	for _, h := range hosts {
		if strings.EqualFold(h.Name, ref) || strings.EqualFold(h.Hostname, ref) {
			return h.Id, nil
		}
	}
	return "", httpx.ErrNotFound
}

func (m *Module) withMetrics(h *api.Host) {
	x, ok := m.metrics.latest(h.Id)
	if !ok {
		return
	}
	s := toAPISample(x)
	h.Metrics = &s
	cpu, mem := x.CPU, percent(x.MemUsed, x.MemTotal)
	disk, _ := fullestDisk(x.Disks)
	h.Cpu, h.Memory, h.Disk = &cpu, &mem, &disk
}

func agentHost(a agenthub.Agent, now time.Time) api.Host {
	h := api.Host{Id: a.ID, Name: a.Name, Kind: api.HostKind(a.Kind), Source: api.Agent, Online: a.Online, Os: a.Os,
		Arch: a.Arch, Hostname: a.Hostname, Version: a.Version, Capabilities: a.Capabilities, LastSeenAt: a.LastSeenAt}
	if a.Online {
		h.LastSeenAt = &now
	}
	return h
}

func (m *Module) sshHost(row db.SshHost, now time.Time) api.Host {
	id := sshHostID(row.ID)
	h := api.Host{Id: id, Name: row.Name, Kind: api.Server, Source: api.Ssh, Os: "linux", Hostname: row.Address,
		Version: "ssh", Capabilities: append([]string{}, sshCapabilities...)}
	if last, ok := m.ssh.lastOK(row.ID); ok {
		h.LastSeenAt = &last
		h.Online = now.Sub(last) < sshOnlineWindow
	}
	return h
}

func (m *Module) listHosts(ctx context.Context, kind string) ([]api.Host, error) {
	kind = canonicalHostKind(kind)
	if kind != "" && m.kindHidden(ctx, kind) {
		return []api.Host{}, nil
	}
	all, err := m.gatherHosts(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]api.Host, 0, len(all))
	for _, h := range all {
		if m.kindHidden(ctx, string(h.Kind)) {
			continue
		}
		if kind != "" && string(h.Kind) != kind {
			continue
		}
		filtered = append(filtered, h)
	}
	return filtered, nil
}

func (m *Module) gatherHosts(ctx context.Context) ([]api.Host, error) {
	now := m.now()
	agents, err := m.d.Agents.List(ctx)
	if err != nil {
		return nil, err
	}
	sshRows, err := m.q.ListSSHHosts(ctx)
	if err != nil {
		return nil, err
	}
	open, err := m.q.ListOpenAlertEvents(ctx)
	if err != nil {
		return nil, err
	}
	active := map[string]int{}
	for _, ev := range open {
		active[ev.HostID]++
	}
	out := make([]api.Host, 0, len(agents)+len(sshRows))
	for _, a := range agents {
		out = append(out, agentHost(a, now))
	}
	for _, row := range sshRows {
		out = append(out, m.sshHost(row, now))
	}
	for i := range out {
		out[i].ActiveAlerts = active[out[i].Id]
		m.withMetrics(&out[i])
		if err := m.decorateHost(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind > out[j].Kind
		}
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// ListHosts is GET /hosts.
func (m *Module) ListHosts(w http.ResponseWriter, r *http.Request, params api.ListHostsParams) {
	kind := ""
	if params.Kind != nil {
		kind = string(*params.Kind)
	}
	hosts, err := m.listHosts(r.Context(), kind)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	briefs, err := m.trafficBriefs(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := make([]api.HostListItem, 0, len(hosts))
	for _, h := range hosts {
		item := api.HostListItem{Id: h.Id, Name: h.Name, Kind: h.Kind, Source: h.Source,
			Online: h.Online, Hostname: h.Hostname, Os: h.Os, LastSeenAt: h.LastSeenAt,
			Cpu: h.Cpu, Memory: h.Memory, Disk: h.Disk, ActiveAlerts: h.ActiveAlerts,
			Info: h.Info, Addresses: h.Addresses, Country: h.Country, SortOrder: h.SortOrder}
		if x := h.Metrics; x != nil {
			item.Metrics = &api.HostListMetrics{At: x.At, Load1: x.Load1, NetRx: x.NetRx,
				NetTx: x.NetTx, UptimeSeconds: x.UptimeSeconds}
		}
		if b, ok := briefs[h.Id]; ok {
			item.Traffic = &b
		}
		items = append(items, item)
	}
	httpx.JSON(w, http.StatusOK, items)
}

// GetHost is GET /hosts/{hostId}.
func (m *Module) GetHost(w http.ResponseWriter, r *http.Request, hostID string) {
	d, err := m.detail(r.Context(), hostID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (m *Module) detail(ctx context.Context, hostID string) (api.HostDetail, error) {
	ref, err := m.host(ctx, hostID)
	if err != nil {
		return api.HostDetail{}, err
	}
	now := m.now()
	var h api.Host
	var out api.HostDetail
	if ref.ssh != nil {
		h = m.sshHost(*ref.ssh, now)
		addr := ref.ssh.Address + ":" + strconv.FormatInt(ref.ssh.Port, 10)
		out.Address = &addr
	} else {
		h = agentHost(*ref.agent, now)
		if info, ok := m.systemInfo(ctx, *ref.agent); ok {
			out.SystemInfo = &api.SystemInfo{Hostname: info.Hostname, Os: info.OS, Platform: info.Platform,
				PlatformVersion: info.PlatformVer, KernelVersion: info.KernelVersion, Arch: info.Arch, CpuModel: info.CPUModel,
				CpuCores: info.CPUCores, MemoryTotal: int64(info.MemoryTotal), UptimeSeconds: int64(info.UptimeSeconds)}
		}
	}
	open, err := m.q.ListOpenAlertEvents(ctx)
	if err != nil {
		return out, err
	}
	for _, ev := range open {
		if ev.HostID == h.Id {
			h.ActiveAlerts++
		}
	}
	m.withMetrics(&h)
	if err := m.decorateHost(ctx, &h); err != nil {
		return out, err
	}
	out.Info, out.Addresses, out.Country, out.SortOrder = h.Info, h.Addresses, h.Country, h.SortOrder
	out.Id, out.Name, out.Kind, out.Source, out.Online = h.Id, h.Name, h.Kind, h.Source, h.Online
	out.Os, out.Arch, out.Hostname, out.Version, out.Capabilities = h.Os, h.Arch, h.Hostname, h.Version, h.Capabilities
	out.LastSeenAt, out.Metrics, out.Cpu, out.Memory, out.Disk, out.ActiveAlerts = h.LastSeenAt, h.Metrics, h.Cpu, h.Memory, h.Disk, h.ActiveAlerts
	return out, nil
}

// systemInfo asks an online agent and caches the answer for offline times.
func (m *Module) systemInfo(ctx context.Context, a agenthub.Agent) (protocol.SystemInfo, bool) {
	if a.Online && a.Has(protocol.CapSystemInfo) {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		var info protocol.SystemInfo
		err := m.d.Agents.Call(cctx, a.ID, protocol.MethodSystemInfo, nil, &info)
		cancel()
		if err == nil {
			m.infoMu.Lock()
			m.info[a.ID] = info
			m.infoMu.Unlock()
			addresses := append([]protocol.HostAddress(nil), info.Addresses...)
			if ip := m.d.Agents.SourceIP(a.ID); ip != "" {
				addresses = append(addresses, protocol.HostAddress{IP: ip})
			}
			if err := m.saveHostAddresses(ctx, a.ID, a.Kind, addresses); err != nil {
				m.d.Log.WarnContext(ctx, "host addresses not saved", "error", err)
			}
			return info, true
		}
	}
	m.infoMu.Lock()
	defer m.infoMu.Unlock()
	info, ok := m.info[a.ID]
	return info, ok
}

// GetHostMetrics is GET /hosts/{hostId}/metrics.
func (m *Module) GetHostMetrics(w http.ResponseWriter, r *http.Request, hostID string, params api.GetHostMetricsParams) {
	if _, err := m.host(r.Context(), hostID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rng := ""
	if params.Range != nil {
		rng = string(*params.Range)
	}
	s, err := m.series(r.Context(), hostID, rng)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

// Summaries implements contracts.Hosts.
func (m *Module) Summaries(ctx context.Context) ([]contracts.HostSummary, error) {
	hosts, err := m.gatherHosts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.HostSummary, 0, len(hosts))
	for _, h := range hosts {
		s := contracts.HostSummary{ID: h.Id, Name: h.Name, Kind: string(h.Kind), Online: h.Online, LastSeen: h.LastSeenAt}
		if h.Cpu != nil {
			s.CPU, s.Memory, s.Disk = *h.Cpu, *h.Memory, *h.Disk
		}
		out = append(out, s)
	}
	return out, nil
}
