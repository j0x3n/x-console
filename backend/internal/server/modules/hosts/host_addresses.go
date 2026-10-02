package hosts

import (
	"context"
	"encoding/json"
	"net/netip"
	"sort"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func normalizeHostAddresses(input []protocol.HostAddress) []protocol.HostAddress {
	out := []protocol.HostAddress{}
	seen := map[string]bool{}
	for _, value := range input {
		addr, err := netip.ParseAddr(value.IP)
		if err != nil || addr.Zone() != "" {
			continue
		}
		addr = addr.Unmap()
		if addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
			continue
		}
		ip := addr.String()
		if seen[ip] {
			continue
		}
		seen[ip] = true
		_, public := publicIP(ip)
		family := "v6"
		if addr.Is4() {
			family = "v4"
		}
		out = append(out, protocol.HostAddress{IP: ip, Family: family, Public: public})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Public != out[j].Public {
			return out[i].Public
		}
		if out[i].Family != out[j].Family {
			return out[i].Family < out[j].Family
		}
		return out[i].IP < out[j].IP
	})
	return out
}

func (m *Module) saveHostAddresses(ctx context.Context, id, kind string, input []protocol.HostAddress) error {
	addresses := normalizeHostAddresses(input)
	raw, _ := json.Marshal(addresses)
	ip := ""
	for _, a := range addresses {
		if a.Public {
			ip = a.IP
			break
		}
	}
	country := ""
	if ip != "" {
		country = m.geo.country(m.baseCtx, ip)
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = m.ensureHostInfo(ctx, tx, id, kind); err != nil {
		return err
	}
	old, err := readHostInfo(ctx, tx, id)
	if err != nil {
		return err
	}
	if country == "" && old.CountryIP == ip {
		country = old.Country
	}
	_, err = tx.ExecContext(ctx, "UPDATE host_info SET addresses=?,country_code=?,country_ip=?,country_checked_at=? WHERE host_id=?", string(raw), country, ip, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (m *Module) decorateHost(ctx context.Context, h *api.Host) error {
	v, err := readHostInfo(ctx, m.d.DB, h.Id)
	if err != nil {
		return err
	}
	h.Info = hostInfoDTO(v)
	h.SortOrder = int(v.Order)
	h.Addresses = []api.HostAddress{}
	var addresses []protocol.HostAddress
	_ = json.Unmarshal([]byte(v.Addresses), &addresses)
	for _, a := range normalizeHostAddresses(addresses) {
		h.Addresses = append(h.Addresses, api.HostAddress{Ip: a.IP, Family: api.HostAddressFamily(a.Family), Public: a.Public})
	}
	if v.Country != "" {
		h.Country = &api.HostCountry{Code: v.Country, Name: countryName(v.Country)}
	}
	return nil
}

func (m *Module) refreshAddresses(ctx context.Context) error {
	agents, err := m.d.Agents.List(ctx)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		if !agent.Online {
			continue
		}
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		var info protocol.SystemInfo
		err = m.d.Agents.Call(callCtx, agent.ID, protocol.MethodSystemInfo, nil, &info)
		cancel()
		if err != nil {
			continue
		}
		m.infoMu.Lock()
		m.info[agent.ID] = info
		m.infoMu.Unlock()
		addresses := append([]protocol.HostAddress(nil), info.Addresses...)
		if observed := m.d.Agents.SourceIP(agent.ID); observed != "" {
			addresses = append(addresses, protocol.HostAddress{IP: observed})
		}
		if err = m.saveHostAddresses(ctx, agent.ID, agent.Kind, addresses); err != nil {
			m.d.Log.WarnContext(ctx, "host addresses not saved", "host", agent.ID, "error", err)
		}
	}
	return nil
}
