package coding

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// BranchPrefix is required for every task branch, so discard can never
// delete a branch the user made.
const BranchPrefix = "xc/"

const gitTimeout = 2 * time.Minute

// git runs git in dir and returns its trimmed stdout.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := gitRaw(ctx, dir, 0, args...)
	return strings.TrimSpace(string(out)), err
}

// gitRaw runs git and returns stdout. limit > 0 caps the output; the
// returned error is errTruncated when the cap was hit.
func gitRaw(ctx context.Context, dir string, limit int, args ...string) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, gitTimeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	hideWindow(cmd)
	var stdout capBuffer
	stdout.max = limit
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.Bytes(), &gitError{args: args, msg: msg}
	}
	if stdout.truncated {
		return stdout.Bytes(), errTruncated
	}
	return stdout.Bytes(), nil
}

var errTruncated = errors.New("output truncated")

type gitError struct {
	args []string
	msg  string
}

func (e *gitError) Error() string {
	name := "git"
	if len(e.args) > 0 {
		name += " " + e.args[0]
	}
	return name + ": " + e.msg
}

type capBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (c *capBuffer) Write(p []byte) (int, error) {
	if c.max > 0 {
		room := c.max - c.buf.Len()
		if room < len(p) {
			c.truncated = true
			if room > 0 {
				c.buf.Write(p[:room])
			}
			return len(p), nil
		}
	}
	return c.buf.Write(p)
}

func (c *capBuffer) Bytes() []byte { return c.buf.Bytes() }

// repoRoot checks that path is the top level of a git repository and
// returns it cleaned.
func repoRoot(ctx context.Context, path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", rpcutil.BadParams("repoPath must be an absolute path")
	}
	path = filepath.Clean(path)
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		return "", &protocol.Error{Code: protocol.CodeNotFound, Message: "repository not found: " + path}
	}
	top, err := git(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", rpcutil.BadParams("not a git repository: %s", path)
	}
	if !samePath(top, path) {
		return "", rpcutil.BadParams("%s is inside the repository %s; register the top level", path, top)
	}
	return path, nil
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(filepath.FromSlash(a)), filepath.Clean(filepath.FromSlash(b))
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		a = ra
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	if os.PathSeparator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// WorktreePath is where the worktree of a task lives.
func WorktreePath(repo string, taskID int64) string {
	return filepath.Join(repo, ".x-console", "worktrees", strconv.FormatInt(taskID, 10))
}

// checkTask validates the parameters shared by every task method and
// returns the repository root and the worktree path.
func checkTask(ctx context.Context, p protocol.CodingTaskParams) (string, string, error) {
	if p.TaskID <= 0 {
		return "", "", rpcutil.BadParams("taskId is required")
	}
	repo, err := repoRoot(ctx, p.RepoPath)
	if err != nil {
		return "", "", err
	}
	if err := checkBranch(ctx, repo, p.Branch); err != nil {
		return "", "", err
	}
	return repo, WorktreePath(repo, p.TaskID), nil
}

func checkBranch(ctx context.Context, repo, branch string) error {
	if !strings.HasPrefix(branch, BranchPrefix) || len(branch) <= len(BranchPrefix) {
		return rpcutil.BadParams("branch must start with %q", BranchPrefix)
	}
	if _, err := git(ctx, repo, "check-ref-format", "--branch", branch); err != nil {
		return rpcutil.BadParams("invalid branch name %q", branch)
	}
	return nil
}

// excludeWorktrees adds /.x-console/ to .git/info/exclude so task worktrees
// never show up as untracked files in the main checkout.
func excludeWorktrees(ctx context.Context, repo string) error {
	common, err := git(ctx, repo, "rev-parse", "--git-common-dir")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(repo, common)
	}
	path := filepath.Join(common, "info", "exclude")
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "/.x-console/" {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	prefix := ""
	if len(raw) > 0 && !bytes.HasSuffix(raw, []byte("\n")) {
		prefix = "\n"
	}
	_, err = fmt.Fprintf(f, "%s# X Console coding task worktrees\n/.x-console/\n", prefix)
	return err
}

// branchExists reports whether refs/heads/branch exists in repo.
func branchExists(ctx context.Context, repo, branch string) bool {
	_, err := git(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// removeWorktree deletes the worktree directory and its registration.
// A missing worktree is not an error.
func removeWorktree(ctx context.Context, repo, wt string) error {
	var errs []error
	if _, err := os.Stat(wt); err == nil {
		if _, err := git(ctx, repo, "worktree", "remove", "--force", "--force", wt); err != nil {
			// Not registered (or locked by a dying process): remove by hand.
			if rmErr := os.RemoveAll(wt); rmErr != nil {
				errs = append(errs, err, rmErr)
			}
		}
	}
	if _, err := git(ctx, repo, "worktree", "prune"); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// deleteBranch removes the task branch. A missing branch is not an error.
func deleteBranch(ctx context.Context, repo, branch string) error {
	if !branchExists(ctx, repo, branch) {
		return nil
	}
	_, err := git(ctx, repo, "branch", "-D", branch)
	return err
}

// baseOf returns the commit the task branch started from.
func baseOf(ctx context.Context, repo string, p protocol.CodingTaskParams) (string, error) {
	if p.BaseCommit != "" {
		sha, err := git(ctx, repo, "rev-parse", "--verify", "--quiet", p.BaseCommit+"^{commit}")
		if err == nil {
			return sha, nil
		}
	}
	base := p.BaseBranch
	if base == "" {
		base = "HEAD"
	}
	sha, err := git(ctx, repo, "merge-base", base, p.Branch)
	if err != nil {
		return "", rpcutil.Failed("cannot find the base commit: %v", err)
	}
	return sha, nil
}

// identityArgs returns -c user.name/-c user.email when git has no identity
// configured, so a commit never fails on a fresh machine.
func identityArgs(ctx context.Context, dir string) []string {
	var args []string
	if v, err := git(ctx, dir, "config", "user.name"); err != nil || v == "" {
		args = append(args, "-c", "user.name=X Console")
	}
	if v, err := git(ctx, dir, "config", "user.email"); err != nil || v == "" {
		args = append(args, "-c", "user.email=x-console@localhost")
	}
	return args
}
