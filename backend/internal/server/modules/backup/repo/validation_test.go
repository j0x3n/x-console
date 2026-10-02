package repo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

func TestConflictingAndOversizedManifestMetadata(t *testing.T) {
	h := sha256.Sum256(nil)
	empty := File{SHA256: hex.EncodeToString(h[:]), Blocks: []Block{}}
	s := Snapshot{Format: 1, ID: "20261002T030000Z-" + strings.Repeat("a", 32), CreatedAt: time.Now(), Migration: "1", Database: File{Size: 1, SHA256: strings.Repeat("a", 64), Blocks: []Block{{Hash: strings.Repeat("b", 64), Size: 1, StoredSize: 28}}}, SizeBytes: 1}
	for _, paths := range [][]string{{"notes/a", "notes/a/b"}, {"notes/a", "notes/A"}, {"backups/archive"}, {"notes/COM1.txt"}} {
		bad := s
		for _, path := range paths {
			f := empty
			f.Path = path
			bad.Files = append(bad.Files, f)
		}
		if validateSnapshot(bad) == nil {
			t.Fatal(paths)
		}
	}
	bad := empty
	bad.Size = 1<<63 - 1
	if validateFile(bad) == nil {
		t.Fatal("integer overflow accepted")
	}
}

func TestCompressionLimitAndPurposeSeparation(t *testing.T) {
	r, _, _, _ := setupRepo(t)
	plain := bytes.Repeat([]byte{0}, ChunkSize)
	sum := sha256.Sum256(plain)
	hash := hex.EncodeToString(sum[:])
	raw, err := r.encodeBlock(plain, hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.decodeBlock(raw, Block{Hash: hash, Size: 1, StoredSize: int64(len(raw))}); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	if _, err := unseal(r.snapshotKey, raw, r.aad("blob", hash, ChunkSize)); err == nil {
		t.Fatal("purpose not separated")
	}
	if _, err := r.decodeBlock(raw, Block{Hash: strings.Repeat("a", 64), Size: ChunkSize, StoredSize: int64(len(raw))}); err == nil {
		t.Fatal("hash not bound")
	}
}

func TestChangedInputAndMissingReferencedBlockFail(t *testing.T) {
	r, s, _, _ := setupRepo(t)
	ctx := context.Background()
	at := time.Now()
	first := create(t, r, map[string]string{"notes/a": "one"}, at)
	if err := s.Delete(ctx, blobPath(first.Database.Blocks[0].Hash)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateSnapshot(ctx, input(t, map[string]string{"notes/a": "one"}, at.Add(time.Hour))); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	in := input(t, nil, at.Add(2*time.Hour))
	in.DatabaseSize = 50
	if _, err := r.CreateSnapshot(ctx, in); err == nil {
		t.Fatal("short reader accepted")
	}
}

func TestInitNeverOverwritesUnrecognizedRepository(t *testing.T) {
	s := memory()
	ctx := context.Background()
	if err := s.Put(ctx, "unknown", strings.NewReader("x"), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(ctx, s, master, strings.Repeat("a", 32)); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	if _, err := s.Stat(ctx, "config.json"); !errors.Is(err, files.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestUnreferencedExistingBlockIsVerifiedBeforeReuse(t *testing.T) {
	r, s, _, _ := setupRepo(t)
	plain := []byte("sqlite database")
	sum := sha256.Sum256(plain)
	hash := hex.EncodeToString(sum[:])
	raw, err := r.encodeBlock(plain, hash)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 1
	if err := s.Put(context.Background(), blobPath(hash), bytes.NewReader(raw), int64(len(raw))); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateSnapshot(context.Background(), input(t, nil, time.Now())); !errors.Is(err, ErrCorrupt) {
		t.Fatal("corrupt orphan reused", err)
	}
	if all, err := r.ListSnapshots(context.Background()); err != nil || len(all) != 0 {
		t.Fatal(all, err)
	}
}
