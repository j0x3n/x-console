package coding

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// KillGrace is how long an interrupted executor gets before it is killed.
// Tests shorten it.
var KillGrace = 10 * time.Second

// maxLine caps one output line of the executor.
const maxLine = 4 << 20

// Run creates the task worktree, runs the executor in it and reports its
// output through emit until the executor exits, the timeout passes, cancel
// is closed or ctx ends. The last event is always CodingEventDone. When the
// executor was stopped (timeout, cancel, ctx) the worktree and branch are
// removed; when it exited by itself they are kept for review.
func (s *Service) Run(ctx context.Context, p protocol.CodingRunParams, emit func(protocol.CodingEvent), cancel <-chan struct{}) {
	done := func(code int, d protocol.CodingDone) {
		ev := event(protocol.CodingEventDone, d.Reason, d)
		ev.ExitCode = &code
		emit(ev)
	}
	fail := func(err error) {
		emit(event(protocol.CodingEventError, errText(err), nil))
		done(-1, protocol.CodingDone{Reason: protocol.CodingEndStartFailed, Error: errText(err)})
	}
	tp := protocol.CodingTaskParams{TaskID: p.TaskID, RepoPath: p.RepoPath, Branch: p.Branch, BaseBranch: p.BaseBranch}
	repo, wt, err := checkTask(ctx, tp)
	if err != nil {
		fail(err)
		return
	}
	if !validExecutor(p.Executor) {
		fail(rpcutil.BadParams("unknown executor %q", p.Executor))
		return
	}
	if strings.TrimSpace(p.Prompt) == "" {
		fail(rpcutil.BadParams("prompt is required"))
		return
	}
	if p.Permission != "" && p.Permission != protocol.CodingPermissionWorkspace && p.Permission != protocol.CodingPermissionFull {
		fail(rpcutil.BadParams("unknown permission %q", p.Permission))
		return
	}
	prompt := p.Prompt
	if p.AllowQuestions {
		prompt += "\n\n如果需要用户回答，请在工作目录写 .xc-question.md 然后停止本次执行。内容用 JSON：{\"title\":\"问题\",\"detail\":\"背景\",\"options\":[\"选项\"]}。不要自行猜测答案。"
	}
	spec, err := s.cfg.commandWith(p.Executor, prompt, p.Model, p.Permission)
	if err != nil {
		fail(fmt.Errorf("%s is not installed or not on PATH: %v", p.Executor, err))
		return
	}
	var base string
	if p.Continue {
		// B47: run again in the kept worktree, to fix a failed build.
		base, err = continueWorktree(ctx, wt, p.Branch, p.BaseCommit)
	} else {
		base, err = createWorktree(ctx, repo, wt, p.Branch, p.BaseBranch, p.PreferRemote)
	}
	if err != nil {
		fail(err)
		return
	}
	emit(event(protocol.CodingEventStatus, "worktree ready", map[string]any{
		"code": "worktree_ready", "worktree": wt, "branch": p.Branch, "baseCommit": base, "executor": spec.path,
	}))
	cleanup := func() {
		cctx, cc := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cc()
		if err := removeWorktree(cctx, repo, wt); err != nil {
			slog.Warn("coding: remove worktree", "path", wt, "err", err)
		}
		if err := deleteBranch(cctx, repo, p.Branch); err != nil {
			slog.Warn("coding: delete branch", "branch", p.Branch, "err", err)
		}
	}

	cmd := exec.Command(spec.path, spec.args...)
	cmd.Dir = wt
	cmd.Env = append(os.Environ(), spec.env...)
	cmd.Env = append(cmd.Env, "X_CONSOLE_TASK_ID="+strconv.FormatInt(p.TaskID, 10))
	if spec.stdin != "" {
		cmd.Stdin = strings.NewReader(spec.stdin)
	}
	parse := newParser(spec.format)
	stdout := &lineWriter{fn: func(line []byte) {
		for _, ev := range parse.line(line) {
			emit(ev)
		}
	}}
	stderr := &lineWriter{fn: func(line []byte) {
		if text := strings.TrimRight(string(bytes.ToValidUTF8(line, []byte("?"))), "\r\n"); strings.TrimSpace(text) != "" {
			emit(event(protocol.CodingEventText, text, map[string]any{"stream": "stderr"}))
		}
	}}
	// Our own pipes, so Wait returns when the executor exits even if
	// something it started still holds the output open.
	outR, outW, err := os.Pipe()
	if err != nil {
		cleanup()
		fail(err)
		return
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		outR.Close()
		outW.Close()
		cleanup()
		fail(err)
		return
	}
	cmd.Stdout, cmd.Stderr = outW, errW
	group := prepareGroup(cmd)
	err = cmd.Start()
	outW.Close()
	errW.Close()
	if err != nil {
		outR.Close()
		errR.Close()
		cleanup()
		fail(fmt.Errorf("start %s: %v", p.Executor, err))
		return
	}
	if err := group.started(); err != nil {
		slog.Warn("coding: process group setup failed", "err", err)
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
	emit(event(protocol.CodingEventStatus, "executor started", map[string]any{"code": "started", "pid": cmd.Process.Pid}))

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	timer := time.NewTimer(Timeout(p.TimeoutSeconds))
	defer timer.Stop()
	reason := protocol.CodingEndExited
	var waitErr error
	select {
	case waitErr = <-waitCh:
	case <-timer.C:
		reason = protocol.CodingEndTimeout
	case <-cancel:
		reason = protocol.CodingEndCanceled
	case <-ctx.Done():
		reason = protocol.CodingEndCanceled
	}
	if reason != protocol.CodingEndExited {
		group.interrupt()
		select {
		case waitErr = <-waitCh:
		case <-time.After(KillGrace):
			group.kill()
			waitErr = <-waitCh
		}
	}
	group.kill() // anything the executor left running
	// The pipes reach EOF once every process holding them is gone. A process
	// that escaped the group could hold them forever: stop reading then.
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
	code := exitCode(cmd, waitErr)

	if reason != protocol.CodingEndExited {
		cleanup()
		msg := "canceled"
		if reason == protocol.CodingEndTimeout {
			msg = "timed out after " + Timeout(p.TimeoutSeconds).String()
		}
		emit(event(protocol.CodingEventStatus, msg, map[string]any{"code": reason}))
		done(-1, protocol.CodingDone{Reason: reason, BaseCommit: base})
		return
	}
	question, qerr := readQuestion(wt)
	if qerr != nil {
		done(-1, protocol.CodingDone{Reason: protocol.CodingEndExited, BaseCommit: base, Error: errText(qerr)})
		return
	}
	// Keep the private question out of the staged changes, including when
	// the executor staged everything before asking.
	if question != nil {
		_, _ = git(context.WithoutCancel(ctx), wt, "reset", "--quiet", "--", ".xc-question.md")
	}
	files, err := changedFiles(context.WithoutCancel(ctx), wt, "--cached", base)
	d := protocol.CodingDone{Reason: reason, BaseCommit: base, Files: files}
	d.Question = question
	if err != nil {
		d.Error = errText(err)
	}
	done(code, d)
}

func errText(err error) string {
	var pe *protocol.Error
	if errors.As(err, &pe) {
		return pe.Message
	}
	return err.Error()
}

func exitCode(cmd *exec.Cmd, err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.Exited() {
		return ee.ExitCode()
	}
	return -1
}

// createWorktree adds the task worktree on a new branch and returns the
// base commit.
func createWorktree(ctx context.Context, repo, wt, branch, baseBranch string, preferRemote bool) (string, error) {
	if err := excludeWorktrees(ctx, repo); err != nil {
		return "", fmt.Errorf("update .git/info/exclude: %w", err)
	}
	var base string
	if baseBranch == "" {
		sha, err := git(ctx, repo, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
		if err != nil {
			return "", errors.New("the repository has no commits yet")
		}
		base = sha
	} else {
		refs := []string{baseBranch, "origin/" + baseBranch}
		if preferRemote {
			refs[0], refs[1] = refs[1], refs[0]
		}
		for _, ref := range refs {
			if sha, err := git(ctx, repo, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err == nil {
				base = sha
				break
			}
		}
		if base == "" {
			return "", fmt.Errorf("base branch %q not found", baseBranch)
		}
	}
	if _, err := os.Stat(wt); err == nil {
		return "", fmt.Errorf("worktree %s already exists", wt)
	}
	if branchExists(ctx, repo, branch) {
		return "", fmt.Errorf("branch %s already exists", branch)
	}
	if err := os.MkdirAll(filepath.Dir(wt), 0o755); err != nil {
		return "", err
	}
	if _, err := git(ctx, repo, "worktree", "add", "--quiet", "-b", branch, wt, base); err != nil {
		return "", err
	}
	return base, nil
}

// continueWorktree checks the kept worktree of a task and returns its base
// commit.
func continueWorktree(ctx context.Context, wt, branch, baseCommit string) (string, error) {
	if st, err := os.Stat(wt); err != nil || !st.IsDir() {
		return "", fmt.Errorf("worktree %s is gone", wt)
	}
	if cur, err := git(ctx, wt, "rev-parse", "--abbrev-ref", "HEAD"); err != nil || cur != branch {
		return "", fmt.Errorf("worktree %s is not on %s", wt, branch)
	}
	if baseCommit == "" {
		return "", errors.New("baseCommit is required to continue")
	}
	sha, err := git(ctx, wt, "rev-parse", "--verify", "--quiet", baseCommit+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("base commit %s not found", baseCommit)
	}
	return sha, nil
}

// changedFiles lists changes. In a worktree it stages everything first and
// compares the index ("--cached", base); for a branch it compares two
// commits (base, branch).
func changedFiles(ctx context.Context, dir string, from, to string) ([]protocol.CodingChangedFile, error) {
	if from == "--cached" {
		if _, err := git(ctx, dir, "add", "-A"); err != nil {
			return nil, err
		}
	}
	numstat, err := gitRaw(ctx, dir, 0, "diff", "--numstat", "-z", "--no-renames", from, to)
	if err != nil {
		return nil, err
	}
	status, err := gitRaw(ctx, dir, 0, "diff", "--name-status", "-z", "--no-renames", from, to)
	if err != nil {
		return nil, err
	}
	return parseChanges(numstat, status), nil
}

// parseChanges merges `git diff --numstat -z` and `--name-status -z`.
func parseChanges(numstat, status []byte) []protocol.CodingChangedFile {
	byPath := map[string]*protocol.CodingChangedFile{}
	var order []string
	get := func(path string) *protocol.CodingChangedFile {
		if f, ok := byPath[path]; ok {
			return f
		}
		f := &protocol.CodingChangedFile{Path: filepath.ToSlash(path), Status: "M"}
		byPath[path] = f
		order = append(order, path)
		return f
	}
	parts := strings.Split(string(status), "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		if parts[i] == "" {
			break
		}
		get(parts[i+1]).Status = parts[i][:1]
	}
	for _, rec := range strings.Split(string(numstat), "\x00") {
		fields := strings.SplitN(rec, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		f := get(fields[2])
		if fields[0] == "-" {
			f.Binary = true
			continue
		}
		f.Additions, _ = strconv.Atoi(fields[0])
		f.Deletions, _ = strconv.Atoi(fields[1])
	}
	out := make([]protocol.CodingChangedFile, 0, len(order))
	for _, p := range order {
		out = append(out, *byPath[p])
	}
	return out
}

// Diff returns the task's changes: the worktree while it exists, the
// branch after a commit.
func (s *Service) Diff(ctx context.Context, p protocol.CodingTaskParams) (protocol.CodingDiff, error) {
	out := protocol.CodingDiff{Files: []protocol.CodingChangedFile{}}
	repo, wt, err := checkTask(ctx, p)
	if err != nil {
		return out, err
	}
	base, err := baseOf(ctx, repo, p)
	if err != nil {
		return out, err
	}
	dir, from, to := wt, "--cached", base
	if _, err := os.Stat(wt); err != nil {
		if !branchExists(ctx, repo, p.Branch) {
			return out, &protocol.Error{Code: protocol.CodeNotFound, Message: "the task worktree and branch no longer exist"}
		}
		dir, from, to = repo, base, p.Branch
	}
	files, err := changedFiles(ctx, dir, from, to)
	if err != nil {
		return out, rpcutil.Failed("%v", err)
	}
	out.Files = files
	raw, err := gitRaw(ctx, dir, protocol.CodingDiffMax, "diff", "--no-renames", "--no-color", "--no-ext-diff", from, to)
	if errors.Is(err, errTruncated) {
		out.Truncated = true
	} else if err != nil {
		return out, rpcutil.Failed("%v", err)
	}
	out.Diff = string(bytes.ToValidUTF8(raw, []byte("?")))
	return out, nil
}

// Commit commits everything in the worktree, then removes the worktree and
// keeps the branch. Commits the executor made itself count as changes.
func (s *Service) Commit(ctx context.Context, p protocol.CodingCommitParams) (protocol.CodingCommitResult, error) {
	var out protocol.CodingCommitResult
	repo, wt, err := checkTask(ctx, p.CodingTaskParams)
	if err != nil {
		return out, err
	}
	msg := strings.TrimSpace(p.Message)
	if msg == "" {
		return out, rpcutil.BadParams("message is required")
	}
	if _, err := os.Stat(wt); err != nil {
		return out, &protocol.Error{Code: protocol.CodeNotFound, Message: "the task worktree no longer exists"}
	}
	if _, err := git(ctx, wt, "add", "-A"); err != nil {
		return out, rpcutil.Failed("%v", err)
	}
	if _, err := git(ctx, wt, "diff", "--cached", "--quiet"); err != nil {
		var ge *gitError
		if !errors.As(err, &ge) {
			return out, rpcutil.Failed("%v", err)
		}
		// Exit status 1: there are staged changes.
		args := append(identityArgs(ctx, wt), "commit", "--quiet", "--no-verify", "-m", msg)
		if _, err := git(ctx, wt, args...); err != nil {
			return out, rpcutil.Failed("%v", err)
		}
	} else {
		base, err := baseOf(ctx, repo, p.CodingTaskParams)
		if err != nil {
			return out, err
		}
		if n, _ := git(ctx, wt, "rev-list", "--count", base+"..HEAD"); n == "0" || n == "" {
			return out, rpcutil.BadParams("there are no changes to commit")
		}
	}
	sha, err := git(ctx, wt, "rev-parse", "HEAD")
	if err != nil {
		return out, rpcutil.Failed("%v", err)
	}
	if err := removeWorktree(ctx, repo, wt); err != nil {
		slog.Warn("coding: remove worktree after commit", "path", wt, "err", err)
	}
	out.SHA = sha
	return out, nil
}

// Push pushes the task branch and sets its upstream.
func (s *Service) Push(ctx context.Context, p protocol.CodingPushParams) error {
	repo, _, err := checkTask(ctx, p.CodingTaskParams)
	if err != nil {
		return err
	}
	if !branchExists(ctx, repo, p.Branch) {
		return &protocol.Error{Code: protocol.CodeNotFound, Message: "branch " + p.Branch + " no longer exists"}
	}
	remote := p.Remote
	if remote == "" {
		remote = "origin"
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	ref := "refs/heads/" + p.Branch
	// No "+" in the refspec and no --force: a push never rewrites history.
	if _, err := gitEnv(pctx, repo, authEnv(p.Auth), "push", "--set-upstream", "--porcelain", remote, ref+":"+ref); err != nil {
		return rpcutil.Failed("%s", redact(errText(err), p.Auth))
	}
	return nil
}

// Discard removes the worktree and deletes the branch. Missing ones are
// fine, so discarding twice succeeds.
func (s *Service) Discard(ctx context.Context, p protocol.CodingTaskParams) error {
	repo, wt, err := checkTask(ctx, p)
	if err != nil {
		return err
	}
	if err := removeWorktree(ctx, repo, wt); err != nil {
		return rpcutil.Failed("%v", err)
	}
	if err := deleteBranch(ctx, repo, p.Branch); err != nil {
		return rpcutil.Failed("%v", err)
	}
	return nil
}

// lineWriter splits written bytes into lines.
type lineWriter struct {
	mu   sync.Mutex
	buf  []byte
	skip bool // inside an overlong line
	fn   func(line []byte)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			w.add(p)
			break
		}
		w.add(p[:i])
		if !w.skip || len(w.buf) > 0 {
			w.fn(w.buf)
		}
		w.buf, w.skip = w.buf[:0], false
		p = p[i+1:]
	}
	return n, nil
}

func (w *lineWriter) add(p []byte) {
	if w.skip {
		return
	}
	if len(w.buf)+len(p) > maxLine {
		// Hand out what fits as plain text and drop the rest of the line.
		w.buf = append(w.buf, p[:maxLine-len(w.buf)]...)
		w.fn(w.buf)
		w.buf, w.skip = w.buf[:0], true
		return
	}
	w.buf = append(w.buf, p...)
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.fn(w.buf)
	}
	w.buf = nil
}
