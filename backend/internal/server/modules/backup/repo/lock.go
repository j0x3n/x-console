package repo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

type remoteLock struct {
	Instance    string    `json:"instance"`
	Token       string    `json:"token"`
	Kind        string    `json:"kind"`
	StartedAt   time.Time `json:"startedAt"`
	HeartbeatAt time.Time `json:"heartbeatAt"`
}

type Lease struct {
	r      *Repository
	file   *os.File
	lock   remoteLock
	mu     sync.Mutex
	cancel context.CancelFunc
	ctx    context.Context
	done   chan struct{}
	closed bool
}

func Acquire(ctx context.Context, s Store, master []byte, dataDir, kind string, initialize bool) (*Repository, *Lease, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(filepath.Join(dataDir, "backup-repo.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, nil, ErrLocked
	}
	fail := func(err error) (*Repository, *Lease, error) { unlockFile(f); f.Close(); return nil, nil, err }
	identityPath := filepath.Join(dataDir, "backup-instance")
	raw, err := os.ReadFile(identityPath)
	owner := string(raw)
	if errors.Is(err, os.ErrNotExist) {
		owner, err = randomID()
		if err == nil {
			err = os.WriteFile(identityPath, []byte(owner), 0o600)
		}
	}
	if err != nil {
		return fail(err)
	}
	if !validHex(owner, 32) {
		return fail(fmt.Errorf("备份实例标识已损坏"))
	}
	r, err := Open(ctx, s, master)
	if errors.Is(err, ErrMissing) && initialize {
		r, err = Init(ctx, s, master, owner)
	}
	if err != nil {
		return fail(err)
	}
	if r.cfg.Owner != owner {
		return fail(ErrLocked)
	}
	previous, err := r.readLock(ctx)
	if err == nil {
		if previous.Instance != owner || r.now().Sub(previous.HeartbeatAt) < 6*time.Hour {
			return fail(ErrLocked)
		}
	} else if !errors.Is(err, files.ErrNotFound) {
		return fail(err)
	}
	token, err := randomID()
	if err != nil {
		return fail(err)
	}
	at := r.now().UTC()
	leaseCtx, cancel := context.WithCancel(ctx)
	l := &Lease{r: r, file: f, ctx: leaseCtx, cancel: cancel, done: make(chan struct{}), lock: remoteLock{Instance: owner, Token: token, Kind: kind, StartedAt: at, HeartbeatAt: at}}
	if err := r.writeLock(ctx, l.lock); err != nil {
		cancel()
		return fail(err)
	}
	if err := l.verify(ctx); err != nil {
		cancel()
		return fail(err)
	}
	r.guard = l.Check
	go l.heartbeat()
	return r, l, nil
}

func LockRuntime(dataDir string) (io.Closer, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dataDir, "server-runtime.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("数据目录已有服务实例：%w", ErrLocked)
	}
	return runtimeLease{f}, nil
}

type runtimeLease struct{ file *os.File }

func (l runtimeLease) Close() error {
	unlockFile(l.file)
	return l.file.Close()
}

func (r *Repository) readLock(ctx context.Context) (remoteLock, error) {
	var v remoteLock
	raw, err := readObject(ctx, r.store, "lock.json", 64<<10)
	if err != nil {
		return v, err
	}
	var env envelope
	if json.Unmarshal(raw, &env) != nil || env.Format != 1 {
		return v, ErrLostLock
	}
	plain, err := unseal(r.lockKey, env.Data, r.aad("lock", "", 0))
	if err != nil || json.Unmarshal(plain, &v) != nil || !validHex(v.Token, 32) || !validHex(v.Instance, 32) || v.HeartbeatAt.IsZero() {
		return v, ErrLostLock
	}
	return v, nil
}

func (r *Repository) writeLock(ctx context.Context, v remoteLock) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data, err := seal(r.lockKey, raw, r.aad("lock", "", 0))
	if err != nil {
		return err
	}
	raw, err = json.Marshal(envelope{Format: 1, Data: data})
	if err != nil {
		return err
	}
	return r.store.Put(ctx, "lock.json", bytes.NewReader(raw), int64(len(raw)))
}

func (l *Lease) Context() context.Context { return l.ctx }

func (l *Lease) verify(ctx context.Context) error {
	v, err := l.r.readLock(ctx)
	if err != nil {
		return err
	}
	if v.Instance != l.lock.Instance || v.Token != l.lock.Token {
		return ErrLostLock
	}
	return nil
}

func (l *Lease) Check(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.ctx.Err() != nil {
		return ErrLostLock
	}
	if err := l.verify(ctx); err != nil {
		l.cancel()
		return err
	}
	return nil
}

func (l *Lease) heartbeat() {
	defer close(l.done)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(l.ctx, 30*time.Second)
			l.mu.Lock()
			err := l.verify(ctx)
			if err == nil {
				l.lock.HeartbeatAt = l.r.now().UTC()
				err = l.r.writeLock(ctx, l.lock)
			}
			l.mu.Unlock()
			cancel()
			if err != nil {
				l.cancel()
				return
			}
		}
	}
}

func (l *Lease) Close() error {
	l.cancel()
	<-l.done
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := l.verify(ctx)
	if err == nil {
		err = l.r.store.Delete(ctx, "lock.json")
	}
	unlockFile(l.file)
	cerr := l.file.Close()
	return errors.Join(err, cerr)
}

func (r *Repository) writable(ctx context.Context) error {
	if r.guard == nil {
		return ErrLocked
	}
	return r.guard(ctx)
}
