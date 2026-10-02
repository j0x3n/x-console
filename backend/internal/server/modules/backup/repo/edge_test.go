package repo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

type changedSource struct {
	files.Store
	onGet bool
}

func (s changedSource) Get(ctx context.Context, key string) (io.ReadCloser, files.Info, error) {
	rc, info, err := s.Store.Get(ctx, key)
	if s.onGet {
		info.ModTime = info.ModTime.Add(time.Second)
	}
	return rc, info, err
}

func (s changedSource) Stat(ctx context.Context, key string) (files.Info, error) {
	info, err := s.Store.Stat(ctx, key)
	if !s.onGet {
		info.ModTime = info.ModTime.Add(time.Second)
	}
	return info, err
}

func TestSnapshotRejectsSameSizeFileChangedBeforeOrDuringRead(t *testing.T) {
	for _, onGet := range []bool{true, false} {
		t.Run(fmt.Sprint(onGet), func(t *testing.T) {
			r, _, _, _ := setupRepo(t)
			in := input(t, map[string]string{"notes/a": "hello"}, time.Now())
			in.Files = changedSource{Store: in.Files, onGet: onGet}
			if _, err := r.CreateSnapshot(context.Background(), in); err == nil {
				t.Fatal("changed file accepted")
			}
			all, err := r.ListSnapshots(context.Background())
			if err != nil || len(all) != 0 {
				t.Fatalf("published %+v %v", all, err)
			}
		})
	}
}

func TestPruneStopsBeforeBlobsWhenSnapshotDeleteFails(t *testing.T) {
	r, s, _, _ := setupRepo(t)
	at := time.Now()
	ctx := context.Background()
	first := create(t, r, map[string]string{"notes/a": "old"}, at)
	create(t, r, map[string]string{"notes/a": "new"}, at.Add(time.Hour))
	before := len(s.objects)
	s.fail = func(method, key string) error {
		if method == "delete" && key == snapshotPath(first.ID) {
			return errors.New("delete failed")
		}
		return nil
	}
	if _, err := r.Prune(ctx, Retention{Last: 1}, at.Add(2*time.Hour), time.UTC); err == nil {
		t.Fatal("prune succeeded")
	}
	if len(s.objects) != before {
		t.Fatal("objects removed after failed snapshot delete")
	}
	s.fail = nil
}

func TestLostLockBlocksMutations(t *testing.T) {
	r, s, l, _ := setupRepo(t)
	ctx := context.Background()
	first := create(t, r, map[string]string{"notes/a": "old"}, time.Now())
	foreign := l.lock
	foreign.Token = strings.Repeat("c", 32)
	if err := r.writeLock(ctx, foreign); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Prune(ctx, Retention{Last: 1}, time.Now(), time.UTC); !errors.Is(err, ErrLostLock) {
		t.Fatal(err)
	}
	if _, err := r.CreateSnapshot(ctx, input(t, nil, time.Now())); !errors.Is(err, ErrLostLock) {
		t.Fatal(err)
	}
	if _, err := r.ReadSnapshot(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); !errors.Is(err, ErrLostLock) {
		t.Fatal(err)
	}
	if _, err := s.Stat(ctx, "lock.json"); err != nil {
		t.Fatal("removed foreign lock", err)
	}
}

func TestSnapshotAcknowledgementLostAndPublishFailure(t *testing.T) {
	r, s, _, _ := setupRepo(t)
	at := time.Now()
	ctx := context.Background()
	s.fail = func(method, key string) error {
		if method == "put" && strings.HasPrefix(key, "snapshots/") {
			return errors.New("publish failed")
		}
		return nil
	}
	if _, err := r.CreateSnapshot(ctx, input(t, map[string]string{"notes/a": "new"}, at)); err == nil {
		t.Fatal("publish succeeded")
	}
	s.fail = nil
	if all, err := r.ListSnapshots(ctx); err != nil || len(all) != 0 {
		t.Fatalf("published %+v %v", all, err)
	}
	if result, err := r.Prune(ctx, Retention{Last: 1}, at, time.UTC); err != nil || result.Blocks == 0 {
		t.Fatalf("orphan prune %+v %v", result, err)
	}
}

func TestEmptyExactAndRepeatedChunks(t *testing.T) {
	r, s, _, _ := setupRepo(t)
	ctx := context.Background()
	in := input(t, map[string]string{"notes/empty": ""}, time.Now())
	content := bytes.Repeat([]byte{42}, 2*ChunkSize)
	in.Database = bytes.NewReader(content)
	in.DatabaseSize = int64(len(content))
	snap, err := r.CreateSnapshot(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Database.Blocks) != 2 || snap.Database.Blocks[0].Hash != snap.Database.Blocks[1].Hash || len(snap.Files[0].Blocks) != 0 {
		t.Fatal("wrong chunk layout")
	}
	if s.puts[blobPath(snap.Database.Blocks[0].Hash)] != 1 {
		t.Fatal("duplicated block upload")
	}
	var db bytes.Buffer
	if _, err := r.Restore(ctx, snap.ID, &db, files.Local{Root: t.TempDir()}, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(db.Bytes(), content) {
		t.Fatal("restore mismatch")
	}
}

func TestRestoreDetectsManifestAndFileTampering(t *testing.T) {
	r, s, _, _ := setupRepo(t)
	ctx := context.Background()
	snap := create(t, r, map[string]string{"notes/a": "hello"}, time.Now())
	key := snapshotPath(snap.ID)
	raw := bytes.Clone(s.objects[key])
	raw[len(raw)/2] ^= 1
	s.objects[key] = raw
	if _, err := r.Restore(ctx, snap.ID, io.Discard, files.Local{Root: t.TempDir()}, nil); err == nil {
		t.Fatal("tampered manifest accepted")
	}
}

func TestReadOnlyRepositoryCannotMutate(t *testing.T) {
	_, s, _, _ := setupRepo(t)
	ctx := context.Background()
	r, err := Open(ctx, s, master)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateSnapshot(ctx, input(t, nil, time.Now())); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
	if _, err := r.Prune(ctx, Retention{Last: 1}, time.Now(), time.UTC); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
}
