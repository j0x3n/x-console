package metrics

import (
	"context"
	"reflect"
	"runtime"
	"testing"

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

func TestSystemInfo(t *testing.T) {
	info := SystemInfo()
	if info.Hostname == "" || info.CPUCores == 0 || info.MemoryTotal == 0 || info.OS != runtime.GOOS {
		t.Fatalf("info: %+v", info)
	}
}
