package metrics

import (
	"context"
	"encoding/json"
	"reflect"
	"runtime"
	"testing"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/shirou/gopsutil/v4/cpu"
)

func TestBusyPercent(t *testing.T) {
	a := cpu.TimesStat{User: 10, System: 5, Idle: 85}
	b := cpu.TimesStat{User: 20, System: 10, Idle: 170}
	// 15 busy of 100 elapsed.
	if got := BusyPercent(a, b); got < 14.99 || got > 15.01 {
		t.Fatalf("busy = %v", got)
	}
	if got := BusyPercent(b, a); got != 0 {
		t.Fatalf("reset = %v", got)
	}
}

func TestRate(t *testing.T) {
	if got := Rate(100, 1100, 10); got != 100 {
		t.Fatalf("rate = %v", got)
	}
	if got := Rate(100, 50, 10); got != 0 {
		t.Fatalf("reset = %v", got)
	}
}

func TestInterfaceRates(t *testing.T) {
	prev := map[string]nicCounters{"eth0": {rx: 100, tx: 200}}
	cur := map[string]nicCounters{"eth1": {rx: 900, tx: 900}, "eth0": {rx: 120, tx: 210}}
	got := interfaceRates(prev, cur, 2)
	if len(got) != 2 || got[0].Name != "eth0" || got[0].RxRate != 10 || got[0].TxRate != 5 || got[1].Name != "eth1" || got[1].RxRate != 0 {
		t.Fatalf("interface rates: %+v", got)
	}
}

func TestSummaryPayloadRoundTrip(t *testing.T) {
	s := protocol.MetricsSample{CPU: 40, MemUsed: 50, MemTotal: 100,
		Disks:     []protocol.DiskUsage{{Mount: "/", Used: 80, Total: 100}},
		NetRxRate: 120, NetTxRate: 40, CPUPerCore: []float64{10, 70},
		NetInterfaces: []protocol.NetInterface{{Name: "eth0", RxRate: 120}}}
	b, err := json.Marshal(summaryPayload(s))
	if err != nil {
		t.Fatal(err)
	}
	var restored protocol.MetricsSample
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.CPU != 40 || restored.MemUsed != 50 || len(restored.Disks) != 1 || restored.NetRxRate != 120 {
		t.Fatalf("lost summary fields: %+v", restored)
	}
	if len(restored.CPUPerCore) != 0 || len(restored.NetInterfaces) != 0 {
		t.Fatalf("sent detail fields: %s", b)
	}
}

func TestPhysicalDisks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("linux device names")
	}
	got := PhysicalDisks([]string{"sda", "sda1", "sda2", "nvme0n1", "nvme0n1p1", "loop0", "dm-0", "vdb"})
	want := []string{"nvme0n1", "sda", "vdb"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestSampleOnThisMachine(t *testing.T) {
	c := NewCollector()
	s, err := c.Sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.MemTotal == 0 || s.MemUsed == 0 || s.MemUsed > s.MemTotal {
		t.Fatalf("memory: %+v", s)
	}
	if s.CPU < 0 || s.CPU > 100 || len(s.CPUPerCore) == 0 {
		t.Fatalf("cpu: %v %v", s.CPU, s.CPUPerCore)
	}
	if s.Procs == 0 || s.At.IsZero() {
		t.Fatalf("sample: %+v", s)
	}
	if s.Disks == nil {
		t.Fatal("disks must not be nil")
	}
}

func TestSummaryOnThisMachine(t *testing.T) {
	s, err := NewCollector().SampleSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.MemTotal == 0 || s.At.IsZero() || len(s.CPUPerCore) != 0 || len(s.Disks) > 1 {
		t.Fatalf("summary: %+v", s)
	}
}

func TestSystemInfo(t *testing.T) {
	info := SystemInfo()
	if info.Hostname == "" || info.CPUCores == 0 || info.MemoryTotal == 0 || info.OS != runtime.GOOS {
		t.Fatalf("info: %+v", info)
	}
}
