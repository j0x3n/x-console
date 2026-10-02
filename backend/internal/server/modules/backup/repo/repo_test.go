package repo

import (
	"bytes"
	"context"
	"errors"
	"io"
	"iter"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

type memoryStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	puts    map[string]int
	lists   map[string]int
	fail    func(string, string) error
	active  atomic.Int64
	peak    atomic.Int64
	pause   chan struct{}
}

func memory() *memoryStore {
	return &memoryStore{objects: map[string][]byte{}, puts: map[string]int{}, lists: map[string]int{}}
}
func (s *memoryStore) fault(method, key string) error {
	if s.fail != nil {
		return s.fail(method, key)
	}
	return nil
}
func (s *memoryStore) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if err := s.fault("put", key); err != nil {
		return err
	}
	if strings.HasPrefix(key, "blobs/") {
		n := s.active.Add(1)
		defer s.active.Add(-1)
		for p := s.peak.Load(); n > p; p = s.peak.Load() {
			if s.peak.CompareAndSwap(p, n) {
				break
			}
		}
		if s.pause != nil {
			select {
			case <-s.pause:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if int64(len(raw)) != size {
		return errors.New("wrong size")
	}
	s.mu.Lock()
	s.objects[key] = raw
	s.puts[key]++
	s.mu.Unlock()
	return nil
}
func (s *memoryStore) Get(ctx context.Context, key string) (io.ReadCloser, files.Info, error) {
	if err := s.fault("get", key); err != nil {
		return nil, files.Info{}, err
	}
	s.mu.Lock()
	raw, ok := s.objects[key]
	raw = bytes.Clone(raw)
	s.mu.Unlock()
	if !ok {
		return nil, files.Info{}, files.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(raw)), files.Info{Key: key, Size: int64(len(raw))}, nil
}
func (s *memoryStore) Stat(ctx context.Context, key string) (files.Info, error) {
	rc, info, err := s.Get(ctx, key)
	if rc != nil {
		rc.Close()
	}
	return info, err
}
func (s *memoryStore) Delete(ctx context.Context, key string) error {
	if err := s.fault("delete", key); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.objects, key)
	s.mu.Unlock()
	return nil
}
func (s *memoryStore) List(ctx context.Context, prefix string) iter.Seq2[files.Info, error] {
	return func(yield func(files.Info, error) bool) {
		if err := s.fault("list", prefix); err != nil {
			yield(files.Info{}, err)
			return
		}
		s.mu.Lock()
		s.lists[prefix]++
		var all []files.Info
		for key, raw := range s.objects {
			if prefix == "" || strings.HasPrefix(key, prefix+"/") {
				all = append(all, files.Info{Key: key, Size: int64(len(raw))})
			}
		}
		s.mu.Unlock()
		sort.Slice(all, func(i, j int) bool { return all[i].Key < all[j].Key })
		for _, info := range all {
			if !yield(info, nil) {
				return
			}
		}
	}
}

var master = bytes.Repeat([]byte{7}, 32)

func setupRepo(t *testing.T) (*Repository, *memoryStore, *Lease, string) {
	t.Helper()
	s := memory()
	dir := t.TempDir()
	r, l, err := Acquire(context.Background(), s, master, dir, "backup", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil && !errors.Is(err, ErrLostLock) {
			t.Error(err)
		}
	})
	return r, s, l, dir
}

func input(t *testing.T, content map[string]string, at time.Time) Input {
	t.Helper()
	local := files.Local{Root: t.TempDir()}
	ctx := context.Background()
	for key, v := range content {
		if err := local.Put(ctx, key, strings.NewReader(v), int64(len(v))); err != nil {
			t.Fatal(err)
		}
	}
	var entries []files.Info
	for info, err := range local.List(ctx, "") {
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, info)
	}
	return Input{Database: strings.NewReader("sqlite database"), DatabaseSize: 15, Files: local, Entries: entries, CreatedAt: at, Migration: "1", Version: "test"}
}

func create(t *testing.T, r *Repository, content map[string]string, at time.Time) Snapshot {
	t.Helper()
	s, err := r.CreateSnapshot(context.Background(), input(t, content, at))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSnapshotsDeduplicateRestoreAndPrune(t *testing.T) {
	r, store, _, _ := setupRepo(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC)
	first := create(t, r, map[string]string{"notes/a": "one", "drive/a": "same"}, at)
	second := create(t, r, map[string]string{"notes/a": "one", "drive/a": "same"}, at.Add(time.Hour))
	if second.UploadedBytes != int64(len(store.objects[snapshotPath(second.ID)])) || second.Added != 0 || second.Modified != 0 || second.Deleted != 0 {
		t.Fatalf("second: %+v", second)
	}
	third := create(t, r, map[string]string{"notes/a": "two"}, at.Add(2*time.Hour))
	if third.Modified != 1 || third.Deleted != 1 || third.UploadedBytes == 0 {
		t.Fatalf("third: %+v", third)
	}
	if store.lists["blobs"] != 3 {
		t.Fatalf("blob list count: %d", store.lists["blobs"])
	}
	for key, n := range store.puts {
		if strings.HasPrefix(key, "blobs/") && n != 1 {
			t.Fatalf("reuploaded %s", key)
		}
	}
	dest := files.Local{Root: t.TempDir()}
	var db bytes.Buffer
	if _, err := r.Restore(ctx, first.ID, &db, dest, nil); err != nil {
		t.Fatal(err)
	}
	if db.String() != "sqlite database" {
		t.Fatal(db.String())
	}
	rc, _, err := dest.Get(ctx, "drive/a")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(rc)
	rc.Close()
	if string(raw) != "same" {
		t.Fatal(string(raw))
	}
	if _, err := r.Check(ctx); err != nil {
		t.Fatal(err)
	}
	v, err := r.Prune(ctx, Retention{Last: 1}, at.Add(3*time.Hour), time.UTC)
	if err != nil || v.Snapshots != 2 || v.Blocks != 2 {
		t.Fatalf("prune: %+v %v", v, err)
	}
	if _, err := r.ReadSnapshot(ctx, first.ID); !errors.Is(err, files.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := r.Check(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestKeyAuthenticationAndCorruption(t *testing.T) {
	r, s, _, _ := setupRepo(t)
	ctx := context.Background()
	first := create(t, r, map[string]string{"notes/secret": "hidden filename content"}, time.Now())
	if _, err := Open(ctx, s, bytes.Repeat([]byte{9}, 32)); !errors.Is(err, ErrKey) {
		t.Fatal(err)
	}
	for _, raw := range s.objects {
		if bytes.Contains(raw, []byte("hidden filename content")) || bytes.Contains(raw, []byte("notes/secret")) {
			t.Fatal("plaintext leaked")
		}
	}
	key := blobPath(first.Files[0].Blocks[0].Hash)
	s.objects[key][len(s.objects[key])-1] ^= 1
	if _, err := r.Check(ctx); err != nil {
		t.Fatal("light check unexpectedly checked content", err)
	}
	if _, err := r.Restore(ctx, first.ID, io.Discard, files.Local{Root: t.TempDir()}, nil); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	s.objects[key] = s.objects[key][:len(s.objects[key])-1]
	if _, err := r.Check(ctx); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
}

func TestFailureNeverPublishesAndPruneStopsOnUnreadableSnapshot(t *testing.T) {
	r, s, _, _ := setupRepo(t)
	ctx := context.Background()
	at := time.Now()
	first := create(t, r, map[string]string{"notes/a": "old"}, at)
	s.fail = func(method, key string) error {
		if method == "put" && strings.HasPrefix(key, "blobs/") {
			return errors.New("offline")
		}
		return nil
	}
	if _, err := r.CreateSnapshot(ctx, input(t, map[string]string{"notes/a": "new"}, at.Add(time.Hour))); err == nil {
		t.Fatal("accepted failed upload")
	}
	s.fail = nil
	all, err := r.ListSnapshots(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("snapshots %d %v", len(all), err)
	}
	create(t, r, map[string]string{"notes/a": "new"}, at.Add(2*time.Hour))
	before := len(s.objects)
	s.objects[snapshotPath(first.ID)] = []byte("broken")
	if _, err := r.Prune(ctx, Retention{Last: 1}, at.Add(3*time.Hour), time.UTC); err == nil {
		t.Fatal("accepted corrupt snapshot")
	}
	if len(s.objects) != before {
		t.Fatal("deleted objects despite unreadable snapshot")
	}
	s.fail = func(method, key string) error {
		if method == "list" {
			return errors.New("list failed")
		}
		return nil
	}
	if _, err := r.ListSnapshots(ctx); err == nil {
		t.Fatal("list failure became empty result")
	}
	s.fail = nil
}

func TestLeaseRejectsOtherInstancesAndProcesses(t *testing.T) {
	r, s, l, dir := setupRepo(t)
	ctx := context.Background()
	if _, _, err := Acquire(ctx, s, master, dir, "backup", true); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Acquire(ctx, s, master, t.TempDir(), "backup", true); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
	owner, _ := os.ReadFile(filepath.Join(dir, "backup-instance"))
	if err := r.writeLock(ctx, remoteLock{Instance: strings.Repeat("a", 32), Token: strings.Repeat("b", 32), StartedAt: time.Now().Add(-24 * time.Hour), HeartbeatAt: time.Now().Add(-24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Acquire(ctx, s, master, dir, "backup", false); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
	if err := r.writeLock(ctx, remoteLock{Instance: string(owner), Token: strings.Repeat("b", 32), StartedAt: time.Now().Add(-24 * time.Hour), HeartbeatAt: time.Now().Add(-24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	_, next, err := Acquire(ctx, s, master, dir, "backup", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFourUploadsAndChunkBoundaries(t *testing.T) {
	r, s, l, _ := setupRepo(t)
	s.pause = make(chan struct{})
	ctx := l.Context()
	content := make([]byte, ChunkSize*5+1)
	for i := range content {
		content[i] = byte(i / ChunkSize)
	}
	in := input(t, nil, time.Now())
	in.Database = bytes.NewReader(content)
	in.DatabaseSize = int64(len(content))
	done := make(chan error, 1)
	go func() {
		snap, err := r.CreateSnapshot(ctx, in)
		if err == nil && len(snap.Database.Blocks) != 6 {
			err = errors.New("wrong chunk count")
		}
		done <- err
	}()
	deadline := time.Now().Add(10 * time.Second)
	for s.peak.Load() < 4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.peak.Load() != 4 {
		close(s.pause)
		t.Fatalf("peak: %d", s.peak.Load())
	}
	close(s.pause)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if s.peak.Load() > 4 {
		t.Fatal("more than four uploads")
	}
}

func TestRetentionUsesNaturalCalendarPeriods(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, loc)
	var all []Snapshot
	for i, text := range []string{"2026-03-09T10:00:00-04:00", "2026-03-09T09:00:00-04:00", "2026-03-08T23:00:00-04:00", "2026-03-02T12:00:00-05:00", "2026-02-28T12:00:00-05:00", "2026-01-31T12:00:00-05:00"} {
		at, _ := time.Parse(time.RFC3339, text)
		all = append(all, Snapshot{ID: string(rune('a' + i)), CreatedAt: at})
	}
	keep := Keep(all, Retention{Last: 2, Daily: 2, Weekly: 2, Monthly: 2}, now, loc)
	for _, id := range []string{"a", "b", "c", "e"} {
		if !keep[id] {
			t.Fatalf("missing %s: %v", id, keep)
		}
	}
	if keep["d"] || keep["f"] || len(keep) != 4 {
		t.Fatal(keep)
	}
}

func TestChangesTruncateAndPathsAreSafe(t *testing.T) {
	var s Snapshot
	for i := 0; i < 205; i++ {
		s.Files = append(s.Files, File{Path: "notes/" + strings.Repeat("a", i+1)})
	}
	setChanges(&s, nil)
	if len(s.Changes) != 200 || !s.ChangesTruncated || s.Added != 205 {
		t.Fatalf("changes %+v", s)
	}
	for _, key := range []string{"backups/repo", "notes/../a", "notes/a:b", "notes/CON", "notes/a.", "notes/a\\b", "/notes/a"} {
		if ValidatePath(key) == nil {
			t.Fatal(key)
		}
	}
}
