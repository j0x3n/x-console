package drive

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
)

func TestTaskQueueCancelAndPrune(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := events.NewBus()
	batch, stop := bus.Subscribe("drive_item.batch", 8)
	defer stop()
	now := time.Now()
	m := &Module{d: &module.Deps{Bus: bus}, tasks: map[string]*driveTask{}, taskSlots: make(chan struct{}, 2), taskBase: ctx, taskNow: func() time.Time { return now }}
	started := make(chan string, 3)
	release := make(chan struct{})
	job := func(ctx context.Context, task *driveTask) error {
		started <- task.dto.Id
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	}
	a := m.startTask(api.Copy, "复制 A", 1, 10, job)
	b := m.startTask(api.Copy, "复制 B", 1, 10, job)
	for range 2 {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("two tasks did not start")
		}
	}
	c := m.startTask(api.Copy, "复制 C", 1, 10, job)
	select {
	case <-started:
		t.Fatal("third task bypassed the two-slot limit")
	default:
	}
	m.tasksMu.Lock()
	queued := m.tasks[c.Id].dto.Current
	m.tasksMu.Unlock()
	if queued == nil || *queued != "等待中" {
		t.Fatalf("queued task: %v", queued)
	}
	rec := httptest.NewRecorder()
	m.CancelDriveTask(rec, httptest.NewRequest(http.MethodPost, "/drive/tasks/"+c.Id+"/cancel", nil), c.Id)
	var canceled api.DriveTask
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &canceled) != nil || canceled.State != api.DriveTaskStateCanceled {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
	}
	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for {
		m.tasksMu.Lock()
		done := m.tasks[a.Id].dto.FinishedAt != nil && m.tasks[b.Id].dto.FinishedAt != nil && m.tasks[c.Id].dto.FinishedAt != nil
		m.tasksMu.Unlock()
		if done && len(batch) == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tasks did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(batch) != 3 {
		t.Fatalf("want one batch event per finished task, got %d", len(batch))
	}
	m.tasksMu.Lock()
	if m.tasks[c.Id].dto.State != api.DriveTaskStateCanceled || m.tasks[a.Id].dto.State != api.DriveTaskStateDone || m.tasks[b.Id].dto.State != api.DriveTaskStateDone {
		t.Fatal("final task states")
	}
	m.tasksMu.Unlock()
	now = now.Add(11 * time.Minute)
	if err := m.pruneTasks(ctx); err != nil {
		t.Fatal(err)
	}
	if len(m.tasks) != 0 {
		t.Fatalf("finished tasks were not pruned: %d", len(m.tasks))
	}
}
