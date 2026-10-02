package repo

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

type inconsistentListing struct {
	*memoryStore
	omitSnapshot string
	omitAfter    int
	calls        int
	failBlobs    bool
}

func (s *inconsistentListing) List(ctx context.Context, prefix string) iter.Seq2[files.Info, error] {
	return func(yield func(files.Info, error) bool) {
		if prefix == "snapshots" {
			s.calls++
		}
		for info, err := range s.memoryStore.List(ctx, prefix) {
			if prefix == "snapshots" && s.calls >= s.omitAfter && info.Key == s.omitSnapshot {
				continue
			}
			if !yield(info, err) {
				return
			}
		}
		if prefix == "blobs" && s.failBlobs {
			yield(files.Info{}, errors.New("listing interrupted"))
		}
	}
}

func TestPruneDoesNotDeleteBlobsAfterIncompleteKeptSnapshotListing(t *testing.T) {
	ctx := context.Background()
	s := &inconsistentListing{memoryStore: memory(), omitAfter: 1 << 20}
	r, lease, err := Acquire(ctx, s, master, t.TempDir(), "backup", true)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	at := time.Now()
	create(t, r, map[string]string{"notes/a": "old"}, at)
	last := create(t, r, map[string]string{"notes/a": "new"}, at.Add(time.Hour))
	s.omitSnapshot, s.omitAfter = snapshotPath(last.ID), s.calls+2
	before := blobCount(s.memoryStore)
	if _, err := r.Prune(ctx, Retention{Last: 1}, at.Add(2*time.Hour), time.UTC); err == nil {
		t.Fatal("prune accepted incomplete listing")
	}
	if got := blobCount(s.memoryStore); got != before {
		t.Fatal("deleted blobs after incomplete snapshot listing", got, before)
	}
}

func TestPruneDoesNotDeleteBlobsAfterPartialBlobListingFailure(t *testing.T) {
	ctx := context.Background()
	s := &inconsistentListing{memoryStore: memory(), omitAfter: 1 << 20}
	r, lease, err := Acquire(ctx, s, master, t.TempDir(), "backup", true)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	at := time.Now()
	create(t, r, map[string]string{"notes/a": "old"}, at)
	create(t, r, map[string]string{"notes/a": "new"}, at.Add(time.Hour))
	s.failBlobs = true
	before := blobCount(s.memoryStore)
	if _, err := r.Prune(ctx, Retention{Last: 1}, at.Add(2*time.Hour), time.UTC); err == nil {
		t.Fatal("prune accepted partial blob list")
	}
	if got := blobCount(s.memoryStore); got != before {
		t.Fatal("deleted blobs before full listing succeeded", got, before)
	}
}

func blobCount(s *memoryStore) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int
	for key := range s.objects {
		if strings.HasPrefix(key, "blobs/") {
			count++
		}
	}
	return count
}
