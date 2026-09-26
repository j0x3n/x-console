//go:build linux

package pty

import (
	"bytes"
	"context"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// pipePeers connects a client peer to an agent peer serving pty.open.
func pipePeers(t *testing.T) *rpc.Peer {
	a, b := rpc.Pipe()
	agent := rpc.NewPeer(a, "a")
	agent.HandleStream(protocol.MethodPTYOpen, Serve)
	client := rpc.NewPeer(b, "s")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go agent.Run(ctx)
	go client.Run(ctx)
	return client
}

// readUntil collects terminal output until it contains want.
func readUntil(t *testing.T, s *rpc.Stream, out *bytes.Buffer, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for !strings.Contains(out.String(), want) {
		chunk, err := s.Recv(ctx)
		if err != nil {
			t.Fatalf("waiting for %q: %v; output so far: %q", want, err, out.String())
		}
		if len(chunk) == 0 || chunk[0] != protocol.PTYFrameData {
			t.Fatalf("unexpected frame %v", chunk)
		}
		out.Write(chunk[1:])
	}
}

func TestShellSession(t *testing.T) {
	client := pipePeers(t)
	ctx := context.Background()
	s, err := client.Open(ctx, protocol.MethodPTYOpen, protocol.PTYOpenParams{Shell: "/bin/sh", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := s.Send(ctx, DataFrame([]byte("echo hello-$((40+2))\n"))); err != nil {
		t.Fatal(err)
	}
	readUntil(t, s, &out, "hello-42")

	if err := s.Send(ctx, ResizeFrame(100, 40)); err != nil {
		t.Fatal(err)
	}
	_ = s.Send(ctx, DataFrame([]byte("stty size; echo P\"\"ID:$$:\n")))
	readUntil(t, s, &out, "40 100")
	pidRe := regexp.MustCompile(`PID:(\d+):`)
	for !pidRe.MatchString(out.String()) {
		readUntil(t, s, &out, "PID:")
		readUntil(t, s, &out, "\n")
	}
	pid, _ := strconv.Atoi(pidRe.FindStringSubmatch(out.String())[1])

	// Closing the stream (the browser left) must end the shell.
	s.Close(nil)
	deadline := time.Now().Add(5 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("shell %d still running after the stream closed", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestShellExitEndsStream(t *testing.T) {
	client := pipePeers(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := client.Open(ctx, protocol.MethodPTYOpen, protocol.PTYOpenParams{Shell: "/bin/sh"})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Send(ctx, DataFrame([]byte("exit\n")))
	for {
		if _, err := s.Recv(ctx); err != nil {
			if ctx.Err() != nil {
				t.Fatal("stream did not end after exit")
			}
			return
		}
	}
}

func TestBadShell(t *testing.T) {
	client := pipePeers(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := client.Open(ctx, protocol.MethodPTYOpen, protocol.PTYOpenParams{Shell: "/no/such/shell"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Recv(ctx)
	var pe *protocol.Error
	if err == nil || !asProto(err, &pe) || pe.Code != protocol.CodeBadParams {
		t.Fatalf("got %v", err)
	}
}

func asProto(err error, pe **protocol.Error) bool {
	p, ok := err.(*protocol.Error)
	*pe = p
	return ok
}
