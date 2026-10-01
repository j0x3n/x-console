package coding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// B47 builds: the steps of a repository run in a task's worktree, one after
// the other, through the system shell.

const (
	defaultStepTimeout = 30 * time.Minute
	maxStepTimeout     = 6 * time.Hour
	maxBuildSteps      = 20
)

func stepTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultStepTimeout
	}
	return min(time.Duration(seconds)*time.Second, maxStepTimeout)
}

// shellCommand runs a command line with the system shell.
func shellCommand(line string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", "/C", line)
	}
	return exec.Command("/bin/sh", "-c", line)
}

// Build runs the steps and reports through emit. The last event is always
// a done event with CodingBuildDone.
func (s *Service) Build(ctx context.Context, p protocol.CodingBuildParams, emit func(protocol.CodingEvent), cancel <-chan struct{}) {
	finish := func(d protocol.CodingBuildDone) {
		emit(event(protocol.CodingEventDone, d.Reason, d))
	}
	_, wt, err := checkTask(ctx, p.CodingTaskParams)
	if err == nil {
		if st, serr := os.Stat(wt); serr != nil || !st.IsDir() {
			err = &protocol.Error{Code: protocol.CodeNotFound, Message: "the task has no worktree; build before committing"}
		}
	}
	if err == nil && (len(p.Steps) == 0 || len(p.Steps) > maxBuildSteps) {
		err = rpcutil.BadParams("1 to %d steps", maxBuildSteps)
	}
	if err != nil {
		emit(event(protocol.CodingEventError, errText(err), nil))
		finish(protocol.CodingBuildDone{Reason: protocol.CodingBuildError, FailedStep: -1, Error: errText(err)})
		return
	}
	for i, step := range p.Steps {
		if strings.TrimSpace(step.Command) == "" {
			finish(protocol.CodingBuildDone{Reason: protocol.CodingBuildError, FailedStep: i, Error: "empty command"})
			return
		}
		emit(event(protocol.CodingEventStatus, step.Name, map[string]any{"code": "build_step", "index": i, "name": step.Name, "command": step.Command}))
		start := time.Now()
		code, reason := s.runStep(ctx, wt, p.TaskID, i, step, emit, cancel)
		emit(event(protocol.CodingEventStatus, step.Name, map[string]any{"code": "build_step_done", "index": i,
			"exitCode": code, "durationMs": time.Since(start).Milliseconds()}))
		if reason != "" {
			finish(protocol.CodingBuildDone{Reason: reason, FailedStep: i})
			return
		}
		if code != 0 {
			finish(protocol.CodingBuildDone{Reason: protocol.CodingBuildFailed, FailedStep: i})
			return
		}
	}
	var patterns []string
	for _, step := range p.Steps {
		patterns = append(patterns, step.Artifacts...)
	}
	artifacts, err := collectArtifacts(wt, patterns)
	d := protocol.CodingBuildDone{Reason: protocol.CodingBuildPassed, FailedStep: -1, Artifacts: artifacts}
	if err != nil {
		d.Error = errText(err)
	}
	finish(d)
}

// runStep runs one step. reason is empty when the command exited by itself.
func (s *Service) runStep(ctx context.Context, wt string, taskID int64, index int, step protocol.CodingBuildStep, emit func(protocol.CodingEvent), cancel <-chan struct{}) (int, string) {
	cmd := shellCommand(step.Command)
	cmd.Dir = wt
	cmd.Env = append(os.Environ(), "X_CONSOLE_TASK_ID="+strconv.FormatInt(taskID, 10), "CI=1")
	lines := func(stream string) *lineWriter {
		return &lineWriter{fn: func(line []byte) {
			text := strings.TrimRight(string(bytes.ToValidUTF8(line, []byte("?"))), "\r\n")
			emit(event(protocol.CodingEventText, text, map[string]any{"stream": stream, "index": index}))
		}}
	}
	stdout, stderr := lines("stdout"), lines("stderr")
	outR, outW, err := os.Pipe()
	if err != nil {
		emit(event(protocol.CodingEventError, err.Error(), nil))
		return -1, protocol.CodingBuildError
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		outR.Close()
		outW.Close()
		emit(event(protocol.CodingEventError, err.Error(), nil))
		return -1, protocol.CodingBuildError
	}
	cmd.Stdout, cmd.Stderr = outW, errW
	group := prepareGroup(cmd)
	err = cmd.Start()
	outW.Close()
	errW.Close()
	if err != nil {
		outR.Close()
		errR.Close()
		emit(event(protocol.CodingEventError, "start: "+err.Error(), nil))
		return -1, protocol.CodingBuildError
	}
	if err := group.started(); err != nil {
		slog.Warn("coding: build process group setup failed", "err", err)
	}
	defer group.close()
	var readers sync.WaitGroup
	for _, pair := range []struct {
		r *os.File
		w *lineWriter
	}{{outR, stdout}, {errR, stderr}} {
		readers.Add(1)
		go func() {
			defer readers.Done()
			_, _ = io.Copy(pair.w, pair.r)
		}()
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	timer := time.NewTimer(stepTimeout(step.TimeoutSeconds))
	defer timer.Stop()
	reason := ""
	var waitErr error
	select {
	case waitErr = <-waitCh:
	case <-timer.C:
		reason = protocol.CodingBuildTimeout
	case <-cancel:
		reason = protocol.CodingBuildCanceled
	case <-ctx.Done():
		reason = protocol.CodingBuildCanceled
	}
	if reason != "" {
		group.interrupt()
		select {
		case waitErr = <-waitCh:
		case <-time.After(KillGrace):
			group.kill()
			waitErr = <-waitCh
		}
	}
	group.kill()
	drained := make(chan struct{})
	go func() {
		readers.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		outR.Close()
		errR.Close()
		<-drained
	}
	outR.Close()
	errR.Close()
	stdout.flush()
	stderr.flush()
	return exitCode(cmd, waitErr), reason
}

var errTooManyArtifacts = errors.New("more than 50 artifacts; the rest were left out")

// collectArtifacts finds the files the patterns match inside wt.
func collectArtifacts(wt string, patterns []string) ([]protocol.CodingArtifact, error) {
	seen := map[string]bool{}
	var out []protocol.CodingArtifact
	var skipped error
	add := func(abs string) error {
		rel, err := filepath.Rel(wt, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
		name := filepath.ToSlash(rel)
		if seen[name] || strings.HasPrefix(name, ".git/") || name == ".git" {
			return nil
		}
		st, err := os.Stat(abs)
		if err != nil || !st.Mode().IsRegular() {
			return nil
		}
		// A symlink may point outside the worktree.
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			if rw, err := filepath.EvalSymlinks(wt); err == nil {
				if r, err := filepath.Rel(rw, real); err != nil || strings.HasPrefix(r, "..") {
					return nil
				}
			}
		}
		if st.Size() > protocol.CodingArtifactMaxSize {
			skipped = errors.New(name + " is larger than 2 GB and was left out")
			return nil
		}
		if len(out) == protocol.CodingArtifactMaxFiles {
			skipped = errTooManyArtifacts
			return fs.SkipAll
		}
		seen[name] = true
		out = append(out, protocol.CodingArtifact{Path: abs, Name: name, Size: st.Size()})
		return nil
	}
	for _, raw := range patterns {
		pattern := path.Clean(strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/"))
		if pattern == "" || pattern == "." || strings.HasPrefix(pattern, "/") || strings.HasPrefix(pattern, "../") || pattern == ".." {
			continue
		}
		if dir, ok := strings.CutSuffix(pattern, "/**"); ok {
			root := filepath.Join(wt, filepath.FromSlash(dir))
			err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				return add(p)
			})
			if errors.Is(err, fs.SkipAll) {
				break
			}
			continue
		}
		matches, err := filepath.Glob(filepath.Join(wt, filepath.FromSlash(pattern)))
		if err != nil {
			continue
		}
		stop := false
		for _, m := range matches {
			if add(m) == fs.SkipAll {
				stop = true
				break
			}
		}
		if stop {
			break
		}
	}
	return out, skipped
}

// serveBuild adapts Build to a stream, like serveRun.
func (s *Service) serveBuild(ctx context.Context, raw json.RawMessage, st *rpc.Stream) error {
	var p protocol.CodingBuildParams
	if err := rpcutil.Decode(raw, &p); err != nil {
		return err
	}
	cancel := make(chan struct{})
	var once sync.Once
	go func() {
		for {
			chunk, err := st.Recv(ctx)
			if err != nil {
				return
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
			slog.Debug("coding build event not sent", "err", err)
		}
	}
	s.Build(ctx, p, emit, cancel)
	return nil
}
