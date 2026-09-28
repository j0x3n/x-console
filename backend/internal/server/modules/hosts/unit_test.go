package hosts

import (
	"errors"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestRing(t *testing.T) {
	s := newMetricStore(3)
	base := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		s.add("a", protocol.MetricsSample{At: base.Add(time.Duration(i) * 10 * time.Second), CPU: float64(i)})
	}
	all := s.between("a", base, base.Add(time.Hour))
	if len(all) != 3 || all[0].CPU != 2 || all[2].CPU != 4 {
		t.Fatalf("ring: %+v", all)
	}
	if x, ok := s.latest("a"); !ok || x.CPU != 4 {
		t.Fatalf("latest: %+v", x)
	}
	// Throttle: two samples 2s apart publish once.
	if !s.add("b", protocol.MetricsSample{At: base}) || s.add("b", protocol.MetricsSample{At: base.Add(2 * time.Second)}) {
		t.Fatal("throttle")
	}
	if !s.add("b", protocol.MetricsSample{At: base.Add(5 * time.Second)}) {
		t.Fatal("publish after interval")
	}
}

func TestParseProcSnapshot(t *testing.T) {
	x, err := parseProcSnapshot(cannedProc)
	if err != nil {
		t.Fatal(err)
	}
	// cpu: total 1000 -> 1200 (+200), idle 800 -> 900 (+100) => 50% busy.
	if x.CPU != 50 {
		t.Fatalf("cpu %v", x.CPU)
	}
	if x.NetRxRate != 3000 || x.NetTxRate != 500 {
		t.Fatalf("net %v %v", x.NetRxRate, x.NetTxRate)
	}
	if x.MemTotal != 2000000*1024 || x.MemUsed != 1000000*1024 || x.SwapUsed != 60000*1024 {
		t.Fatalf("mem %+v", x)
	}
	if x.Load1 != 0.5 || x.Procs != 123 || x.UptimeSeconds != 12345 {
		t.Fatalf("load %+v", x)
	}
	if len(x.Disks) != 2 || x.Disks[0].Mount != "/" || x.Disks[1].Mount != "/mnt/my data" {
		t.Fatalf("disks %+v", x.Disks)
	}
	if _, err := parseProcSnapshot("garbage"); err == nil {
		t.Fatal("garbage parsed")
	}
}

func TestBucket(t *testing.T) {
	base := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	var rows []rollupRow
	for i := 0; i < 10; i++ {
		rows = append(rows, rollupRow{At: base.Add(time.Duration(i) * time.Minute), CPU: float64(i), DiskJSON: "[]"})
	}
	b := bucket(rows, 5*time.Minute)
	if len(b) != 2 || b[0].CPU != 2 || b[1].CPU != 7 || !b[1].At.Equal(base.Add(5*time.Minute)) {
		t.Fatalf("buckets: %+v", b)
	}
}

func TestCheck(t *testing.T) {
	now := time.Now().UTC()
	disk := 95.0
	h := api.Host{Online: true, Metrics: &api.MetricsSample{At: now, Cpu: 50, MemUsed: 90, MemTotal: 100}, Disk: &disk}
	cases := []struct {
		rule db.AlertRule
		want bool
	}{
		{db.AlertRule{Metric: "cpu", Op: "gt", Threshold: 40}, true},
		{db.AlertRule{Metric: "cpu", Op: "lt", Threshold: 40}, false},
		{db.AlertRule{Metric: "memory", Op: "gt", Threshold: 80}, true},
		{db.AlertRule{Metric: "disk", Op: "gt", Threshold: 90}, true},
		{db.AlertRule{Metric: "offline"}, false},
	}
	for _, c := range cases {
		got, _, _, ok := check(c.rule, h, now)
		if !ok || got != c.want {
			t.Fatalf("%+v: got %v ok %v", c.rule, got, ok)
		}
	}
	stale := h
	stale.Metrics = &api.MetricsSample{At: now.Add(-time.Hour), Cpu: 99}
	if _, _, _, ok := check(db.AlertRule{Metric: "cpu", Threshold: 1}, stale, now); ok {
		t.Fatal("stale sample used")
	}
	seen := now.Add(-10 * time.Minute)
	off := api.Host{Online: false, LastSeenAt: &seen}
	breached, value, since, ok := check(db.AlertRule{Metric: "offline"}, off, now)
	if !breached || !ok || !since.Equal(seen) || value < 9.9 {
		t.Fatalf("offline: %v %v %v %v", breached, value, since, ok)
	}
	if humanMinutes(0.5) != "不到 1 分钟" || humanMinutes(75) != "1 小时 15 分钟" {
		t.Fatal(humanMinutes(75))
	}
}

func TestAgentErr(t *testing.T) {
	cases := map[string]int{
		protocol.CodeNotFound:    404,
		protocol.CodePermission:  403,
		protocol.CodeExists:      409,
		protocol.CodeUnsupported: 501,
		protocol.CodeBadParams:   400,
		protocol.CodeFailed:      502,
	}
	for code, status := range cases {
		var he *httpx.Error
		if err := agentErr(&protocol.Error{Code: code, Message: "x"}); !errors.As(err, &he) || he.Status != status {
			t.Fatalf("%s: got %v want %d", code, err, status)
		}
		// The hub's mapped form is refined the same way.
		hub := &httpx.Error{Status: 502, Code: "agent_" + code, Message: "x"}
		if err := agentErr(hub); !errors.As(err, &he) || he.Status != status {
			t.Fatalf("hub %s: got %v want %d", code, err, status)
		}
	}
}

const cannedProc = `@@stat1
cpu  100 0 100 800 0 0 0 0 0 0
@@net1
Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:  5000      10    0    0    0     0          0         0     5000      10    0    0    0     0       0          0
  eth0: 10000      10    0    0    0     0          0         0     2000      10    0    0    0     0       0          0
@@stat2
cpu  150 0 150 900 0 0 0 0 0 0
@@net2
    lo:  9000      10    0    0    0     0          0         0     9000      10    0    0    0     0       0          0
  eth0: 13000      10    0    0    0     0          0         0     2500      10    0    0    0     0       0          0
@@mem
MemTotal:        2000000 kB
MemFree:          500000 kB
MemAvailable:    1000000 kB
SwapTotal:        100000 kB
SwapFree:          40000 kB
@@load
0.50 0.40 0.30 2/123 4567
@@uptime
12345.67 40000.00
@@df
Filesystem     1024-blocks    Used Available Capacity Mounted on
udev               1000000       0   1000000       0% /dev
/dev/vda1         10000000 9200000    800000      92% /
/dev/vdb           2000000  500000   1500000      25% /mnt/my data
tmpfs               100000    1000     99000       1% /run
@@end
`
