package files

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

func client(t *testing.T) *rpc.Peer {
	return clientWithWrite(t, ServeWrite)
}

func clientWithWrite(t *testing.T, write rpc.StreamHandler) *rpc.Peer {
	a, b := rpc.Pipe()
	agent := rpc.NewPeer(a, "a")
	agent.HandleStream(protocol.MethodFilesRead, ServeRead)
	agent.HandleStream(protocol.MethodFilesWrite, write)
	c := rpc.NewPeer(b, "s")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go agent.Run(ctx)
	go c.Run(ctx)
	return c
}

func code(err error) string {
	var pe *protocol.Error
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func TestListMkdirRenameRemove(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Mkdir(filepath.Join(dir, "Adir")); err != nil {
		t.Fatal(err)
	}
	if _, err := Mkdir(filepath.Join(dir, "Adir")); code(err) != protocol.CodeExists {
		t.Fatalf("mkdir twice: %v", err)
	}
	list, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if list.Path != dir || list.Parent != filepath.Dir(dir) || len(list.Entries) != 2 {
		t.Fatalf("list: %+v", list)
	}
	if list.Entries[0].Name != "Adir" || list.Entries[0].Type != "dir" || list.Entries[1].Size != 5 || list.Entries[1].Type != "file" {
		t.Fatalf("entries: %+v", list.Entries)
	}
	if _, err := List("relative/path"); code(err) != protocol.CodeBadParams {
		t.Fatalf("relative: %v", err)
	}
	if _, err := List(filepath.Join(dir, "missing")); code(err) != protocol.CodeNotFound {
		t.Fatalf("missing: %v", err)
	}
	if home, err := List(""); err != nil || home.Path == "" {
		t.Fatalf("home: %+v %v", home, err)
	}

	moved, err := Rename(filepath.Join(dir, "b.txt"), filepath.Join(dir, "Adir", "c.txt"))
	if err != nil || moved.Name != "c.txt" {
		t.Fatalf("rename: %+v %v", moved, err)
	}
	if _, err := Rename(filepath.Join(dir, "Adir"), filepath.Join(dir, "Adir", "c.txt")); code(err) != protocol.CodeExists {
		t.Fatalf("rename onto existing: %v", err)
	}
	if err := Remove(filepath.Join(dir, "Adir"), false); err == nil {
		t.Fatal("removed non-empty dir without recursive")
	}
	if err := Remove(filepath.Join(dir, "Adir"), true); err != nil {
		t.Fatal(err)
	}
	if err := Remove("/", true); code(err) != protocol.CodeBadParams {
		t.Fatalf("root: %v", err)
	}
}

func TestDriveEntries(t *testing.T) {
	e := DriveEntries(1<<2 | 1<<3) // C and D
	if len(e) != 2 || e[0].Path != `C:\` || e[1].Name != `D:\` || e[0].Type != "dir" {
		t.Fatalf("%+v", e)
	}
}

func TestUploadAndDownload(t *testing.T) {
	c := client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dir := t.TempDir()
	path := filepath.Join(dir, "blob.bin")
	data := make([]byte, 3*protocol.FileChunkSize+123)
	_, _ = rand.Read(data)

	// Upload in uneven chunks.
	s, err := c.Open(ctx, protocol.MethodFilesWrite, protocol.FilesWriteParams{Path: path, Size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}
	for off := 0; off < len(data); off += 50000 {
		if err := s.Send(ctx, data[off:min(off+50000, len(data))]); err != nil {
			t.Fatal(err)
		}
	}
	ack, err := s.Recv(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var e protocol.FileEntry
	if err := json.Unmarshal(ack, &e); err != nil || e.Size != int64(len(data)) {
		t.Fatalf("ack %s: %v", ack, err)
	}
	if _, err := s.Recv(ctx); !errors.Is(err, io.EOF) {
		t.Fatalf("after ack: %v", err)
	}

	// Download it again.
	s, err = c.Open(ctx, protocol.MethodFilesRead, protocol.FilesReadParams{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	head, err := s.Recv(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var h protocol.FileHeader
	if err := json.Unmarshal(head, &h); err != nil || h.Size != int64(len(data)) || h.Name != "blob.bin" {
		t.Fatalf("header %s", head)
	}
	var got bytes.Buffer
	for {
		chunk, err := s.Recv(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(chunk) > protocol.FileChunkSize {
			t.Fatalf("chunk of %d bytes", len(chunk))
		}
		got.Write(chunk)
	}
	if !bytes.Equal(got.Bytes(), data) {
		t.Fatal("downloaded content differs")
	}

	// A download of a missing file fails with not_found before any data.
	s, _ = c.Open(ctx, protocol.MethodFilesRead, protocol.FilesReadParams{Path: filepath.Join(dir, "nope")})
	if _, err := s.Recv(ctx); code(err) != protocol.CodeNotFound {
		t.Fatalf("missing download: %v", err)
	}
}

func TestReadRange(t *testing.T) {
	c := client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "large.log")
	data := bytes.Repeat([]byte("abc123\n"), 450000)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ offset, length int64 }{{int64(len(data)) - 1<<20, 1 << 20}, {17, 1234}, {int64(len(data)) + 10, 100}} {
		s, err := c.Open(ctx, protocol.MethodFilesRead, protocol.FilesReadParams{Path: path, Offset: tc.offset, Length: tc.length})
		if err != nil {
			t.Fatal(err)
		}
		head, err := s.Recv(ctx)
		var h protocol.FileHeader
		if err != nil || json.Unmarshal(head, &h) != nil || h.Size != int64(len(data)) {
			t.Fatalf("header: %s %v", head, err)
		}
		var got bytes.Buffer
		for {
			chunk, err := s.Recv(ctx)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			got.Write(chunk)
		}
		start := min(tc.offset, int64(len(data)))
		end := min(start+tc.length, int64(len(data)))
		if !bytes.Equal(got.Bytes(), data[start:end]) {
			t.Fatalf("range %d:%d differs", tc.offset, tc.length)
		}
	}
	s, _ := c.Open(ctx, protocol.MethodFilesRead, protocol.FilesReadParams{Path: path, Offset: -1, Length: 4})
	if _, err := s.Recv(ctx); code(err) != protocol.CodeBadParams {
		t.Fatalf("negative offset: %v", err)
	}
}

func TestUploadAbortLeavesNothing(t *testing.T) {
	for _, delayed := range []bool{false, true} {
		name := "partial upload"
		if delayed {
			name = "abort before handler starts"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			start := make(chan struct{})
			finished := make(chan error, 1)
			c := clientWithWrite(t, func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
				<-start
				err := ServeWrite(ctx, raw, s)
				finished <- err
				return err
			})
			if !delayed {
				close(start)
			}
			s, err := c.Open(ctx, protocol.MethodFilesWrite, protocol.FilesWriteParams{Path: filepath.Join(dir, "x"), Size: 1000})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Send(ctx, make([]byte, 10)); err != nil {
				t.Fatal(err)
			}
			s.Close(nil)
			if delayed {
				close(start)
			}
			select {
			case err := <-finished:
				if code(err) != protocol.CodeFailed {
					t.Fatalf("aborted upload: %v", err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("left behind: %v", entries)
			}
		})
	}
}
