package focusblock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/focusblock/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// SettingsKey is where the switches and lists are kept.
const SettingsKey = "focusblock.settings"

const maxHosts = 20

// options is the stored configuration.
type options struct {
	Enabled      bool     `json:"enabled"`
	HostIDs      []string `json:"hostIds"`
	Domains      []string `json:"domains"`
	Apps         []string `json:"apps"`
	IPv6         bool     `json:"ipv6"`
	BlockInBreak bool     `json:"blockInBreak"`
	HAStart      string   `json:"haStart"`
	HAEnd        string   `json:"haEnd"`
}

func (o options) lists() bool { return len(o.Domains)+len(o.Apps) > 0 }

// hash identifies the part of the configuration the agent receives, so a block
// that was sent with older lists is sent again.
func (o options) hash() string {
	b, _ := json.Marshal(struct {
		D []string
		A []string
		V bool
	}{o.Domains, o.Apps, o.IPv6})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func (m *Module) options(ctx context.Context) options {
	o := options{}
	if err := m.d.Settings.Get(ctx, SettingsKey, &o); err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("focus block settings", "err", err)
	}
	if o.HostIDs == nil {
		o.HostIDs = []string{}
	}
	if o.Domains == nil {
		o.Domains = []string{}
	}
	if o.Apps == nil {
		o.Apps = []string{}
	}
	return o
}

// entityPattern is a scene or script of Home Assistant.
var entityPattern = regexp.MustCompile(`^(scene|script)\.[a-z0-9_]{1,80}$`)

func describeDomainError(entry string, err error) error {
	switch {
	case errors.Is(err, protocol.ErrBlockDomainIP):
		return httpx.Invalid(fmt.Sprintf("“%s”是 IP 地址。请写域名，比如 youtube.com", entry))
	case errors.Is(err, protocol.ErrBlockDomainOne):
		return httpx.Invalid(fmt.Sprintf("“%s”不完整。请写完整域名，比如 youtube.com", entry))
	case errors.Is(err, protocol.ErrBlockEmpty):
		return httpx.Invalid("网址不能是空的")
	}
	return httpx.Invalid(fmt.Sprintf("“%s”不是有效的网址。请写域名，比如 youtube.com", entry))
}

func describeAppError(entry string, err error) error {
	switch {
	case errors.Is(err, protocol.ErrBlockProtected):
		return httpx.Invalid(fmt.Sprintf("“%s”是系统关键程序，不能结束", entry))
	case errors.Is(err, protocol.ErrBlockAppPath):
		return httpx.Invalid(fmt.Sprintf("“%s”带了路径。只写程序名，比如 steam", entry))
	case errors.Is(err, protocol.ErrBlockEmpty):
		return httpx.Invalid("程序名不能是空的")
	}
	return httpx.Invalid(fmt.Sprintf("“%s”不是有效的程序名", entry))
}

// cleanDomains normalizes the list, removes repeats and keeps the order.
func cleanDomains(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, d := range in {
		n, err := protocol.NormalizeBlockDomain(d)
		if err != nil {
			return nil, describeDomainError(d, err)
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	if len(out) > protocol.FocusBlockMaxDomains {
		return nil, httpx.Invalid(fmt.Sprintf("网址最多 %d 个", protocol.FocusBlockMaxDomains))
	}
	return out, nil
}

func cleanApps(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, a := range in {
		n, err := protocol.NormalizeBlockApp(a)
		if err == nil && protocol.IsProtectedProcess(n) {
			err = protocol.ErrBlockProtected
		}
		if err != nil {
			return nil, describeAppError(a, err)
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	if len(out) > protocol.FocusBlockMaxApps {
		return nil, httpx.Invalid(fmt.Sprintf("程序最多 %d 个", protocol.FocusBlockMaxApps))
	}
	return out, nil
}

// merge checks the input and returns the new options.
func (m *Module) merge(ctx context.Context, o options, in api.FocusBlockSettingsInput) (options, error) {
	var err error
	if in.Enabled != nil {
		o.Enabled = *in.Enabled
	}
	if in.Ipv6 != nil {
		o.IPv6 = *in.Ipv6
	}
	if in.BlockInBreak != nil {
		o.BlockInBreak = *in.BlockInBreak
	}
	if in.Domains != nil {
		if o.Domains, err = cleanDomains(*in.Domains); err != nil {
			return o, err
		}
	}
	if in.Apps != nil {
		if o.Apps, err = cleanApps(*in.Apps); err != nil {
			return o, err
		}
	}
	if in.HostIds != nil {
		ids := []string{}
		for _, id := range *in.HostIds {
			if slices.Contains(ids, id) {
				continue
			}
			if _, err := m.d.Agents.Get(ctx, id); err != nil {
				return o, httpx.Invalid("找不到这台电脑")
			}
			ids = append(ids, id)
		}
		if len(ids) > maxHosts {
			return o, httpx.Invalid(fmt.Sprintf("电脑最多选 %d 台", maxHosts))
		}
		o.HostIDs = ids
	}
	for _, f := range []struct {
		in  *string
		dst *string
	}{{in.HaStart, &o.HAStart}, {in.HaEnd, &o.HAEnd}} {
		if f.in == nil {
			continue
		}
		v := *f.in
		if v != "" && !entityPattern.MatchString(v) {
			return o, httpx.Invalid("Home Assistant 的场景或脚本要写成 scene.xxx 或 script.xxx")
		}
		*f.dst = v
	}
	return o, nil
}
