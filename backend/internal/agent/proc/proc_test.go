package proc

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestSort(t *testing.T) {
	items := []protocol.ProcessInfo{
		{PID: 3, Name: "b", CPU: 1, MemRSS: 10},
		{PID: 1, Name: "C", CPU: 5, MemRSS: 5},
		{PID: 2, Name: "a", CPU: 1, MemRSS: 30},
	}
	Sort(items, "cpu")
	if items[0].PID != 1 || items[1].PID != 2 || items[2].PID != 3 {
		t.Fatalf("cpu: %+v", items)
	}
	Sort(items, "mem")
	if items[0].PID != 2 {
		t.Fatalf("mem: %+v", items)
	}
	Sort(items, "name")
	if items[0].Name != "a" || items[2].Name != "C" {
		t.Fatalf("name: %+v", items)
	}
	Sort(items, "pid")
	if items[0].PID != 1 {
		t.Fatalf("pid: %+v", items)
	}
}

func TestCPUPercent(t *testing.T) {
	if got := CPUPercent(1, 1.5, 0.5); got != 100 {
		t.Fatalf("got %v", got)
	}
	if got := CPUPercent(2, 1, 1); got != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestListFindsSelf(t *testing.T) {
	sampleWindow = 50 * time.Millisecond
	list, err := List(context.Background(), protocol.ProcListParams{Sort: "pid", Limit: 100000})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range list.Items {
		if int(p.PID) == os.Getpid() {
			found = true
			if p.MemRSS == 0 || p.StartedAt.IsZero() {
				t.Fatalf("self: %+v", p)
			}
		}
	}
	if !found || list.Total < 1 {
		t.Fatalf("own process missing from %d items", len(list.Items))
	}
	if _, err := List(context.Background(), protocol.ProcListParams{Sort: "bogus"}); err == nil {
		t.Fatal("bad sort accepted")
	}
}

func TestKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip(err)
	}
	if err := Kill(context.Background(), protocol.ProcKillParams{PID: int32(cmd.Process.Pid), Signal: "bogus"}); err == nil {
		t.Fatal("bogus signal accepted")
	}
	if err := Kill(context.Background(), protocol.ProcKillParams{PID: int32(cmd.Process.Pid), Signal: "SIGKILL"}); err != nil {
		t.Fatal(err)
	}
	err := cmd.Wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("wait: %v", err)
	}
	if err := Kill(context.Background(), protocol.ProcKillParams{PID: int32(os.Getpid())}); err == nil {
		t.Fatal("killing self allowed")
	}
}
