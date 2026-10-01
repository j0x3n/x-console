package coding

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/agent/coding/gittest"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const testToken = "tok-SECRET-42"

func TestEnsureRepoCloneFetchPush(t *testing.T) {
	needGit(t)
	work, origin := newRepo(t)
	run(t, origin, "config", "http.receivepack", "true")
	run(t, origin, "symbolic-ref", "HEAD", "refs/heads/main")
	srv := gittest.New(t, filepath.Dir(origin), "alice", testToken)
	cloneURL := srv.URL + "/origin.git"
	s := New(Config{ReposDir: t.TempDir()})
	auth := &protocol.CodingGitAuth{Username: "alice", Token: testToken}
	ctx := context.Background()

	// Wrong token: the error never shows it.
	_, err := s.EnsureRepo(ctx, protocol.CodingEnsureRepoParams{Dir: "1/team/demo", CloneURL: cloneURL, Auth: &protocol.CodingGitAuth{Username: "alice", Token: "wrong-" + testToken}})
	if err == nil || strings.Contains(err.Error(), testToken) {
		t.Fatalf("bad token: %v", err)
	}

	repo, err := s.EnsureRepo(ctx, protocol.CodingEnsureRepoParams{Dir: "1/team/demo", CloneURL: cloneURL, Auth: auth})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(s.cfg.ReposDir, "1", "team", "demo")
	if repo.Path != want || repo.DefaultBranch != "main" || repo.RemoteURL != cloneURL {
		t.Fatalf("clone: %+v", repo)
	}
	assertNoToken(t, repo.Path)

	// A new commit upstream arrives with the next call (fetch).
	run(t, work, "commit", "--quiet", "--allow-empty", "-m", "second")
	run(t, work, "push", "--quiet", "origin", "main")
	upstream := run(t, work, "rev-parse", "HEAD")
	if _, err := s.EnsureRepo(ctx, protocol.CodingEnsureRepoParams{Dir: "1/team/demo", CloneURL: cloneURL, Auth: auth}); err != nil {
		t.Fatal(err)
	}
	if got := run(t, repo.Path, "rev-parse", "origin/main"); got != upstream {
		t.Fatalf("fetch: origin/main %s, want %s", got, upstream)
	}
	// The local main is stale; PreferRemote starts from origin/main.
	wt := WorktreePath(repo.Path, 7)
	base, err := createWorktree(ctx, repo.Path, wt, "xc/7-remote", "main", true)
	if err != nil || base != upstream {
		t.Fatalf("worktree base %s, want %s: %v", base, upstream, err)
	}

	// Push the task branch with the token.
	if err := os.WriteFile(filepath.Join(wt, "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tp := protocol.CodingTaskParams{TaskID: 7, RepoPath: repo.Path, Branch: "xc/7-remote", BaseBranch: "main", BaseCommit: base}
	if _, err := s.Commit(ctx, protocol.CodingCommitParams{CodingTaskParams: tp, Message: "add new.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Push(ctx, protocol.CodingPushParams{CodingTaskParams: tp}); err == nil {
		t.Fatal("push without token worked")
	}
	if err := s.Push(ctx, protocol.CodingPushParams{CodingTaskParams: tp, Auth: auth}); err != nil {
		t.Fatal(err)
	}
	if out := run(t, origin, "branch", "--list", "xc/7-remote"); !strings.Contains(out, "xc/7-remote") {
		t.Fatalf("branch not pushed: %q", out)
	}
	assertNoToken(t, repo.Path)
	// Never the default branch.
	tp.Branch = "main"
	if err := s.Push(ctx, protocol.CodingPushParams{CodingTaskParams: tp, Auth: auth}); err == nil {
		t.Fatal("pushed main")
	}
}

// assertNoToken checks that the token is nowhere in the clone's git files.
func assertNoToken(t *testing.T, repo string) {
	t.Helper()
	err := filepath.Walk(filepath.Join(repo, ".git"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Size() > 1<<20 {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), testToken) {
			t.Errorf("token in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEnsureRepoRejects(t *testing.T) {
	s := New(Config{ReposDir: t.TempDir()})
	ctx := context.Background()
	for _, p := range []protocol.CodingEnsureRepoParams{
		{Dir: "../x", CloneURL: "https://h/x.git"},
		{Dir: "1/./x", CloneURL: "https://h/x.git"},
		{Dir: "1/a/b/c/d", CloneURL: "https://h/x.git"},
		{Dir: "1/x", CloneURL: "file:///etc"},
		{Dir: "1/x", CloneURL: "ext::sh -c id"},
		{Dir: "1/x", CloneURL: "https://user:pw@h/x.git"},
	} {
		if _, err := s.EnsureRepo(ctx, p); err == nil {
			t.Errorf("accepted %+v", p)
		}
	}
}

func TestCommandModelAndPermission(t *testing.T) {
	cfg := Config{Executors: map[string]ExecutorConfig{"claude": {Path: "/bin/sh"}, "codex": {Path: "/bin/sh"}}}
	c, _ := cfg.commandWith("codex", "p", "gpt-5", protocol.CodingPermissionFull)
	if got := strings.Join(c.args, " "); got != "exec --json --sandbox danger-full-access --model gpt-5 -" {
		t.Fatalf("codex: %s", got)
	}
	c, _ = cfg.commandWith("claude", "p", "opus", "")
	if got := strings.Join(c.args, " "); !strings.Contains(got, "acceptEdits") || !strings.HasSuffix(got, "--model opus") {
		t.Fatalf("claude: %s", got)
	}
	c, _ = cfg.commandWith("claude", "p", "", protocol.CodingPermissionFull)
	if got := strings.Join(c.args, " "); !strings.Contains(got, "bypassPermissions") {
		t.Fatalf("claude full: %s", got)
	}
}
