// Package selfupdate serves agent.update (B233): the panel sends a new agent
// program, the agent checks it, runs it once, swaps it in for the running
// program and restarts. Nothing is replaced unless every check passes.
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/nowindow"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

const (
	// maxSize bounds the program a panel may send. Agents are about 15 MB.
	maxSize = 256 << 20
	// transferTimeout is how long receiving may take in all.
	transferTimeout = 10 * time.Minute
	trialTimeout    = 15 * time.Second
	// oldSuffix is added to the name of the program that was replaced.
	oldSuffix = ".old"
)

// Updater does the update. The zero value does the real thing; tests replace
// the steps that touch the running process.
type Updater struct {
	// Exe returns the path of the running program with symlinks resolved.
	Exe func() (string, error)
	// Trial runs `<path> version` and returns what it printed.
	Trial func(ctx context.Context, path string) (string, error)
	// Rename moves a file. Both names are in the program's directory.
	Rename func(from, to string) error
	// Restart starts the new program in place of this process. On success it
	// does not return (Linux replaces the process, Windows exits it); tests
	// return nil.
	Restart func(exe string) error
}

// Default is the Updater of the real agent.
var Default = &Updater{}

// Handle serves protocol.MethodAgentUpdate with the default Updater.
func Handle(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
	return Default.Handle(ctx, raw, s)
}

// Disabled answers protocol.MethodAgentUpdate on an agent whose config turned
// updating off.
func Disabled(context.Context, json.RawMessage, *rpc.Stream) error {
	return &protocol.Error{Code: protocol.CodeUnsupported, Message: "这台机器关掉了更新"}
}

// Register wires agent.update into the client. Updating is on unless
// disabled; the capability is only announced when it is on, and a server that
// tries anyway is refused.
func Register(c *conn.Client, disabled bool) {
	if disabled {
		c.HandleStream(protocol.MethodAgentUpdate, Disabled)
		return
	}
	for _, v := range c.Hello.Capabilities {
		if v == protocol.CapUpdate {
			c.HandleStream(protocol.MethodAgentUpdate, Handle)
			return
		}
	}
	c.Hello.Capabilities = append(c.Hello.Capabilities, protocol.CapUpdate)
	c.HandleStream(protocol.MethodAgentUpdate, Handle)
}

func fail(format string, a ...any) error {
	return &protocol.Error{Code: protocol.CodeFailed, Message: fmt.Sprintf(format, a...)}
}

func (u *Updater) exe() (string, error) {
	if u.Exe != nil {
		return u.Exe()
	}
	return Executable()
}

func (u *Updater) rename(from, to string) error {
	if u.Rename != nil {
		return u.Rename(from, to)
	}
	return os.Rename(from, to)
}

// Executable is the path of the running program, symlinks resolved so the
// real file is the one that gets replaced.
func Executable() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

// Handle receives the program in s and installs it.
func (u *Updater) Handle(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
	var in protocol.AgentUpdateParams
	if err := json.Unmarshal(raw, &in); err != nil {
		return &protocol.Error{Code: protocol.CodeBadParams, Message: err.Error()}
	}
	want, err := hex.DecodeString(in.SHA256)
	if err != nil || len(want) != sha256.Size || in.Size <= 0 || in.Size > maxSize ||
		in.Version == "" || len(in.Version) > 64 || strings.TrimSpace(in.Version) != in.Version || strings.ContainsAny(in.Version, "\r\n") {
		return &protocol.Error{Code: protocol.CodeBadParams, Message: "更新参数不对"}
	}
	exe, err := u.exe()
	if err != nil {
		return fail("找不到程序文件：%v", err)
	}
	ctx, cancel := context.WithTimeout(ctx, transferTimeout)
	defer cancel()

	tmp, err := u.receive(ctx, s, exe, in, want)
	if err != nil {
		return err
	}
	// From here the temporary file is removed unless it was renamed.
	installed := false
	defer func() {
		if !installed {
			_ = os.Remove(tmp)
		}
	}()

	if err := u.trial(ctx, tmp, in.Version); err != nil {
		return err
	}
	if err := u.swap(exe, tmp); err != nil {
		return err
	}
	installed = true
	slog.Info("agent program replaced, restarting", "version", in.Version, "exe", exe)
	// The panel waits for this frame. The restart ends the connection.
	// If the panel is gone the program is still swapped, so restart anyway.
	_ = s.Send(ctx, nil)
	restart := u.Restart
	if restart == nil {
		restart = restartProcess
	}
	if err := restart(exe); err != nil {
		slog.Error("restart after update failed", "err", err)
		return fail("换好了新程序，但重启失败：%v", err)
	}
	return nil
}

// receive writes the stream to a temporary file next to the program and
// checks size and digest. The file is removed on any error.
func (u *Updater) receive(ctx context.Context, s *rpc.Stream, exe string, in protocol.AgentUpdateParams, want []byte) (string, error) {
	base := filepath.Base(exe)
	ext := filepath.Ext(base)
	f, err := os.CreateTemp(filepath.Dir(exe), strings.TrimSuffix(base, ext)+".new-*"+ext)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return "", fail("没有权限写程序文件")
		}
		return "", fail("写不了程序文件：%v", err)
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()
	h := sha256.New()
	var got int64
	for got < in.Size {
		b, err := s.Recv(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", fail("传输中断了，只收到 %d 字节", got)
			}
			return "", err
		}
		if len(b) == 0 {
			continue
		}
		got += int64(len(b))
		if got > in.Size {
			return "", fail("收到的程序比说好的大")
		}
		h.Write(b)
		if _, err := f.Write(b); err != nil {
			return "", fail("写程序文件失败：%v", err)
		}
	}
	if err := f.Sync(); err != nil {
		return "", fail("写程序文件失败：%v", err)
	}
	if err := f.Close(); err != nil {
		return "", fail("写程序文件失败：%v", err)
	}
	if hex.EncodeToString(h.Sum(nil)) != hex.EncodeToString(want) {
		return "", fail("校验不对")
	}
	mode := os.FileMode(0o755)
	if st, err := os.Stat(exe); err == nil {
		mode = st.Mode().Perm() | 0o111
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return "", fail("设置程序权限失败：%v", err)
	}
	ok = true
	return tmp, nil
}

// trial runs the new program once and checks the version it reports.
func (u *Updater) trial(ctx context.Context, path, version string) error {
	run := u.Trial
	if run == nil {
		run = runVersion
	}
	out, err := run(ctx, path)
	if err != nil {
		slog.Warn("trial run of the new agent failed", "err", err)
		return fail("新程序跑不起来")
	}
	if got := strings.TrimSpace(out); got != version {
		slog.Warn("new agent reports another version", "want", version, "got", got)
		return fail("新程序的版本不对，应该是 %s，跑出来是 %s", version, shorten(got))
	}
	return nil
}

func shorten(s string) string {
	if r := []rune(s); len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return s
}

// runVersion runs `<path> version`. A program that cannot be started at all
// is tried a few times: right after writing a file Linux can answer "text
// file busy" for a moment.
func runVersion(ctx context.Context, path string) (string, error) {
	var lastErr error
	for i := 0; i < 5; i++ {
		cctx, cancel := context.WithTimeout(ctx, trialTimeout)
		cmd := exec.CommandContext(cctx, path, "version")
		nowindow.Hide(cmd)
		out, err := cmd.Output()
		cancel()
		if err == nil {
			return string(out), nil
		}
		lastErr = err
		var ee *exec.ExitError
		if errors.As(err, &ee) || ctx.Err() != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", lastErr
}

// swap moves the running program to <name>.old and puts tmp in its place. A
// running program cannot be overwritten but can be renamed, also on Windows.
func (u *Updater) swap(exe, tmp string) error {
	old := exe + oldSuffix
	_ = os.Remove(old)
	if err := u.rename(exe, old); err != nil {
		return swapError(err)
	}
	if err := u.rename(tmp, exe); err != nil {
		// Put the old program back so the machine is not left without one.
		if rerr := u.rename(old, exe); rerr != nil {
			slog.Error("could not restore the agent program", "err", rerr)
		}
		return swapError(err)
	}
	return nil
}

func swapError(err error) error {
	if errors.Is(err, os.ErrPermission) {
		return fail("没有权限换掉程序文件")
	}
	return fail("换程序文件失败：%v", err)
}

// CleanOld removes <program>.old left by an update. On Windows the old
// process may still be running for a moment and its file cannot be deleted
// yet, so it tries again every wait, up to tries times. It returns whether
// the file is gone.
func CleanOld(tries int, wait time.Duration) bool {
	exe, err := Executable()
	if err != nil {
		return false
	}
	return cleanOld(exe+oldSuffix, tries, wait)
}

func cleanOld(old string, tries int, wait time.Duration) bool {
	for i := 0; i < tries; i++ {
		if _, err := os.Lstat(old); errors.Is(err, os.ErrNotExist) {
			return true
		}
		if err := os.Remove(old); err == nil {
			return true
		}
		if i < tries-1 {
			time.Sleep(wait)
		}
	}
	return false
}

// CleanOldInBackground is CleanOld for the agent's start: it does not hold
// up the start.
func CleanOldInBackground() {
	go CleanOld(30, 2*time.Second)
}
