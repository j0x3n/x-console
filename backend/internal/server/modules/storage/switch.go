package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage/api"
)

// saveEvery is the shortest gap between two progress writes.
const saveEvery = time.Second

// move is one run of copying every file to the other location.
type move struct {
	cancel context.CancelFunc
	done   chan struct{} // closed when the run has finished

	mu       sync.Mutex
	state    api.StorageMigration
	savedAt  time.Time
	canceled bool
}

func (mv *move) snapshot() api.StorageMigration {
	mv.mu.Lock()
	defer mv.mu.Unlock()
	return mv.state
}

func (mv *move) update(f func(s *api.StorageMigration)) api.StorageMigration {
	mv.mu.Lock()
	defer mv.mu.Unlock()
	f(&mv.state)
	return mv.state
}

// SwitchStorage is POST /storage/switch.
func (m *Module) SwitchStorage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.SwitchStorageJSONBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	deleteSource := in.DeleteSource != nil && *in.DeleteSource
	err := m.startMove(ctx, in.Target, deleteSource)
	m.d.Audit.Record(ctx, "storage.switch.start", string(in.Target), map[string]any{"deleteSource": deleteSource}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// CancelStorageSwitch is POST /storage/switch/cancel.
func (m *Module) CancelStorageSwitch(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	mv := m.move
	m.mu.Unlock()
	if mv == nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "conflict", "现在没有在搬迁"))
		return
	}
	mv.mu.Lock()
	mv.canceled = true
	mv.mu.Unlock()
	mv.cancel()
	<-mv.done
	w.WriteHeader(http.StatusNoContent)
}

// startMove checks the request, then runs the copy in the background.
func (m *Module) startMove(ctx context.Context, target api.StorageBackend, deleteSource bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.move != nil {
		return httpx.NewError(http.StatusConflict, "conflict", "已经在搬迁了")
	}
	if target != api.Local && target != api.S3 {
		return httpx.Invalid("目标位置不正确")
	}
	if target == m.backend {
		return httpx.Invalid("现在已经在用这个位置了")
	}
	dst, err := m.targetStore(ctx, target)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	mv := &move{done: make(chan struct{}), state: api.StorageMigration{State: api.Running, Target: target, StartedAt: &now}}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	mv.cancel = cancel
	m.move = mv
	first := mv.state
	m.saveMigration(runCtx, first)
	m.d.Bus.Publish("storage.progress", first)
	// From now on new files are written to both places.
	m.d.Files.SetMirror(dst)
	go m.run(runCtx, mv, m.raw, dst, target, deleteSource)
	return nil
}

// targetStore opens the place files will move to. For s3 it checks that the
// bucket answers first.
func (m *Module) targetStore(ctx context.Context, target api.StorageBackend) (files.Store, error) {
	if target == api.Local {
		return m.local(), nil
	}
	s, secret, err := m.s3Settings(ctx)
	if err != nil {
		return nil, err
	}
	if !s.complete(secret) {
		return nil, httpx.ErrIntegrationMissing
	}
	remote, err := files.NewS3(s.config(secret))
	if err != nil {
		return nil, httpx.Invalid(err.Error())
	}
	if err := remote.Check(ctx); err != nil {
		return nil, httpx.Invalid("连接测试没通过：" + err.Error())
	}
	return remote, nil
}

func (m *Module) saveMigration(ctx context.Context, s api.StorageMigration) {
	if err := m.d.Settings.Set(context.WithoutCancel(ctx), keyMigration, s); err != nil {
		m.d.Log.Warn("storage: save migration state", "error", err)
	}
}

// run copies everything, checks it, then switches. It is the only writer of mv.
func (m *Module) run(ctx context.Context, mv *move, src, dst files.Store, target api.StorageBackend, deleteSource bool) {
	defer close(mv.done)
	err := m.copyAll(ctx, mv, src, dst)
	if err == nil {
		err = m.finish(ctx, mv, dst, target)
	}
	if err == nil && deleteSource {
		// The switch is done, so a failure here is not a failed move.
		if derr := deleteAll(context.WithoutCancel(ctx), src); derr != nil {
			m.d.Log.Warn("storage: delete old files", "error", derr)
		}
	}
	m.d.Audit.Record(context.WithoutCancel(ctx), "storage.switch", string(target),
		map[string]any{"files": mv.snapshot().DoneFiles, "deleteSource": deleteSource}, err)

	final := m.settle(ctx, mv, err)
	m.mu.Lock()
	if err != nil {
		m.d.Files.SetMirror(nil)
	}
	m.last, m.move = final, nil
	m.usage.clear()
	m.mu.Unlock()
	m.saveMigration(ctx, final)
	m.d.Bus.Publish("storage.changed", final)
}

// settle writes the last state of a run.
func (m *Module) settle(ctx context.Context, mv *move, err error) api.StorageMigration {
	now := time.Now().UTC()
	return mv.update(func(s *api.StorageMigration) {
		s.FinishedAt = &now
		switch {
		case err == nil:
			s.State, s.Error = api.Done, nil
		case mv.canceled:
			s.State, s.Error = api.Canceled, nil
		default:
			s.State, s.Error = api.Failed, ptr(err.Error())
		}
	})
}

// copyAll copies every file of src that dst does not already have.
func (m *Module) copyAll(ctx context.Context, mv *move, src, dst files.Store) error {
	var list []files.Info
	var totalBytes int64
	for info, err := range siteFiles(ctx, src) {
		if err != nil {
			return fmt.Errorf("读取原位置的文件列表失败：%w", err)
		}
		list = append(list, info)
		totalBytes += info.Size
	}
	m.progress(ctx, mv, true, func(s *api.StorageMigration) {
		s.TotalFiles, s.TotalBytes = int64(len(list)), totalBytes
	})
	for _, info := range list {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyOne(ctx, src, dst, info); err != nil {
			return err
		}
		m.progress(ctx, mv, false, func(s *api.StorageMigration) {
			s.DoneFiles++
			s.DoneBytes += info.Size
		})
	}
	m.progress(ctx, mv, true, func(*api.StorageMigration) {})
	return nil
}

// copyOne copies one file. A file that is already there with the same size is
// left alone, which is what makes a retry cheap. A file that vanished while
// the move ran was deleted by the user, so it is skipped.
func copyOne(ctx context.Context, src, dst files.Store, info files.Info) error {
	if have, err := dst.Stat(ctx, info.Key); err == nil && have.Size == info.Size {
		return nil
	}
	rc, got, err := src.Get(ctx, info.Key)
	if errors.Is(err, files.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s：%w", info.Key, err)
	}
	defer rc.Close()
	if err := dst.Put(ctx, info.Key, rc, got.Size); err != nil {
		return fmt.Errorf("%s：%w", info.Key, err)
	}
	return nil
}

// progress applies f and, at most once a second, saves and announces it.
func (m *Module) progress(ctx context.Context, mv *move, force bool, f func(*api.StorageMigration)) {
	s := mv.update(f)
	mv.mu.Lock()
	due := force || time.Since(mv.savedAt) >= saveEvery
	if due {
		mv.savedAt = time.Now()
	}
	mv.mu.Unlock()
	if due {
		m.saveMigration(ctx, s)
		m.d.Bus.Publish("storage.progress", s)
	}
}

// finish checks that dst holds every file of the current location and
// then switches. Writes made meanwhile go to both places, so none is lost.
func (m *Module) finish(ctx context.Context, mv *move, dst files.Store, target api.StorageBackend) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	have := map[string]int64{}
	for info, err := range siteFiles(ctx, dst) {
		if err != nil {
			return fmt.Errorf("核对新位置的文件失败：%w", err)
		}
		have[info.Key] = info.Size
	}
	for info, err := range siteFiles(ctx, m.raw) {
		if err != nil {
			return fmt.Errorf("核对原位置的文件失败：%w", err)
		}
		if size, ok := have[info.Key]; !ok || size != info.Size {
			return fmt.Errorf("%s：新位置的文件缺失或大小不一致", info.Key)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.clearCache(); err != nil {
		return err
	}
	raw, cache, err := m.build(ctx, target)
	if err != nil {
		return err
	}
	if err := m.d.Settings.Set(ctx, keyBackend, string(target)); err != nil {
		return err
	}
	m.backend, m.raw, m.cache = target, raw, cache
	m.d.Files.Swap(m.storeFor(raw, cache))
	return nil
}

// deleteAll removes every file of a location that was just left.
func deleteAll(ctx context.Context, s files.Store) error {
	var keys []string
	for info, err := range siteFiles(ctx, s) {
		if err != nil {
			return err
		}
		keys = append(keys, info.Key)
	}
	var errs []error
	for _, key := range keys {
		errs = append(errs, s.Delete(ctx, key))
	}
	return errors.Join(errs...)
}
