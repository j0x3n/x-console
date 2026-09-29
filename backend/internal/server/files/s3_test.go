package files_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/files/fakes3"
)

func newS3(t *testing.T, prefix string) (*files.S3, *fakes3.Server) {
	t.Helper()
	fake := fakes3.New(t, "bucket")
	s, err := files.NewS3(files.S3Config{Endpoint: fake.URL, Region: "us-east-1", Bucket: "bucket", Prefix: prefix,
		AccessKeyID: "key", SecretAccessKey: "secret", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	return s, fake
}

func TestS3(t *testing.T) {
	s, fake := newS3(t, "site/x")
	runStoreTests(t, s)
	// Objects live under the prefix.
	for key := range fake.Objects() {
		if !strings.HasPrefix(key, "site/x/") {
			t.Errorf("object outside the prefix: %s", key)
		}
	}
}

func TestS3WithoutPrefix(t *testing.T) {
	s, _ := newS3(t, "")
	runStoreTests(t, s)
}

func TestS3Check(t *testing.T) {
	s, fake := newS3(t, "p")
	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(fake.Objects()) != 0 {
		t.Fatalf("check left files behind: %v", fake.Objects())
	}
	missing, err := files.NewS3(files.S3Config{Endpoint: fake.URL, Region: "us-east-1", Bucket: "nope", AccessKeyID: "k", SecretAccessKey: "s", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := missing.Check(context.Background()); err == nil || err.Error() != "桶不存在" {
		t.Fatalf("missing bucket: %v", err)
	}
	for _, bad := range []files.S3Config{
		{Endpoint: "ftp://x", Bucket: "b"},
		{Endpoint: "http://x/path", Bucket: "b"},
		{Endpoint: "http://x", Bucket: ""},
		{Endpoint: "http://x", Bucket: "a/b"},
		{Endpoint: "http://x", Bucket: "b", Prefix: "../x"},
	} {
		if _, err := files.NewS3(bad); err == nil {
			t.Errorf("config %+v should be rejected", bad)
		}
	}
}

func TestCached(t *testing.T) {
	remote, _ := newS3(t, "c")
	cache, err := files.NewCached(remote, t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	runStoreTests(t, cache)
}

func TestCachedReadsFromDirectory(t *testing.T) {
	ctx := context.Background()
	remote, fake := newS3(t, "")
	dir := t.TempDir()
	cache, err := files.NewCached(remote, dir, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	// A file written before the cache existed comes from the remote once.
	fake.Set("drive/old", []byte("old content"))
	for i := 0; i < 3; i++ {
		rc, _, err := cache.Get(ctx, "drive/old")
		if got := read(t, rc, err); got != "old content" {
			t.Fatalf("read %d: %q", i, got)
		}
	}
	if got := fake.GetCount(); got != 1 {
		t.Fatalf("expected one download, got %d", got)
	}
	// Files written through the cache are already there.
	put(t, cache, "notes/new", "new content")
	gets := fake.GetCount()
	rc, _, err := cache.Get(ctx, "notes/new")
	if got := read(t, rc, err); got != "new content" || fake.GetCount() != gets {
		t.Fatalf("written file should be cached: %q, gets %d -> %d", got, gets, fake.GetCount())
	}
	// A range of a cached file does not ask the remote either.
	rc, _, err = cache.GetRange(ctx, "notes/new", 4, 7)
	if got := read(t, rc, err); got != "content" || fake.GetCount() != gets {
		t.Fatalf("range: %q", got)
	}
	// A range of an uncached file is read remotely and not cached.
	fake.Set("drive/other", []byte("0123456789"))
	rc, _, err = cache.GetRange(ctx, "drive/other", 2, 3)
	if got := read(t, rc, err); got != "234" {
		t.Fatalf("remote range: %q", got)
	}
	if n, _, _ := cache.Usage(ctx); n != 2 {
		t.Fatalf("a range must not fill the cache, files: %d", n)
	}
	// Deleting removes both copies.
	if err := cache.Delete(ctx, "notes/new"); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Stat(ctx, "notes/new"); !errors.Is(err, files.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestCachedRemovesLeastRecentlyRead(t *testing.T) {
	ctx := context.Background()
	remote, fake := newS3(t, "")
	cache, err := files.NewCached(remote, t.TempDir(), 30)
	if err != nil {
		t.Fatal(err)
	}
	ten := strings.Repeat("x", 10)
	put(t, cache, "a/1", ten)
	time.Sleep(20 * time.Millisecond)
	put(t, cache, "a/2", ten)
	time.Sleep(20 * time.Millisecond)
	put(t, cache, "a/3", ten)
	time.Sleep(20 * time.Millisecond)
	// Reading a/1 makes it the newest, so a/2 goes first.
	rc, _, err := cache.Get(ctx, "a/1")
	read(t, rc, err)
	time.Sleep(20 * time.Millisecond)
	put(t, cache, "a/4", ten)

	n, size, err := cache.Usage(ctx)
	if err != nil || n != 3 || size != 30 {
		t.Fatalf("usage: %d files, %d bytes, %v", n, size, err)
	}
	gets := fake.GetCount()
	rc, _, err = cache.Get(ctx, "a/2") // was removed: comes from the remote again
	if got := read(t, rc, err); got != ten || fake.GetCount() != gets+1 {
		t.Fatalf("a/2 should be fetched again: gets %d -> %d", gets, fake.GetCount())
	}
	// Nothing is lost: the remote still has everything.
	if got := len(fake.Objects()); got != 4 {
		t.Fatalf("remote objects: %d", got)
	}
	// Lowering the limit trims at once.
	if err := cache.SetLimit(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if n, size, _ := cache.Usage(ctx); n != 1 || size != 10 {
		t.Fatalf("after SetLimit: %d files, %d bytes", n, size)
	}
}

func TestCachedWithZeroLimitKeepsNothing(t *testing.T) {
	ctx := context.Background()
	remote, _ := newS3(t, "")
	cache, err := files.NewCached(remote, t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	put(t, cache, "a/1", "hello")
	rc, _, err := cache.Get(ctx, "a/1")
	if got := read(t, rc, err); got != "hello" {
		t.Fatalf("read with no cache: %q", got)
	}
	if n, _, _ := cache.Usage(ctx); n != 0 {
		t.Fatalf("cache should stay empty, has %d files", n)
	}
}
