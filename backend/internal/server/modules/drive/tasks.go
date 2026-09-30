package drive

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
)

type driveTask struct {
	m       *Module
	dto     api.DriveTask // protected by tasksMu
	cancel  context.CancelFunc
	lastPub time.Time
}

type taskJob func(context.Context, *driveTask) error

// startTask detaches a job from the HTTP request. Jobs wait for one of two
// slots; the queue stays visible as a running task with Current="等待中".
func (m *Module) startTask(kind api.DriveTaskKind, title string, totalItems int, totalBytes int64, job taskJob, targetID ...int64) api.DriveTask {
	base := m.taskBase
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	queued := "等待中"
	dto := api.DriveTask{
		Id: secrets.RandomID(), Kind: kind, State: api.DriveTaskStateRunning,
		Title: title, TotalItems: totalItems, TotalBytes: totalBytes,
		CreatedAt: m.taskNow().UTC(), Current: &queued,
	}
	if len(targetID) != 0 {
		id := targetID[0]
		dto.TargetId = &id
	}
	t := &driveTask{m: m, cancel: cancel, dto: dto}
	m.tasksMu.Lock()
	m.tasks[t.dto.Id] = t
	m.tasksMu.Unlock()
	created := t.dto
	m.publishTask(created)
	go m.runTask(ctx, t, job)
	return created
}

func (m *Module) runTask(ctx context.Context, t *driveTask, job taskJob) {
	select {
	case m.taskSlots <- struct{}{}:
		defer func() { <-m.taskSlots }()
	case <-ctx.Done():
		m.finishTask(t, ctx.Err())
		return
	}
	if ctx.Err() != nil {
		m.finishTask(t, ctx.Err())
		return
	}
	m.changeTask(t, true, func(dto *api.DriveTask) { dto.Current = nil })
	err := job(ctx, t)
	m.finishTask(t, err)
}

// progress reports user-visible work. Progress events are throttled; callers
// can update after each item without flooding the browser event stream.
func (t *driveTask) progress(current string, doneItems int, doneBytes int64) {
	t.m.changeTask(t, false, func(dto *api.DriveTask) {
		if dto.State != api.DriveTaskStateRunning {
			return
		}
		dto.Current = &current
		dto.DoneItems = doneItems
		dto.DoneBytes = doneBytes
	})
}

func (m *Module) changeTask(t *driveTask, force bool, edit func(*api.DriveTask)) {
	m.tasksMu.Lock()
	before := t.dto.State
	edit(&t.dto)
	now := m.taskNow()
	publish := force || before != t.dto.State || now.Sub(t.lastPub) >= 300*time.Millisecond
	var dto api.DriveTask
	if publish {
		t.lastPub = now
		dto = t.dto
	}
	m.tasksMu.Unlock()
	if publish {
		m.publishTask(dto)
	}
}

func (m *Module) publishTask(dto api.DriveTask) {
	m.d.Bus.Publish("drive_task.updated", dto)
}

func (m *Module) finishTask(t *driveTask, err error) {
	defer t.cancel()
	m.changeTask(t, true, func(dto *api.DriveTask) {
		if dto.FinishedAt != nil {
			return
		}
		if dto.State == api.DriveTaskStateCanceled || errors.Is(err, context.Canceled) {
			dto.State = api.DriveTaskStateCanceled
		} else if err != nil {
			dto.State = api.DriveTaskStateFailed
			var apiErr *httpx.Error
			if errors.As(err, &apiErr) {
				dto.Error = &apiErr.Message
				dto.ErrorCode = &apiErr.Code
			} else {
				msg := "处理失败，请查看服务端日志"
				dto.Error = &msg
				slog.Error("drive task failed", "task", dto.Id, "kind", dto.Kind, "error", err)
			}
		} else {
			dto.State = api.DriveTaskStateDone
		}
		now := m.taskNow().UTC()
		dto.FinishedAt = &now
		dto.Current = nil
	})
	m.d.Bus.Publish("drive_item.batch", map[string]any{"taskId": t.dto.Id})
}

func (m *Module) ListDriveTasks(w http.ResponseWriter, r *http.Request) {
	m.tasksMu.Lock()
	cutoff := m.taskNow().Add(-10 * time.Minute)
	items := make([]api.DriveTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		if t.dto.FinishedAt == nil || t.dto.FinishedAt.After(cutoff) {
			items = append(items, t.dto)
		}
	}
	m.tasksMu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (m *Module) CancelDriveTask(w http.ResponseWriter, r *http.Request, taskID string) {
	m.tasksMu.Lock()
	t := m.tasks[taskID]
	if t == nil {
		m.tasksMu.Unlock()
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if t.dto.FinishedAt != nil || t.dto.State == api.DriveTaskStateCanceled {
		dto := t.dto
		m.tasksMu.Unlock()
		httpx.JSON(w, http.StatusOK, dto)
		return
	}
	t.dto.State = api.DriveTaskStateCanceled
	dto := t.dto
	m.tasksMu.Unlock()
	t.cancel()
	m.publishTask(dto)
	httpx.JSON(w, http.StatusOK, dto)
}

func (m *Module) pruneTasks(context.Context) error {
	m.tasksMu.Lock()
	cutoff := m.taskNow().Add(-10 * time.Minute)
	for id, t := range m.tasks {
		if t.dto.FinishedAt != nil && !t.dto.FinishedAt.After(cutoff) {
			delete(m.tasks, id)
		}
	}
	m.tasksMu.Unlock()
	return nil
}
