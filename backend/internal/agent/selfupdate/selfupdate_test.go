package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// fixture is a panel peer and an agent peer joined by a pipe. The agent
// serves agent.update with an Updater that works in a temporary directory.
type fixture struct {
	t        *testing.T
	ctx      context.Context
	panel    *rpc.Peer
	exe      string
	restarts atomic.Int32
	u        *Updater
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the new programs in these tests are shell scripts")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	dir := t.TempDir()
	exe := filepath.Join(dir, "x-console-agent")
	if err := os.WriteFile(exe, []byte("OLD PROGRAM"), 0o751); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, ctx: ctx, exe: exe}
	f.u = &Updater{Exe: func() (string, error) { return exe, nil }, Restart: func(string) error { f.restarts.Add(1); return nil }}
	a, b := rpc.Pipe()
	f.panel = rpc.NewPeer(a, "s")
	agent := rpc.NewPeer(b, "a")
	agent.HandleStream(protocol.MethodAgentUpdate, func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
		return f.u.Handle(ctx, raw, s)
	})
	go f.panel.Run(ctx)
	go agent.Run(ctx)
	return f
}

func script(version string) []byte {
	return []byte("#!/bin/sh\necho " + version + "\n")
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// push sends program the way the panel does (32 KB frames) and returns the
// error the agent ends the stream with, or nil when it sent the ready frame.
func (f *fixture) push(p protocol.AgentUpdateParams, program []byte) error {
	f.t.Helper()
	s, err := f.panel.Open(f.ctx, protocol.MethodAgentUpdate, p)
	if err != nil {
		f.t.Fatal(err)
	}
	defer s.Close(nil)
	for off := 0; off < len(program) && s.Context().Err() == nil; off += 32 << 10 {
		end := min(off+32<<10, len(program))
		if err := s.Send(f.ctx, append([]byte(nil), program[off:end]...)); err != nil {
			break
		}
	}
	b, err := s.Recv(f.ctx)
	if err != nil {
		return err
	}
	if len(b) != 0 {
		f.t.Fatalf("ready frame has %d bytes", len(b))
	}
	return nil
}

func (f *fixture) read(path string) string {
	f.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

// noTempFiles checks that nothing but the program (and .old) is left.
func (f *fixture) noTempFiles() {
	f.t.Helper()
	entries, _ := os.ReadDir(filepath.Dir(f.exe))
	for _, e := range entries {
		if e.Name() != "x-console-agent" && e.Name() != "x-console-agent.old" {
			f.t.Fatalf("left over file %s", e.Name())
		}
	}
}

func wantMessage(t *testing.T, err error, part string) {
	t.Helper()
	var pe *protocol.Error
	if !errors.As(err, &pe) || !strings.Contains(pe.Message, part) {
		t.Fatalf("want error with %q, got %v", part, err)
	}
}

func TestUpdateReplacesAndRestarts(t *testing.T) {
	f := newFixture(t)
	prog := append(script("v2"), []byte(strings.Repeat("# padding\n", 10000))...) // several frames
	if err := f.push(protocol.AgentUpdateParams{Version: "v2", Size: int64(len(prog)), SHA256: digest(prog)}, prog); err != nil {
		t.Fatal(err)
	}
	if f.read(f.exe) != string(prog) {
		t.Fatal("the new program is not in place")
	}
	if f.read(f.exe+".old") != "OLD PROGRAM" {
		t.Fatal("the old program was not kept as .old")
	}
	if st, _ := os.Stat(f.exe); st.Mode().Perm() != 0o751 {
		t.Fatalf("mode %v, want the old program's", st.Mode().Perm())
	}
	f.noTempFiles()
	deadline := time.Now().Add(2 * time.Second)
	for f.restarts.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if f.restarts.Load() != 1 {
		t.Fatalf("restarts %d", f.restarts.Load())
	}
}

func TestUpdateRefusals(t *testing.T) {
	prog := script("v2")
	good := protocol.AgentUpdateParams{Version: "v2", Size: int64(len(prog)), SHA256: digest(prog)}
	cases := []struct {
		name    string
		params  protocol.AgentUpdateParams
		program []byte
		want    string
	}{
		{"wrong digest", protocol.AgentUpdateParams{Version: "v2", Size: good.Size, SHA256: digest([]byte("other"))}, prog, "校验不对"},
		{"wrong version", protocol.AgentUpdateParams{Version: "v3", Size: good.Size, SHA256: good.SHA256}, prog, "版本不对"},
		{"not a program", protocol.AgentUpdateParams{Version: "v2", Size: 4, SHA256: digest([]byte("nope"))}, []byte("nope"), "跑不起来"},
		{"more than announced", protocol.AgentUpdateParams{Version: "v2", Size: good.Size - 1, SHA256: good.SHA256}, prog, "比说好的大"},
		{"bad digest text", protocol.AgentUpdateParams{Version: "v2", Size: good.Size, SHA256: "zz"}, prog, "参数不对"},
		{"no size", protocol.AgentUpdateParams{Version: "v2", SHA256: good.SHA256}, prog, "参数不对"},
		{"no version", protocol.AgentUpdateParams{Size: good.Size, SHA256: good.SHA256}, prog, "参数不对"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			wantMessage(t, f.push(c.params, c.program), c.want)
			if f.read(f.exe) != "OLD PROGRAM" {
				t.Fatal("the program was replaced")
			}
			f.noTempFiles()
			if f.restarts.Load() != 0 {
				t.Fatal("restarted")
			}
		})
	}
}

func TestUpdateStreamEndsEarly(t *testing.T) {
	f := newFixture(t)
	prog := script("v2")
	s, err := f.panel.Open(f.ctx, protocol.MethodAgentUpdate, protocol.AgentUpdateParams{Version: "v2", Size: int64(len(prog)) + 100, SHA256: digest(prog)})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Send(f.ctx, prog)
	s.Close(nil)
	time.Sleep(200 * time.Millisecond)
	if f.read(f.exe) != "OLD PROGRAM" {
		t.Fatal("the program was replaced")
	}
	f.noTempFiles()
}

func TestUpdateCannotWriteNextToProgram(t *testing.T) {
	f := newFixture(t)
	f.u.Exe = func() (string, error) { return filepath.Join(t.TempDir(), "missing", "x-console-agent"), nil }
	prog := script("v2")
	wantMessage(t, f.push(protocol.AgentUpdateParams{Version: "v2", Size: int64(len(prog)), SHA256: digest(prog)}, prog), "写不了程序文件")
}

func TestUpdateRestoresOldProgramWhenSwapFails(t *testing.T) {
	f := newFixture(t)
	calls := 0
	f.u.Rename = func(from, to string) error {
		calls++
		if calls == 2 { // moving the new program into place
			return os.ErrPermission
		}
		return os.Rename(from, to)
	}
	prog := script("v2")
	wantMessage(t, f.push(protocol.AgentUpdateParams{Version: "v2", Size: int64(len(prog)), SHA256: digest(prog)}, prog), "没有权限换掉程序文件")
	if f.read(f.exe) != "OLD PROGRAM" {
		t.Fatal("the old program was not put back")
	}
	f.noTempFiles()
	if f.restarts.Load() != 0 {
		t.Fatal("restarted")
	}
}

func TestUpdateReportsFailedRestart(t *testing.T) {
	f := newFixture(t)
	f.u.Restart = func(string) error { return errors.New("exec failed") }
	prog := script("v2")
	s, err := f.panel.Open(f.ctx, protocol.MethodAgentUpdate, protocol.AgentUpdateParams{Version: "v2", Size: int64(len(prog)), SHA256: digest(prog)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(nil)
	_ = s.Send(f.ctx, prog)
	if _, err := s.Recv(f.ctx); err != nil { // ready
		t.Fatal(err)
	}
	_, err = s.Recv(f.ctx)
	wantMessage(t, err, "重启失败")
}

func TestDisabledAgentRefusesAndDoesNotAnnounce(t *testing.T) {
	c := conn.New("http://127.0.0.1:1", "x", protocol.Hello{})
	Register(c, true)
	for _, v := range c.Hello.Capabilities {
		if v == protocol.CapUpdate {
			t.Fatal("announced update although disabled")
		}
	}
	c = conn.New("http://127.0.0.1:1", "x", protocol.Hello{})
	Register(c, false)
	Register(c, false)
	n := 0
	for _, v := range c.Hello.Capabilities {
		if v == protocol.CapUpdate {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("update announced %d times", n)
	}

	a, b := rpc.Pipe()
	panel, agent := rpc.NewPeer(a, "s"), rpc.NewPeer(b, "a")
	agent.HandleStream(protocol.MethodAgentUpdate, Disabled)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go panel.Run(ctx)
	go agent.Run(ctx)
	s, err := panel.Open(ctx, protocol.MethodAgentUpdate, protocol.AgentUpdateParams{Version: "v2", Size: 1, SHA256: digest([]byte("x"))})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Recv(ctx)
	var pe *protocol.Error
	if !errors.As(err, &pe) || pe.Code != protocol.CodeUnsupported || !strings.Contains(pe.Message, "关掉了更新") {
		t.Fatalf("got %v", err)
	}
}

func TestCleanOld(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "x-console-agent.old")
	if !cleanOld(old, 1, 0) {
		t.Fatal("nothing to remove counts as clean")
	}
	if err := os.WriteFile(old, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !cleanOld(old, 3, time.Millisecond) {
		t.Fatal("not removed")
	}
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("still there")
	}
}

func TestExecutableResolvesSymlinks(t *testing.T) {
	got, err := Executable()
	if err != nil {
		t.Fatal(err)
	}
	if real, _ := filepath.EvalSymlinks(got); real != got {
		t.Fatalf("%s is not resolved", got)
	}
}
