// Package coding runs coding tasks (M4): it detects the Claude Code and
// Codex command line tools, finds git repositories, runs an executor in a
// git worktree per task and reports its output as normalized events, and
// then shows, commits, pushes or discards the changes.
//
// The package keeps no task state between calls. A task's worktree lives at
// <repo>/.x-console/worktrees/<taskId> on the branch the server chose (it
// must start with "xc/").
package coding

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os/exec"
	"runtime"
	"slices"
	"sync"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// Service serves the coding.* methods with one configuration.
type Service struct {
	cfg Config
}

// New builds a Service.
func New(cfg Config) *Service { return &Service{cfg: cfg} }

var lookPath = exec.LookPath

// Available reports whether this machine should announce the coding
// capability before the config is read: always on Windows (the desktop
// agent, where the capability is expected even before an executor is
// installed, so the UI can say what is missing), otherwise when claude or
// codex is on PATH. Register adds the capability later when only a
// configured executor path exists.
func Available() bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return New(Config{}).anyExecutor()
}

func (s *Service) anyExecutor() bool {
	for _, name := range Executors {
		if _, err := s.cfg.resolve(name); err == nil {
			return true
		}
	}
	return false
}

// Register adds the coding.* methods. raw is the "coding" object of the
// agent config. When an executor is found and the hello does not announce
// the coding capability yet, it is added.
func Register(c *conn.Client, raw json.RawMessage) {
	cfg, err := ParseConfig(raw)
	if err != nil {
		slog.Warn("invalid coding config, using defaults", "err", err)
	}
	s := New(cfg)
	if !slices.Contains(c.Hello.Capabilities, protocol.CapCoding) && s.anyExecutor() {
		c.Hello.Capabilities = append(c.Hello.Capabilities, protocol.CapCoding)
	}
	s.Register(c)
}

// Register adds the handlers of s to c.
func (s *Service) Register(c *conn.Client) {
	c.Handle(protocol.MethodCodingExecutors, func(ctx context.Context, _ json.RawMessage) (any, error) {
		return s.Executors(ctx), nil
	})
	c.Handle(protocol.MethodCodingRepos, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.CodingReposParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return s.Repos(ctx, p)
	})
	c.HandleStream(protocol.MethodCodingRun, s.serveRun)
	c.Handle(protocol.MethodCodingDiff, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.CodingTaskParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return s.Diff(ctx, p)
	})
	c.Handle(protocol.MethodCodingCommit, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.CodingCommitParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return s.Commit(ctx, p)
	})
	c.Handle(protocol.MethodCodingPush, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.CodingPushParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return nil, s.Push(ctx, p)
	})
	c.Handle(protocol.MethodCodingDiscard, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.CodingTaskParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return nil, s.Discard(ctx, p)
	})
}

// serveRun adapts Run to a stream: events go out as JSON lines, a
// CodingControl{Op: "cancel"} chunk cancels the run, and the end of the
// stream (server gone) interrupts it like a cancel.
func (s *Service) serveRun(ctx context.Context, raw json.RawMessage, st *rpc.Stream) error {
	var p protocol.CodingRunParams
	if err := rpcutil.Decode(raw, &p); err != nil {
		return err
	}
	cancel := make(chan struct{})
	var once sync.Once
	go func() {
		for {
			chunk, err := st.Recv(ctx)
			if err != nil {
				return // ctx ends too, Run sees that
			}
			var c protocol.CodingControl
			if json.Unmarshal(chunk, &c) == nil && c.Op == "cancel" {
				once.Do(func() { close(cancel) })
			}
		}
	}()
	var mu sync.Mutex
	emit := func(ev protocol.CodingEvent) {
		line, err := json.Marshal(ev)
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if err := st.Send(ctx, append(line, '\n')); err != nil && !errors.Is(err, context.Canceled) {
			slog.Debug("coding event not sent", "err", err)
		}
	}
	s.Run(ctx, p, emit, cancel)
	return nil
}
