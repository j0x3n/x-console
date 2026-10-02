package repo

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type lostAcknowledgement struct{ *memoryStore }

func (s lostAcknowledgement) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if err := s.memoryStore.Put(ctx, key, r, size); err != nil {
		return err
	}
	if strings.HasPrefix(key, "snapshots/") {
		return errors.New("response lost")
	}
	return nil
}

func TestCommittedSnapshotSurvivesLostAcknowledgement(t *testing.T) {
	s := lostAcknowledgement{memory()}
	r, l, err := Acquire(context.Background(), s, master, t.TempDir(), "backup", true)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	snap, err := r.CreateSnapshot(l.Context(), input(t, map[string]string{"notes/a": "hello"}, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	all, err := r.ListSnapshots(l.Context())
	if err != nil || len(all) != 1 || all[0].ID != snap.ID {
		t.Fatalf("%+v %v", all, err)
	}
}
