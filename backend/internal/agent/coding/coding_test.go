package coding

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestParseClaude(t *testing.T) {
	lines := []string{
		`{"type":"system","subtype":"init","model":"m1","session_id":"abc"}`,
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"hm"},{"type":"text","text":"Hello"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./...\nmore","description":"run"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"ok"}],"is_error":true}]}}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"Done","num_turns":3,"total_cost_usd":0.5}`,
		`{"type":"result","subtype":"error_max_turns","is_error":true}`,
		`plain text`,
		`   `,
		`{"type":"stream_event"}`,
		`{"type":"rate_limit"}`,
	}
	var got []protocol.CodingEvent
	for _, l := range lines {
		got = append(got, ParseLine("claude", []byte(l))...)
	}
	want := []struct{ kind, text string }{
		{"status", "session started"},
		{"text", "Hello"},
		{"tool", "Bash: go test ./..."},
		{"tool", "ok"},
		{"status", "finished"},
		{"error", "error_max_turns"},
		{"text", "plain text"},
		{"status", "rate_limit"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events: %+v", len(got), got)
	}
	for i, w := range want {
		if got[i].Kind != w.kind || got[i].Text != w.text {
			t.Errorf("event %d = %s %q, want %s %q", i, got[i].Kind, got[i].Text, w.kind, w.text)
		}
	}
	var data map[string]any
	_ = json.Unmarshal(got[3].Data, &data)
	if data["id"] != "t1" || data["result"] != true || data["isError"] != true {
		t.Errorf("tool result data %v", data)
	}
	_ = json.Unmarshal(got[0].Data, &data)
	if data["code"] != "session_started" || data["model"] != "m1" {
		t.Errorf("init data %v", data)
	}
}

func TestParseCodex(t *testing.T) {
	lines := []string{
		`{"type":"thread.started","thread_id":"th1"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"i0","type":"reasoning","text":"thinking"}}`,
		`{"type":"item.started","item":{"id":"i1","type":"command_execution","command":"bash -lc ls","status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"i1","type":"command_execution","command":"bash -lc ls","aggregated_output":"a.go\n","exit_code":0,"status":"completed"}}`,
		`{"type":"item.completed","item":{"id":"i2","type":"file_change","changes":[{"path":"a.go","kind":"update"}],"status":"completed"}}`,
		`{"type":"item.completed","item":{"id":"i3","type":"agent_message","text":"All done"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":10}}`,
		`{"type":"turn.failed","error":{"message":"boom"}}`,
		`{"msg":{"type":"agent_message","message":"legacy"}}`,
		`oops`,
	}
	var got []protocol.CodingEvent
	for _, l := range lines {
		got = append(got, ParseLine("codex", []byte(l))...)
	}
	want := []struct{ kind, text string }{
		{"status", "session started"},
		{"tool", "Shell: bash -lc ls"},
		{"tool", "a.go\n"},
		{"tool", "Edit: 1 files"},
		{"text", "All done"},
		{"status", "finished"},
		{"error", "boom"},
		{"text", "legacy"},
		{"text", "oops"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events: %+v", len(got), got)
	}
	for i, w := range want {
		if got[i].Kind != w.kind || got[i].Text != w.text {
			t.Errorf("event %d = %s %q, want %s %q", i, got[i].Kind, got[i].Text, w.kind, w.text)
		}
	}
}

func TestLineWriter(t *testing.T) {
	var lines []string
	w := &lineWriter{fn: func(b []byte) { lines = append(lines, string(b)) }}
	_, _ = w.Write([]byte("ab"))
	_, _ = w.Write([]byte("c\nde\n\nf"))
	w.flush()
	if strings.Join(lines, "|") != "abc|de||f" {
		t.Fatalf("%q", lines)
	}
	lines = nil
	long := strings.Repeat("x", maxLine+10)
	_, _ = w.Write([]byte(long + "\nnext\n"))
	if len(lines) != 2 || len(lines[0]) != maxLine || lines[1] != "next" {
		t.Fatalf("long line: %d lines", len(lines))
	}
}

func TestConfigCommand(t *testing.T) {
	cfg, err := ParseConfig(json.RawMessage(`{"executors":{"claude":{"path":"/bin/sh","extraArgs":["--model","x"],"env":{"A":"1"}},"codex":{"path":"/bin/sh","args":["exec","{prompt}"]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	c, err := cfg.command("claude", "do it")
	if err != nil {
		t.Fatal(err)
	}
	if c.stdin != "do it" || c.args[0] != "-p" || c.args[len(c.args)-1] != "x" || c.env[0] != "A=1" || c.format != "claude" {
		t.Fatalf("%+v", c)
	}
	c, _ = cfg.command("codex", "do it")
	if c.stdin != "" || strings.Join(c.args, " ") != "exec do it" || c.format != "codex" {
		t.Fatalf("%+v", c)
	}
	if Timeout(0) != time.Hour || Timeout(90) != 90*time.Second {
		t.Fatal("timeout")
	}
}

// ---- tests with git and the fake executor ----

func needGit(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake executor is a shell script")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

// FakeExecutor is the path of testdata/fake-claude.sh.
func fakeExecutor(t *testing.T) string {
	p, err := filepath.Abs("testdata/fake-claude.sh")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@x")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo makes a repository with one commit on main and a bare origin.
func newRepo(t *testing.T) (repo, origin string) {
	t.Helper()
	base := t.TempDir()
	origin = filepath.Join(base, "origin.git")
	repo = filepath.Join(base, "work", "demo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, base, "init", "--quiet", "--bare", origin)
	run(t, repo, "init", "--quiet", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "add", ".")
	run(t, repo, "commit", "--quiet", "-m", "init")
	run(t, repo, "remote", "add", "origin", origin)
	run(t, repo, "push", "--quiet", "-u", "origin", "main")
	return repo, origin
}

type recorder struct {
	mu     sync.Mutex
	events []protocol.CodingEvent
}

func (r *recorder) emit(ev protocol.CodingEvent) {
	r.mu.Lock()
	r.events = append(r.events, ev)
	r.mu.Unlock()
}

func (r *recorder) find(kind, code string) (protocol.CodingEvent, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ev := range r.events {
		if ev.Kind != kind {
			continue
		}
		var d struct{ Code string }
		_ = json.Unmarshal(ev.Data, &d)
		if code == "" || d.Code == code {
			return ev, true
		}
	}
	return protocol.CodingEvent{}, false
}

func (r *recorder) done(t *testing.T) (int, protocol.CodingDone) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	last := r.events[len(r.events)-1]
	if last.Kind != protocol.CodingEventDone || last.ExitCode == nil {
		t.Fatalf("last event is not done: %+v", last)
	}
	var d protocol.CodingDone
	_ = json.Unmarshal(last.Data, &d)
	return *last.ExitCode, d
}

func fakeService(t *testing.T) *Service {
	return New(Config{Executors: map[string]ExecutorConfig{"claude": {Path: fakeExecutor(t)}, "codex": {Path: "/nonexistent/codex"}}})
}

func TestExecutorsAndRepos(t *testing.T) {
	needGit(t)
	s := fakeService(t)
	list := s.Executors(context.Background())
	if len(list.Items) != 2 || !list.Items[0].Available || list.Items[0].Version != "9.9.9 (Fake Claude)" || list.Items[1].Available || list.Items[1].Error == "" {
		t.Fatalf("%+v", list)
	}
	if !s.anyExecutor() {
		t.Fatal("anyExecutor")
	}

	repo, origin := newRepo(t)
	root := filepath.Dir(filepath.Dir(repo)) // contains work/demo and origin.git (bare, no .git)
	deep := filepath.Join(root, "a", "b", "c", "deep")
	hidden := filepath.Join(root, ".hidden", "repo")
	for _, d := range []string{deep, hidden} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		run(t, d, "init", "--quiet", "-b", "trunk")
	}
	got, err := s.Repos(context.Background(), protocol.CodingReposParams{Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Path != repo || got.Items[0].Name != "demo" ||
		got.Items[0].CurrentBranch != "main" || got.Items[0].DefaultBranch != "main" || got.Items[0].RemoteURL != origin {
		t.Fatalf("%+v", got.Items)
	}
	got, _ = s.Repos(context.Background(), protocol.CodingReposParams{Roots: []string{root}, Depth: 4})
	if len(got.Items) != 2 || got.Items[0].Path != deep || got.Items[0].DefaultBranch != "" {
		t.Fatalf("depth 4: %+v", got.Items)
	}
	got, _ = s.Repos(context.Background(), protocol.CodingReposParams{Roots: []string{repo}, Depth: -1})
	if len(got.Items) != 1 {
		t.Fatalf("single: %+v", got.Items)
	}
	got, _ = s.Repos(context.Background(), protocol.CodingReposParams{Roots: []string{root}, Depth: -1})
	if len(got.Items) != 0 {
		t.Fatalf("root is not a repo: %+v", got.Items)
	}
}

func TestRunReviewCommitPushDiscard(t *testing.T) {
	needGit(t)
	s := fakeService(t)
	repo, origin := newRepo(t)
	ctx := context.Background()
	rec := &recorder{}
	p := protocol.CodingRunParams{TaskID: 7, RepoPath: repo, Executor: "claude", Prompt: "add hello", BaseBranch: "main", Branch: "xc/7-add-hello"}
	s.Run(ctx, p, rec.emit, nil)
	code, done := rec.done(t)
	if code != 0 || done.Reason != protocol.CodingEndExited || done.BaseCommit == "" {
		t.Fatalf("done: %d %+v", code, done)
	}
	if len(done.Files) != 2 || done.Files[0].Path != "README.md" || done.Files[0].Status != "M" ||
		done.Files[1].Path != "hello.txt" || done.Files[1].Status != "A" || done.Files[1].Additions != 1 {
		t.Fatalf("files: %+v", done.Files)
	}
	for _, want := range []struct{ kind, code string }{{"status", "worktree_ready"}, {"status", "session_started"}, {"status", "started"}, {"tool", ""}, {"text", ""}, {"status", "result"}} {
		if _, ok := rec.find(want.kind, want.code); !ok {
			t.Errorf("missing %s %s event", want.kind, want.code)
		}
	}
	var stderr, plainLine bool
	for _, ev := range rec.events {
		stderr = stderr || (ev.Kind == "text" && strings.Contains(string(ev.Data), "stderr") && ev.Text == "warning: fake stderr line")
		plainLine = plainLine || ev.Text == "not json: plain progress line"
	}
	if !stderr || !plainLine {
		t.Errorf("stderr %v plain %v", stderr, plainLine)
	}
	wt := WorktreePath(repo, 7)
	if b, err := os.ReadFile(filepath.Join(wt, "hello.txt")); err != nil || string(b) != "add hello\n" {
		t.Fatalf("hello.txt: %q %v", b, err)
	}
	if st := run(t, repo, "status", "--porcelain"); st != "" {
		t.Fatalf("main checkout is not clean: %q", st)
	}

	tp := protocol.CodingTaskParams{TaskID: 7, RepoPath: repo, Branch: p.Branch, BaseBranch: "main", BaseCommit: done.BaseCommit}
	diff, err := s.Diff(ctx, tp)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Files) != 2 || !strings.Contains(diff.Diff, "+changed") || !strings.Contains(diff.Diff, "+add hello") || diff.Truncated {
		t.Fatalf("diff: %+v", diff)
	}

	if _, err := s.Commit(ctx, protocol.CodingCommitParams{CodingTaskParams: tp}); err == nil {
		t.Fatal("empty message accepted")
	}
	res, err := s.Commit(ctx, protocol.CodingCommitParams{CodingTaskParams: tp, Message: "Add hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.SHA) != 40 || run(t, repo, "rev-parse", p.Branch) != res.SHA {
		t.Fatalf("sha %q", res.SHA)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatal("worktree kept after commit")
	}
	diff, err = s.Diff(ctx, tp) // from the branch now
	if err != nil || len(diff.Files) != 2 {
		t.Fatalf("diff after commit: %+v %v", diff, err)
	}

	if err := s.Push(ctx, protocol.CodingPushParams{CodingTaskParams: tp}); err != nil {
		t.Fatal(err)
	}
	if got := run(t, origin, "rev-parse", "refs/heads/"+p.Branch); got != res.SHA {
		t.Fatalf("remote has %q", got)
	}

	if err := s.Discard(ctx, tp); err != nil {
		t.Fatal(err)
	}
	if branchExists(ctx, repo, p.Branch) {
		t.Fatal("branch kept after discard")
	}
	if err := s.Discard(ctx, tp); err != nil {
		t.Fatalf("second discard: %v", err)
	}
	if err := s.Discard(ctx, protocol.CodingTaskParams{TaskID: 7, RepoPath: repo, Branch: "main"}); err == nil {
		t.Fatal("discard accepted a branch without the xc/ prefix")
	}
}

func TestRunFailureKeepsWorktree(t *testing.T) {
	needGit(t)
	s := fakeService(t)
	repo, _ := newRepo(t)
	rec := &recorder{}
	s.Run(context.Background(), protocol.CodingRunParams{TaskID: 8, RepoPath: repo, Executor: "claude", Prompt: "FAIL please", Branch: "xc/8-fail"}, rec.emit, nil)
	code, done := rec.done(t)
	if code != 3 || done.Reason != protocol.CodingEndExited {
		t.Fatalf("%d %+v", code, done)
	}
	if ev, ok := rec.find("error", "result"); !ok || ev.Text != "fake failure" {
		t.Fatalf("error event %+v", ev)
	}
	if _, err := os.Stat(WorktreePath(repo, 8)); err != nil {
		t.Fatal("worktree removed after a failed run")
	}
}

func startedPID(t *testing.T, rec *recorder) int {
	t.Helper()
	ev, ok := rec.find("status", "started")
	if !ok {
		t.Fatal("no started event")
	}
	var d struct{ PID int }
	_ = json.Unmarshal(ev.Data, &d)
	return d.PID
}

func TestRunCancel(t *testing.T) {
	needGit(t)
	s := fakeService(t)
	repo, _ := newRepo(t)
	rec := &recorder{}
	cancel := make(chan struct{})
	go func() {
		for {
			if _, ok := rec.find("text", ""); ok {
				close(cancel)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	start := time.Now()
	s.Run(context.Background(), protocol.CodingRunParams{TaskID: 9, RepoPath: repo, Executor: "claude", Prompt: "SLEEP", Branch: "xc/9-sleep"}, rec.emit, cancel)
	if time.Since(start) > 8*time.Second {
		t.Fatalf("cancel took %v", time.Since(start))
	}
	code, done := rec.done(t)
	if code != -1 || done.Reason != protocol.CodingEndCanceled {
		t.Fatalf("%d %+v", code, done)
	}
	if _, err := os.Stat(WorktreePath(repo, 9)); !os.IsNotExist(err) {
		t.Fatal("worktree kept after cancel")
	}
	if branchExists(context.Background(), repo, "xc/9-sleep") {
		t.Fatal("branch kept after cancel")
	}
	if pid := startedPID(t, rec); !groupGone(pid) {
		t.Fatal("process group still alive")
	}
}

func TestRunTimeoutKillsAfterGrace(t *testing.T) {
	needGit(t)
	old := KillGrace
	KillGrace = 500 * time.Millisecond
	defer func() { KillGrace = old }()
	s := fakeService(t)
	repo, _ := newRepo(t)
	rec := &recorder{}
	start := time.Now()
	s.Run(context.Background(), protocol.CodingRunParams{TaskID: 10, RepoPath: repo, Executor: "claude", Prompt: "SLEEP TRAP", Branch: "xc/10-trap", TimeoutSeconds: 1}, rec.emit, nil)
	if d := time.Since(start); d > 10*time.Second || d < time.Second {
		t.Fatalf("timeout took %v", d)
	}
	code, done := rec.done(t)
	if code != -1 || done.Reason != protocol.CodingEndTimeout {
		t.Fatalf("%d %+v", code, done)
	}
	if _, err := os.Stat(WorktreePath(repo, 10)); !os.IsNotExist(err) {
		t.Fatal("worktree kept after timeout")
	}
	if pid := startedPID(t, rec); !groupGone(pid) {
		t.Fatal("process group still alive")
	}
}

func TestRunRejectsBadParams(t *testing.T) {
	needGit(t)
	s := fakeService(t)
	repo, _ := newRepo(t)
	for _, p := range []protocol.CodingRunParams{
		{TaskID: 1, RepoPath: repo, Executor: "claude", Prompt: "x", Branch: "feature/x"},
		{TaskID: 1, RepoPath: "relative/path", Executor: "claude", Prompt: "x", Branch: "xc/1"},
		{TaskID: 1, RepoPath: repo, Executor: "vim", Prompt: "x", Branch: "xc/1"},
		{TaskID: 1, RepoPath: repo, Executor: "codex", Prompt: "x", Branch: "xc/1"},
		{TaskID: 1, RepoPath: repo, Executor: "claude", Prompt: "x", Branch: "xc/1", BaseBranch: "nope"},
		{TaskID: 0, RepoPath: repo, Executor: "claude", Prompt: "x", Branch: "xc/1"},
	} {
		rec := &recorder{}
		s.Run(context.Background(), p, rec.emit, nil)
		code, done := rec.done(t)
		if code != -1 || done.Reason != protocol.CodingEndStartFailed || done.Error == "" {
			t.Errorf("%+v: %d %+v", p, code, done)
		}
	}
	if branchExists(context.Background(), repo, "xc/1") {
		t.Fatal("branch created by a rejected run")
	}
}
