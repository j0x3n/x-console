package files

import (
	"context"
	"errors"
	"io"
	"iter"
	"sync"
)

// Manager holds the Store the whole site uses right now. The storage module
// swaps it when the location changes. Modules take their Store with For and
// may keep it: every call goes to whatever Store is current.
type Manager struct {
	mu      sync.RWMutex
	current Store
	mirror  Store // during a move, every write also goes here
}

// NewManager starts with s as the current Store.
func NewManager(s Store) *Manager { return &Manager{current: s} }

// For returns the Store of one module.
func (m *Manager) For(module string) Store { return Scoped(live{m}, module) }

// Store returns a Store over all modules, for backup and storage tools.
func (m *Manager) Store() Store { return live{m} }

// Swap makes s the current Store and returns the previous one.
func (m *Manager) Swap(s Store) Store {
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.current
	m.current, m.mirror = s, nil
	return old
}

// SetMirror makes every write, delete and copy also happen on s, until
// Swap or SetMirror(nil). The storage move uses it so files written while
// the move runs are not lost.
func (m *Manager) SetMirror(s Store) {
	m.mu.Lock()
	m.mirror = s
	m.mu.Unlock()
}

func (m *Manager) stores() (current, mirror Store) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current, m.mirror
}

// live forwards every call to the Manager's current Store.
type live struct{ m *Manager }

func (l live) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	cur, mirror := l.m.stores()
	if err := cur.Put(ctx, key, r, size); err != nil || mirror == nil {
		return err
	}
	return mirrorCopy(ctx, cur, mirror, key)
}

// mirrorCopy copies key from one Store to another.
func mirrorCopy(ctx context.Context, from, to Store, key string) error {
	rc, info, err := from.Get(ctx, key)
	if err != nil {
		return err
	}
	defer rc.Close()
	return to.Put(ctx, key, rc, info.Size)
}

func (l live) Get(ctx context.Context, key string) (io.ReadCloser, Info, error) {
	cur, _ := l.m.stores()
	return cur.Get(ctx, key)
}

func (l live) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, Info, error) {
	cur, _ := l.m.stores()
	return cur.GetRange(ctx, key, offset, length)
}

func (l live) Stat(ctx context.Context, key string) (Info, error) {
	cur, _ := l.m.stores()
	return cur.Stat(ctx, key)
}

func (l live) Delete(ctx context.Context, key string) error {
	cur, mirror := l.m.stores()
	err := cur.Delete(ctx, key)
	if mirror != nil {
		err = errors.Join(err, mirror.Delete(ctx, key))
	}
	return err
}

func (l live) List(ctx context.Context, prefix string) iter.Seq2[Info, error] {
	cur, _ := l.m.stores()
	return cur.List(ctx, prefix)
}

func (l live) Copy(ctx context.Context, from, to string) error {
	cur, mirror := l.m.stores()
	if err := cur.Copy(ctx, from, to); err != nil || mirror == nil {
		return err
	}
	return mirrorCopy(ctx, cur, mirror, to)
}
