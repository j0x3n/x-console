package coding

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// B47 builds. A task in review keeps its worktree; the repository's build
// steps run there (coding.build). Artifacts are copied from the agent into
// the coding file store under artifacts/<task id>/. A failed build of an AI
// agent's task goes back to the same agent with the end of the log, up to
// the agent's retry count.

const (
	buildPassed  = "passed"
	buildFailed  = "failed"
	buildRunning = "running"
	// fixLogLines is how much of a failed build the agent gets to see.
	fixLogLines = 200
)

type buildConfig struct {
	Linux   []protocol.CodingBuildStep `json:"linux"`
	Windows []protocol.CodingBuildStep `json:"windows"`
}

func parseBuildConfig(raw string) buildConfig {
	var c buildConfig
	_ = json.Unmarshal([]byte(raw), &c)
	if c.Linux == nil {
		c.Linux = []protocol.CodingBuildStep{}
	}
	if c.Windows == nil {
		c.Windows = []protocol.CodingBuildStep{}
	}
	return c
}

func toAPIBuildConfig(raw string) *api.BuildConfig {
	c := parseBuildConfig(raw)
	conv := func(steps []protocol.CodingBuildStep) []api.BuildStep {
		out := make([]api.BuildStep, 0, len(steps))
		for _, s := range steps {
			step := api.BuildStep{Name: s.Name, Command: s.Command}
			if s.TimeoutSeconds > 0 {
				t := s.TimeoutSeconds
				step.TimeoutSeconds = &t
			}
			if len(s.Artifacts) > 0 {
				a := s.Artifacts
				step.Artifacts = &a
			}
			out = append(out, step)
		}
		return out
	}
	return &api.BuildConfig{Linux: conv(c.Linux), Windows: conv(c.Windows)}
}

// artifactRef is one stored artifact.
type artifactRef struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Key  string `json:"key"`
}

func artifactsOf(raw string) []artifactRef {
	var out []artifactRef
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

// SetRepoBuildConfig implements PUT /coding/repos/{id}/build-config.
func (m *Module) SetRepoBuildConfig(w http.ResponseWriter, r *http.Request, id api.RepoId) {
	ctx := r.Context()
	var body api.BuildConfig
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := func() (row db.CodingRepo, err error) {
		defer func() {
			m.d.Audit.Record(ctx, "coding_repo.build_config", itoa(id), map[string]any{"config": body}, err)
		}()
		if err := auth.RequireElevated(ctx); err != nil {
			return row, err
		}
		c := buildConfig{}
		for _, set := range []struct {
			in  []api.BuildStep
			out *[]protocol.CodingBuildStep
		}{{body.Linux, &c.Linux}, {body.Windows, &c.Windows}} {
			if len(set.in) > 20 {
				return row, httpx.Invalid("构建步骤最多 20 步")
			}
			*set.out = []protocol.CodingBuildStep{}
			for _, s := range set.in {
				step := protocol.CodingBuildStep{Name: strings.TrimSpace(s.Name), Command: strings.TrimSpace(s.Command)}
				if step.Name == "" || step.Command == "" {
					return row, httpx.Invalid("每一步都要有名字和命令")
				}
				if s.TimeoutSeconds != nil {
					step.TimeoutSeconds = *s.TimeoutSeconds
				}
				if s.Artifacts != nil {
					for _, a := range *s.Artifacts {
						if a = strings.TrimSpace(a); a != "" {
							step.Artifacts = append(step.Artifacts, a)
						}
					}
				}
				*set.out = append(*set.out, step)
			}
		}
		raw, _ := json.Marshal(c)
		row, err = m.q.SetBuildConfig(ctx, db.SetBuildConfigParams{BuildConfig: string(raw), ID: id})
		if errors.Is(err, sql.ErrNoRows) {
			return row, httpx.ErrNotFound
		}
		return row, err
	}()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	repo := m.toRepo(row, m.agentNames(ctx))
	m.d.Bus.Publish("coding_repo.updated", repo)
	httpx.JSON(w, http.StatusOK, repo)
}

// stepsFor picks the steps for the machine of the task.
func (m *Module) stepsFor(r taskRow) ([]protocol.CodingBuildStep, error) {
	hello, ok := m.d.Agents.Hello(r.AgentID)
	if !ok {
		return nil, httpx.NewError(http.StatusServiceUnavailable, "agent_offline", "机器不在线，不能构建")
	}
	if !hasCap(hello.Capabilities, protocol.CapCodingRemote) {
		return nil, httpx.NewError(http.StatusNotImplemented, "feature_unavailable", "这台机器的代理版本太旧，不能构建，请先升级代理")
	}
	c := parseBuildConfig(r.RepoBuildConfig)
	steps := c.Linux
	if hello.OS == "windows" {
		steps = c.Windows
	}
	if len(steps) == 0 {
		return nil, httpx.Invalid("这个仓库没有设置 " + hello.OS + " 上的构建步骤")
	}
	return steps, nil
}

// BuildTask implements POST /coding/tasks/{id}/build.
func (m *Module) BuildTask(w http.ResponseWriter, r *http.Request, id api.TaskId) {
	err := m.startBuild(r.Context(), id)
	m.d.Audit.Record(r.Context(), "coding_task.build", itoa(id), nil, err)
	m.respond(w, r, id, err)
}

// startBuild opens the build stream of a task in review.
func (m *Module) startBuild(ctx context.Context, id int64) error {
	row, err := m.row(ctx, id)
	if err != nil {
		return err
	}
	steps, err := m.stepsFor(row)
	if err != nil {
		return err
	}
	runCtx := m.runCtx()
	if runCtx == nil {
		return errors.New("coding module not started")
	}
	n, err := m.q.StartBuild(ctx, db.StartBuildParams{Now: m.now(), ID: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return conflict("只有待审查、不在构建中的任务能构建")
	}
	m.publishTask(ctx, id)
	st, err := m.d.Agents.Open(runCtx, row.AgentID, protocol.MethodCodingBuild,
		protocol.CodingBuildParams{CodingTaskParams: m.taskParams(row), Steps: steps})
	if err != nil {
		m.finishBuild(runCtx, row, buildFailed, "无法在代理上开始构建："+err.Error(), nil, nil)
		return nil
	}
	seq, err := m.q.LastSeq(ctx, id)
	if err != nil {
		st.Close(nil)
		return err
	}
	run := &taskRun{id: id, stream: st, executor: row.Executor, started: m.now(), seq: seq}
	limit := time.Duration(0)
	for _, s := range steps {
		limit += time.Duration(max(s.TimeoutSeconds, 0)) * time.Second
		if s.TimeoutSeconds <= 0 {
			limit += 30 * time.Minute
		}
	}
	m.mu.Lock()
	m.builds[id] = run
	m.mu.Unlock()
	m.wg.Add(1)
	go m.consumeBuild(runCtx, run, row, limit+m.overtime)
	return nil
}

func (m *Module) publishTask(ctx context.Context, id int64) {
	if t, err := m.task(ctx, id); err == nil {
		m.d.Bus.Publish("coding_task.updated", t)
	}
}

func (m *Module) building(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.builds[id]
	return ok
}

// releaseBuild frees the build slot of the task if it still holds this run.
// A later build of the same task keeps its own slot.
func (m *Module) releaseBuild(run *taskRun) {
	m.mu.Lock()
	if m.builds[run.id] == run {
		delete(m.builds, run.id)
	}
	m.mu.Unlock()
}

// consumeBuild stores the build output and records the result.
func (m *Module) consumeBuild(ctx context.Context, run *taskRun, row taskRow, limit time.Duration) {
	defer m.wg.Done()
	defer m.releaseBuild(run)
	events := make(chan protocol.CodingEvent, 256)
	go func() {
		defer close(events)
		for {
			chunk, err := run.stream.Recv(ctx)
			if err != nil {
				return
			}
			for _, line := range bytes.Split(chunk, []byte("\n")) {
				if len(bytes.TrimSpace(line)) == 0 {
					continue
				}
				var ev protocol.CodingEvent
				if err := json.Unmarshal(line, &ev); err != nil || ev.Kind == "" {
					ev = protocol.CodingEvent{Kind: protocol.CodingEventText, Text: string(line), At: m.now()}
				}
				events <- ev
			}
		}
	}()
	ticker := time.NewTicker(m.flushEvery)
	defer ticker.Stop()
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	var pending []protocol.CodingEvent
	var tail []string
	var done *protocol.CodingBuildDone
loop:
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				break loop
			}
			pending = append(pending, ev)
			switch ev.Kind {
			case protocol.CodingEventText, protocol.CodingEventError:
				tail = append(tail, ev.Text)
				if len(tail) > fixLogLines {
					tail = tail[len(tail)-fixLogLines:]
				}
			case protocol.CodingEventDone:
				var d protocol.CodingBuildDone
				_ = json.Unmarshal(ev.Data, &d)
				done = &d
			}
		case <-ticker.C:
			pending = m.flush(ctx, run, pending)
		case <-deadline.C:
			run.stream.Close(nil)
		}
	}
	run.stream.Close(nil)
	if ctx.Err() != nil {
		return
	}
	m.flush(ctx, run, pending)
	// The task reads as building until its result is stored, so free the
	// slot first: a commit right after the result shows must not get 409.
	m.releaseBuild(run)
	switch {
	case done == nil:
		m.finishBuild(ctx, row, buildFailed, "代理没有报告构建结果就断开了。", nil, tail)
	case done.Reason == protocol.CodingBuildPassed:
		refs, err := m.fetchArtifacts(ctx, row, done.Artifacts)
		msg := done.Error
		if err != nil {
			msg = strings.TrimSpace(msg + " 产物没有全部取回：" + err.Error())
		}
		m.finishBuild(ctx, row, buildPassed, msg, refs, nil)
	default:
		msg := "构建出错：" + done.Error
		switch done.Reason {
		case protocol.CodingBuildFailed:
			msg = fmt.Sprintf("第 %d 步失败了。", done.FailedStep+1)
		case protocol.CodingBuildTimeout:
			msg = fmt.Sprintf("第 %d 步超时了。", done.FailedStep+1)
		case protocol.CodingBuildCanceled:
			msg = "构建被中止了。"
		}
		m.finishBuild(ctx, row, buildFailed, msg, nil, tail)
	}
}

// finishBuild stores the result, tells the AI agents module and, for a
// failed build of an agent's task, asks the agent to fix it.
func (m *Module) finishBuild(ctx context.Context, row taskRow, status, msg string, refs []artifactRef, tail []string) {
	if refs == nil {
		refs = artifactsOf(row.Artifacts)
		if status == buildFailed {
			refs = []artifactRef{}
		}
	}
	raw, _ := json.Marshal(refs)
	if err := m.q.FinishBuild(ctx, db.FinishBuildParams{BuildStatus: status, BuildError: msg, Artifacts: string(raw),
		UpdatedAt: m.now(), ID: row.ID}); err != nil {
		slog.Error("coding: finish build", "task", row.ID, "err", err)
		return
	}
	t, err := m.task(ctx, row.ID)
	if err != nil {
		return
	}
	m.d.Bus.Publish("coding_task.updated", t)
	m.d.Bus.Publish("coding_task.build", map[string]any{"taskId": row.ID, "status": status, "error": msg,
		"issueKey": row.IssueKey, "aiAgentId": row.AiAgentID, "attempt": row.BuildAttempts + 1})
	if status == buildFailed && row.AiAgentID != nil {
		m.maybeFix(ctx, row, tail)
	}
	if status == buildPassed {
		m.autoPR(ctx, row.ID)
	}
}

// maybeFix gives a failed build back to the task's agent when it has
// retries left: the executor runs again in the same worktree.
func (m *Module) maybeFix(ctx context.Context, row taskRow, tail []string) {
	agents, ok := module.Lookup[contracts.AIAgents](m.d.Registry, contracts.AIAgentsKey)
	if !ok {
		return
	}
	a, err := agents.Get(ctx, *row.AiAgentID)
	if err != nil || !a.Enabled || a.OverBudget {
		return
	}
	// BuildAttempts counted this build already (row is from before it).
	if int(row.BuildAttempts+1) > a.BuildRetries {
		return
	}
	prompt := "上一次的改动构建失败了。请根据下面的构建日志修好它，只改和失败有关的地方。\n\n```\n" +
		strings.Join(tail, "\n") + "\n```\n\n原来的需求：\n\n" + row.Prompt
	if err := m.resume(ctx, row, prompt); err != nil {
		slog.Warn("coding: retry after failed build", "task", row.ID, "err", err)
	}
}

// resume runs the executor again in the kept worktree of a task in review.
func (m *Module) resume(ctx context.Context, row taskRow, prompt string) error {
	n, err := m.q.ResumeForFix(ctx, db.ResumeForFixParams{Now: m.now(), ID: row.ID})
	if err != nil || n == 0 {
		return err
	}
	params := protocol.CodingRunParams{
		AllowQuestions: row.AiAgentID != nil,
		TaskID:         row.ID, RepoPath: row.RepoPath, Executor: row.Executor, Prompt: prompt, Branch: row.Branch,
		TimeoutSeconds: int(time.Duration(row.TimeoutMinutes) * m.timeoutUnit / time.Second),
		Model:          row.Model, Permission: row.Permission, Continue: true, BaseCommit: row.BaseCommit,
	}
	st, err := m.d.Agents.Open(ctx, row.AgentID, protocol.MethodCodingRun, params)
	if err != nil {
		m.finish(ctx, row.ID, statusFailed, nil, "无法在代理上继续任务："+err.Error(), nil)
		return nil
	}
	seq, _ := m.q.LastSeq(ctx, row.ID)
	run := &taskRun{id: row.ID, stream: st, executor: row.Executor, started: m.now(), seq: seq}
	m.mu.Lock()
	m.runs[row.ID] = run
	m.mu.Unlock()
	m.publishTask(ctx, row.ID)
	m.wg.Add(1)
	go m.consume(ctx, run, row)
	return nil
}

// afterReview starts the automatic build of an AI agent's task.
func (m *Module) afterReview(ctx context.Context, id int64) {
	row, err := m.row(ctx, id)
	if err != nil || row.AiAgentID == nil || row.WaitingQuestion != "" {
		return
	}
	agents, ok := module.Lookup[contracts.AIAgents](m.d.Registry, contracts.AIAgentsKey)
	if !ok {
		return
	}
	a, err := agents.Get(ctx, *row.AiAgentID)
	if err != nil {
		return
	}
	if !a.AutoBuild {
		m.autoPR(ctx, id)
		return
	}
	if _, err := m.stepsFor(row); err != nil {
		m.autoPR(ctx, id)
		return // nothing to build here
	}
	if err := m.startBuild(ctx, id); err != nil {
		slog.Warn("coding: automatic build", "task", id, "err", err)
	}
}

// fetchArtifacts copies the files from the agent into the file store.
func (m *Module) fetchArtifacts(ctx context.Context, row taskRow, list []protocol.CodingArtifact) ([]artifactRef, error) {
	store := m.d.Files.For("coding")
	prefix := "artifacts/" + itoa(row.ID) + "/"
	if hello, ok := m.d.Agents.Hello(row.AgentID); len(list) > 0 && (!ok || !hasCap(hello.Capabilities, protocol.CapFiles)) {
		return []artifactRef{}, errors.New("这台机器的代理不能读文件，取不回产物")
	}
	// Old artifacts of this task go first.
	for _, old := range artifactsOf(row.Artifacts) {
		_ = store.Delete(ctx, old.Key)
	}
	refs := []artifactRef{}
	var errs []error
	for i, a := range list {
		if i == protocol.CodingArtifactMaxFiles {
			break
		}
		key := prefix + strconv.Itoa(i) + "-" + safeName(a.Name)
		size, err := m.copyFromAgent(ctx, row.AgentID, a.Path, key)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", a.Name, err))
			continue
		}
		refs = append(refs, artifactRef{Name: a.Name, Size: size, Key: key})
	}
	return refs, errors.Join(errs...)
}

// safeName turns a relative path into one key segment.
func safeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/' || r == '\\':
			b.WriteRune('_')
		case r < 0x20 || r == 0x7f:
		default:
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" || s == "." || s == ".." {
		s = "file"
	}
	if len(s) > 200 {
		s = s[len(s)-200:]
	}
	return s
}

// copyFromAgent streams one file (files.read) into the store.
func (m *Module) copyFromAgent(ctx context.Context, agentID, path, key string) (int64, error) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	s, err := m.d.Agents.Open(cctx, agentID, protocol.MethodFilesRead, protocol.FilesReadParams{Path: path})
	if err != nil {
		return 0, err
	}
	defer s.Close(nil)
	first, err := s.Recv(cctx)
	if err != nil {
		return 0, err
	}
	var head protocol.FileHeader
	if err := json.Unmarshal(first, &head); err != nil {
		return 0, errors.New("代理返回的数据格式不对")
	}
	if head.Size > protocol.CodingArtifactMaxSize {
		return 0, errors.New("文件超过 2 GB")
	}
	pr, pw := io.Pipe()
	go func() {
		var n int64
		for {
			chunk, err := s.Recv(cctx)
			if errors.Is(err, io.EOF) {
				if n != head.Size {
					pw.CloseWithError(fmt.Errorf("只收到 %d / %d 字节", n, head.Size))
					return
				}
				pw.Close()
				return
			}
			if err != nil {
				pw.CloseWithError(err)
				return
			}
			n += int64(len(chunk))
			if _, err := pw.Write(chunk); err != nil {
				return
			}
		}
	}()
	err = m.d.Files.For("coding").Put(cctx, key, pr, head.Size)
	_ = pr.CloseWithError(err)
	return head.Size, err
}

// DownloadTaskArtifact implements GET /coding/tasks/{id}/artifacts/{index}.
func (m *Module) DownloadTaskArtifact(w http.ResponseWriter, r *http.Request, id api.TaskId, index int) {
	ctx := r.Context()
	row, err := m.row(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	refs := artifactsOf(row.Artifacts)
	if index < 0 || index >= len(refs) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	ref := refs[index]
	rc, info, err := m.d.Files.For("coding").Get(ctx, ref.Key)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	defer rc.Close()
	name := ref.Name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, rc)
}

// deleteArtifacts removes the stored artifacts of tasks.
func (m *Module) deleteArtifacts(ctx context.Context, taskIDs []int64) {
	store := m.d.Files.For("coding")
	for _, id := range taskIDs {
		for info, err := range store.List(ctx, "artifacts/"+itoa(id)) {
			if err != nil {
				break
			}
			_ = store.Delete(ctx, info.Key)
		}
	}
}
