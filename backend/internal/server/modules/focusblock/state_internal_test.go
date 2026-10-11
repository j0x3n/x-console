package focusblock

import (
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/focusblock/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/focusblock/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestComputerState(t *testing.T) {
	ok := api.FocusBlockComputer{Supported: true, Selected: true, Online: true}
	row := func(state, hosts string) db.FocusBlockAgent {
		return db.FocusBlockAgent{State: state, HostsStatus: hosts, Sites: 6, Killed: 3, HostsError: "access denied", Message: "boom"}
	}
	cases := []struct {
		name   string
		c      api.FocusBlockComputer
		row    db.FocusBlockAgent
		has    bool
		active bool
		want   api.FocusBlockComputerState
		text   string
	}{
		{"too old", api.FocusBlockComputer{Selected: true, Online: true}, db.FocusBlockAgent{}, false, true, api.Unsupported, "太旧"},
		{"not chosen", api.FocusBlockComputer{Supported: true, Online: true}, db.FocusBlockAgent{}, false, true, api.Off, ""},
		{"offline and quiet", api.FocusBlockComputer{Supported: true, Selected: true}, db.FocusBlockAgent{}, false, false, api.Offline, "不在线"},
		{"offline while blocking", api.FocusBlockComputer{Supported: true, Selected: true}, row("blocking", "ok"), true, true, api.Offline, "自己恢复"},
		{"waiting", ok, db.FocusBlockAgent{}, false, true, api.Idle, "等待"},
		{"idle", ok, row("idle", ""), true, false, api.Idle, "开始时生效"},
		{"error", ok, row("error", ""), true, true, api.Failed, "boom"},
		{"ok", ok, row("blocking", protocol.FocusBlockHostsOK), true, true, api.Active, "6 个网址名"},
		{"programs only", ok, row("blocking", protocol.FocusBlockHostsNone), true, true, api.Active, "只结束程序"},
		{"permission", ok, row("blocking", protocol.FocusBlockHostsPermission), true, true, api.Permission, "权限"},
		{"hosts failed", ok, row("blocking", protocol.FocusBlockHostsFailed), true, true, api.Failed, "access denied"},
	}
	for _, c := range cases {
		got, text := computerState(c.c, c.row, c.has, c.active)
		if got != c.want || !strings.Contains(text, c.text) {
			t.Errorf("%s: got %s %q, want %s %q", c.name, got, text, c.want, c.text)
		}
	}
}

func TestPlanTTL(t *testing.T) {
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		until time.Time
		want  int
	}{
		{now.Add(10 * time.Minute), 15 * 60},                        // the rest of the focus plus the grace time
		{now.Add(-time.Hour), 60},                                   // never below a minute
		{now.Add(10 * time.Hour), protocol.FocusBlockMaxTTLSeconds}, // never above what the agent accepts
	} {
		if got := (plan{until: c.until}).ttl(now); got != c.want {
			t.Errorf("until %v: ttl %d, want %d", c.until, got, c.want)
		}
	}
}
