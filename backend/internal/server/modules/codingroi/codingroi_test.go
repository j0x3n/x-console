package codingroi_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/codingroi"
	"github.com/j0x3n/x-console/backend/internal/server/modules/codingroi/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

var cst = time.FixedZone("CST", 8*3600)

// now is Saturday 2026-10-10 12:00 in Asia/Shanghai. Its week is Monday
// 2026-10-05 to Sunday 2026-10-11.
var now = time.Date(2026, 10, 10, 12, 0, 0, 0, cst)

// at is a time on the test week: day 0 is Monday 2026-10-05.
func at(day, hour, min int) time.Time {
	return time.Date(2026, 10, 5+day, hour, min, 0, 0, cst).UTC()
}

type rig struct {
	t   *testing.T
	env *testutil.Env
	m   *codingroi.Module
}

func (r *rig) exec(query string, args ...any) {
	r.t.Helper()
	if _, err := r.env.App.Deps.DB.Exec(query, args...); err != nil {
		r.t.Fatalf("%s: %v", query, err)
	}
}

func setup(t *testing.T) *rig {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*codingroi.Module](env.App.Deps.Registry, codingroi.ServiceKey)
	if !ok {
		t.Fatal("codingroi module not registered")
	}
	codingroi.SetNow(m, func() time.Time { return now.UTC() })
	return &rig{t: t, env: env, m: m}
}

type taskSeed struct {
	id                      int64
	repo                    int64
	executor, prompt        string
	status, prURL           string
	agent                   any
	created, started, ended any
	updated                 time.Time
	runs                    int
}

func (r *rig) task(s taskSeed) {
	r.t.Helper()
	if s.updated.IsZero() {
		s.updated = at(0, 0, 0)
	}
	r.exec(`INSERT INTO coding_tasks (id, repo_id, executor, prompt, status, pr_url, ai_agent_id, created_at, started_at, finished_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, s.id, s.repo, s.executor, s.prompt, s.status, s.prURL, s.agent, s.created, s.started, s.ended, s.updated)
	for i := 1; i <= s.runs; i++ {
		r.exec(`INSERT INTO coding_task_events (task_id, seq, at, kind, text, data) VALUES (?, ?, ?, 'done', '', '{"reason":"exited","_exitCode":0}')`,
			s.id, i, at(0, 0, 0))
	}
}

func (r *rig) usage(task int64, cost any, in, out, ms int64) {
	r.t.Helper()
	r.exec(`INSERT INTO ai_usage (provider_name, model, purpose, input_tokens, output_tokens, duration_ms, cost, created_at, source, ref)
VALUES ('Claude Code', 'm', 'agent', ?, ?, ?, ?, ?, 'coding', ?)`, in, out, ms, cost, at(0, 0, 0), itoa(task))
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// screen writes count minutes in a row starting at the given time.
func (r *rig) screen(start time.Time, count int, category string) {
	r.t.Helper()
	first := start.Unix() / 60
	for i := 0; i < count; i++ {
		r.exec(`INSERT OR REPLACE INTO screen_minutes (host_id, minute, app, category, title) VALUES ('pc1', ?, 'Code.exe', ?, '')`, first+int64(i), category)
	}
}

// seed fills one week. See docs/specs/B129.md for what each task stands for.
func (r *rig) seed() {
	r.exec(`INSERT INTO agents (id, name, kind, token_hash, created_at) VALUES ('a1', '台式机', 'desktop', 'h1', ?)`, at(0, 0, 0))
	r.exec(`INSERT INTO coding_repos (id, agent_id, path, name, created_at) VALUES (1, 'a1', '/src/xc', 'x-console', ?)`, at(0, 0, 0))
	r.exec(`INSERT INTO coding_repos (id, agent_id, path, name, created_at) VALUES (2, 'a1', '/src/blog', 'blog', ?)`, at(0, 0, 0))
	r.exec(`INSERT INTO ai_agents (id, name, kind, created_at, updated_at) VALUES (1, 'Builder', 'claude_code', ?, ?)`, at(0, 0, 0), at(0, 0, 0))

	// T1: Claude Code by an agent, one run of 20 minutes, merged on Monday 15:00.
	r.task(taskSeed{id: 1, repo: 1, executor: "claude", prompt: "修复溢出\n详细说明", status: "pr_opened", prURL: "https://github.com/o/xc/pull/1",
		agent: 1, created: at(0, 9, 0), started: at(0, 9, 0), ended: at(0, 9, 20), runs: 1})
	r.exec(`INSERT INTO coding_task_events (task_id, seq, at, kind, text, data) VALUES (1, 2, ?, 'done', '', '{"reason":"passed","failedStep":-1}')`, at(0, 9, 25))
	r.usage(1, 0.5, 1000, 200, 1_200_000)
	if err := codingroi.RecordOutcome(r.m, context.Background(), 1, "merged", at(0, 15, 0)); err != nil {
		r.t.Fatal(err)
	}
	r.screen(at(0, 9, 30), 30, "coding")
	r.screen(at(0, 10, 0), 10, "web")

	// T2: Codex by hand, two runs, no cost, discarded Tuesday 11:00.
	r.task(taskSeed{id: 2, repo: 1, executor: "codex", prompt: "=SUM(1)", status: "discarded",
		created: at(1, 10, 0), started: at(1, 10, 0), ended: at(1, 10, 30), updated: at(1, 11, 0), runs: 2})
	r.usage(2, nil, 500, 100, 600_000)
	r.usage(2, nil, 500, 100, 300_000)

	// T3: Claude Code by hand, committed Tuesday 10:35, no result since. One of
	// its two usage records has no cost.
	r.task(taskSeed{id: 3, repo: 2, executor: "claude", prompt: "写文章", status: "committed",
		created: at(1, 10, 0), started: at(1, 10, 0), ended: at(1, 10, 35), runs: 1})
	r.usage(3, 0.3, 300, 50, 100)
	r.usage(3, nil, 10, 10, 100)
	r.screen(at(1, 10, 40), 10, "coding") // inside T2 and T3
	r.screen(at(1, 12, 0), 10, "coding")  // inside T3 only

	// T4: failed after 2 minutes, no usage recorded.
	r.task(taskSeed{id: 4, repo: 2, executor: "claude", prompt: "失败的", status: "failed",
		created: at(2, 9, 0), started: at(2, 9, 0), ended: at(2, 9, 2), runs: 1})

	// T5: still queued.
	r.task(taskSeed{id: 5, repo: 2, executor: "codex", prompt: "排队中", status: "queued", created: at(3, 9, 0)})

	// T6: waiting for the user since Friday 15:00, with no computer time recorded then.
	r.task(taskSeed{id: 6, repo: 1, executor: "claude", prompt: "等你看", status: "review",
		created: at(4, 14, 0), started: at(4, 14, 0), ended: at(4, 15, 0), runs: 1})

	// T7: the week before.
	r.task(taskSeed{id: 7, repo: 1, executor: "claude", prompt: "上周的", status: "discarded",
		created: at(-1, 20, 0), started: at(-1, 20, 0), ended: at(-1, 20, 10), updated: at(-1, 21, 0), runs: 1})
}

func (r *rig) report(query string) api.CodingROIReport {
	r.t.Helper()
	var rep api.CodingROIReport
	r.env.MustDo(http.MethodGet, "/codingroi/report"+query, nil, &rep)
	return rep
}

func taskByID(t *testing.T, rep api.CodingROIReport, id int64) api.CodingROITask {
	t.Helper()
	for _, x := range rep.Tasks {
		if x.Id == id {
			return x
		}
	}
	t.Fatalf("task %d not in report", id)
	return api.CodingROITask{}
}

func groupByKey(t *testing.T, rep api.CodingROIReport, key string) api.CodingROIGroup {
	t.Helper()
	for _, g := range rep.Groups {
		if g.Key == key {
			return g
		}
	}
	t.Fatalf("group %q not in report: %+v", key, rep.Groups)
	return api.CodingROIGroup{}
}

func near(a *float64, want float64) bool { return a != nil && *a > want-0.001 && *a < want+0.001 }

func TestWeeklyReportPerTask(t *testing.T) {
	r := setup(t)
	r.seed()
	rep := r.report("?date=2026-10-07")
	if rep.From != "2026-10-05" || rep.To != "2026-10-11" || rep.Period != "week" || rep.GroupBy != "repo" {
		t.Fatalf("period: %+v", rep)
	}
	if len(rep.Tasks) != 6 {
		t.Fatalf("the week has 6 tasks (not the one of last week): %d", len(rep.Tasks))
	}
	if !rep.ScreenAvailable {
		t.Fatal("screen time is usable")
	}

	t1 := taskByID(t, rep, 1)
	if t1.Outcome != "merged" || t1.MergedAt == nil || t1.Runs != 1 || t1.Rounds == nil || *t1.Rounds != 0 {
		t.Fatalf("t1: %+v", t1)
	}
	if t1.AgentName == nil || *t1.AgentName != "Builder" || t1.Title != "修复溢出" {
		t.Fatalf("t1 names: %+v", t1)
	}
	if !near(t1.Cost, 0.5) || t1.CostPartial || t1.InputTokens == nil || *t1.InputTokens != 1000 || *t1.OutputTokens != 200 {
		t.Fatalf("t1 cost: %+v", t1)
	}
	// The build "done" event has no exit code, so it is not a run.
	if t1.RunSeconds == nil || *t1.RunSeconds != 1200 {
		t.Fatalf("t1 run seconds: %v", t1.RunSeconds)
	}
	// Monday 09:20 to 15:00 holds 30 coding minutes and 10 web minutes.
	if !near(t1.ReviewMinutes, 30) || t1.ReviewState != "done" || !near(t1.ReviewPerRun, 1.5) {
		t.Fatalf("t1 review: %+v", t1)
	}

	// T2: two runs, so the run time is the sum of the usage records. The cost
	// is unknown, not zero. 10 coding minutes are shared with T3.
	t2 := taskByID(t, rep, 2)
	if t2.Outcome != "reverted" || t2.Cost != nil || t2.Rounds == nil || *t2.Rounds != 1 {
		t.Fatalf("t2: %+v", t2)
	}
	if t2.RunSeconds == nil || *t2.RunSeconds != 900 || *t2.InputTokens != 1000 {
		t.Fatalf("t2 run: %+v", t2)
	}
	if !near(t2.ReviewMinutes, 5) || t2.ReviewState != "done" {
		t.Fatalf("t2 review: %+v", t2)
	}

	// T3: the review window is cut at 72 hours, so it is done, not ongoing.
	t3 := taskByID(t, rep, 3)
	if t3.Outcome != "waiting" || !t3.CostPartial || !near(t3.Cost, 0.3) {
		t.Fatalf("t3: %+v", t3)
	}
	if !near(t3.ReviewMinutes, 15) || t3.ReviewState != "done" {
		t.Fatalf("t3 review: %+v", t3)
	}
	if t3.RunSeconds == nil || *t3.RunSeconds != 2100 {
		t.Fatalf("t3 run: %v", t3.RunSeconds)
	}

	// T4: a failed task has no review window. It has no usage record.
	t4 := taskByID(t, rep, 4)
	if t4.Outcome != "failed" || t4.Cost != nil || t4.InputTokens != nil || t4.ReviewState != "none" || t4.ReviewMinutes != nil {
		t.Fatalf("t4: %+v", t4)
	}
	if t4.RunSeconds == nil || *t4.RunSeconds != 120 {
		t.Fatalf("t4 run: %v", t4.RunSeconds)
	}

	// T5: never started, so nothing is recorded and nothing is missing.
	t5 := taskByID(t, rep, 5)
	if t5.Outcome != "active" || t5.StartedAt != nil || t5.RunSeconds != nil || t5.Rounds != nil || t5.ReviewState != "none" {
		t.Fatalf("t5: %+v", t5)
	}

	// T6: waiting for the user. No computer time at all in that window.
	t6 := taskByID(t, rep, 6)
	if t6.Outcome != "active" || t6.ReviewState != "no_data" || t6.ReviewMinutes != nil {
		t.Fatalf("t6: %+v", t6)
	}
}

func TestGroupsAndTotals(t *testing.T) {
	r := setup(t)
	r.seed()

	repo := r.report("?date=2026-10-07&groupBy=repo")
	xc := groupByKey(t, repo, "x-console")
	if xc.Tasks != 3 || xc.Merged != 1 || xc.Reverted != 1 || xc.Active != 1 || xc.Rounds != 1 {
		t.Fatalf("x-console: %+v", xc)
	}
	// Merge rate counts only tasks with a result: 1 of 2.
	if !near(xc.MergeRate, 0.5) {
		t.Fatalf("merge rate: %v", xc.MergeRate)
	}
	// Only T1 has a cost. T2 and T6 started and have none.
	if !near(xc.Cost, 0.5) || xc.CostMissing != 2 || !near(xc.CostPerMerge, 0.5) {
		t.Fatalf("x-console cost: %+v", xc)
	}
	if xc.RunSeconds == nil || *xc.RunSeconds != 1200+900+3600 || xc.RunMissing != 0 {
		t.Fatalf("x-console run: %+v", xc)
	}
	if !near(xc.ReviewMinutes, 35) || xc.ReviewMissing != 1 || !near(xc.ReviewMinutesPerMerge, 35) {
		t.Fatalf("x-console review: %+v", xc)
	}
	blog := groupByKey(t, repo, "blog")
	if blog.Tasks != 3 || blog.Waiting != 1 || blog.Failed != 1 || blog.Active != 1 || blog.Merged != 0 {
		t.Fatalf("blog: %+v", blog)
	}
	// Nothing merged: there is no cost per merge. The merge rate is 0 of 1.
	if blog.CostPerMerge != nil || blog.MergeRate == nil || *blog.MergeRate != 0 {
		t.Fatalf("blog ratios: %+v", blog)
	}
	// T4 started and has no cost; T5 never started and is not missing.
	if !near(blog.Cost, 0.3) || blog.CostMissing != 1 {
		t.Fatalf("blog cost: %+v", blog)
	}
	if rep := repo.Totals; rep.Tasks != 6 || rep.Merged != 1 || !near(rep.Cost, 0.8) || rep.Tasks != xc.Tasks+blog.Tasks {
		t.Fatalf("totals: %+v", rep)
	}
	if repo.Groups[0].Label == "" || len(repo.Groups) != 2 {
		t.Fatalf("groups: %+v", repo.Groups)
	}

	tool := r.report("?date=2026-10-07&groupBy=tool")
	claude, codex := groupByKey(t, tool, "claude"), groupByKey(t, tool, "codex")
	if claude.Label != "Claude Code" || claude.Tasks != 4 || codex.Label != "Codex" || codex.Tasks != 2 {
		t.Fatalf("tools: %+v %+v", claude, codex)
	}
	// Codex reports no cost, so none of its tasks has one.
	if codex.Cost != nil || codex.CostMissing != 1 || codex.InputTokens == nil || *codex.InputTokens != 1000 {
		t.Fatalf("codex: %+v", codex)
	}

	agent := r.report("?date=2026-10-07&groupBy=agent")
	builder, manual := groupByKey(t, agent, "1"), groupByKey(t, agent, "")
	if builder.Label != "Builder" || builder.Tasks != 1 || builder.Merged != 1 || manual.Label != "手动任务" || manual.Tasks != 5 {
		t.Fatalf("agents: %+v %+v", builder, manual)
	}
	var sum int
	for _, g := range agent.Groups {
		sum += g.Tasks
	}
	if sum != agent.Totals.Tasks {
		t.Fatalf("groups add up to %d, totals say %d", sum, agent.Totals.Tasks)
	}
}

func TestMonthIncludesEarlierWeek(t *testing.T) {
	r := setup(t)
	r.seed()
	rep := r.report("?period=month&date=2026-10-20")
	if rep.From != "2026-10-01" || rep.To != "2026-10-31" || len(rep.Tasks) != 7 {
		t.Fatalf("month: %s %s %d", rep.From, rep.To, len(rep.Tasks))
	}
	// The default is the week of today.
	if def := r.report(""); def.From != "2026-10-05" {
		t.Fatalf("default week: %s", def.From)
	}
	// An empty week has no rows and no division by zero.
	empty := r.report("?date=2026-08-03")
	if len(empty.Tasks) != 0 || len(empty.Groups) != 0 || empty.Totals.Tasks != 0 || empty.Totals.Cost != nil || empty.Totals.MergeRate != nil {
		t.Fatalf("empty: %+v", empty)
	}
}

func TestBadParameters(t *testing.T) {
	r := setup(t)
	for _, q := range []string{"?period=year", "?groupBy=color", "?date=10-07", "?date=2026-13-40"} {
		if status, _ := r.env.Do(http.MethodGet, "/codingroi/report"+q, nil, nil); status != http.StatusBadRequest {
			t.Fatalf("%s: %d", q, status)
		}
	}
}

func TestCSV(t *testing.T) {
	r := setup(t)
	r.seed()
	resp, err := r.env.Client.Get(r.env.URL("/codingroi/report.csv?date=2026-10-07"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/csv") ||
		!strings.Contains(resp.Header.Get("Content-Disposition"), "coding-roi-2026-10-05-2026-10-11.csv") {
		t.Fatalf("csv %d %v", resp.StatusCode, resp.Header)
	}
	if !strings.HasPrefix(string(raw), "\xef\xbb\xbf") {
		t.Fatal("the file needs a byte order mark for Excel")
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), "\xef\xbb\xbf"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 7 || rows[0][0] != "任务" {
		t.Fatalf("rows: %d %v", len(rows), rows[0])
	}
	byTask := map[string][]string{}
	for _, row := range rows[1:] {
		byTask[row[0]] = row
	}
	head := rows[0]
	col := func(name string) int {
		for i, h := range head {
			if h == name {
				return i
			}
		}
		t.Fatalf("no column %s", name)
		return -1
	}
	t1 := byTask["1"]
	if t1[col("结果")] != "已合并" || t1[col("花费美元")] != "0.5000" || t1[col("审查返工分钟（估算）")] != "30.0" || t1[col("AI 运行分钟")] != "20.0" || t1[col("工具")] != "Claude Code" {
		t.Fatalf("t1 row: %v", t1)
	}
	// A formula in a title must not run in a spreadsheet.
	if got := byTask["2"][col("标题")]; got != "'=SUM(1)" {
		t.Fatalf("title: %q", got)
	}
	// No record is an empty cell, not 0.
	t4 := byTask["4"]
	if t4[col("花费美元")] != "" || t4[col("输入 Token")] != "" || t4[col("审查时间状态")] != "不适用" {
		t.Fatalf("t4 row: %v", t4)
	}
	if byTask["6"][col("审查时间状态")] != "无记录" || byTask["6"][col("审查返工分钟（估算）")] != "" {
		t.Fatalf("t6 row: %v", byTask["6"])
	}
}

func TestOutcomesFromEventAndGitHubCache(t *testing.T) {
	r := setup(t)
	ctx := context.Background()
	r.exec(`INSERT INTO agents (id, name, kind, token_hash, created_at) VALUES ('a1', '台式机', 'desktop', 'h1', ?)`, at(0, 0, 0))
	r.exec(`INSERT INTO coding_repos (id, agent_id, path, name, created_at) VALUES (1, 'a1', '/src/xc', 'x-console', ?)`, at(0, 0, 0))
	for i, url := range []string{"https://github.com/o/r/pull/1", "https://github.com/o/r/pull/2", "https://github.com/o/r/pull/3", "https://github.com/o/r/pull/4"} {
		r.task(taskSeed{id: int64(i + 1), repo: 1, executor: "claude", prompt: "p", status: "pr_opened", prURL: url,
			created: at(0, 9, 0), started: at(0, 9, 0), ended: at(0, 9, 10), runs: 1})
	}
	cache := func(number, state, updated string) {
		r.exec(`INSERT INTO github_cache_v2 (connection_id, repo, kind, object_id, data, updated_at) VALUES (0, 'o/r', 'pull', ?, ?, ?)`,
			number, `{"url":"https://github.com/o/r/pull/`+number+`","state":"`+state+`","updatedAt":"`+updated+`"}`, at(0, 0, 0))
	}
	cache("2", "merged", "2026-10-06T08:00:00Z")
	cache("3", "closed", "2026-10-06T09:00:00Z")
	cache("4", "open", "2026-10-06T10:00:00Z")
	if err := codingroi.SyncPulls(r.m, ctx); err != nil {
		t.Fatal(err)
	}
	// Running it again records nothing new and does not fail.
	if err := codingroi.SyncPulls(r.m, ctx); err != nil {
		t.Fatal(err)
	}

	// The Git webhook announces a merge of task 1 on the event bus.
	r.env.App.Deps.Bus.Publish("coding_task.merged", map[string]any{"taskId": 1, "prUrl": "https://github.com/o/r/pull/1"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		rep := r.report("?date=2026-10-07")
		if taskByID(t, rep, 1).Outcome == "merged" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the merge event was not recorded")
		}
		time.Sleep(20 * time.Millisecond)
	}

	rep := r.report("?date=2026-10-07")
	if o := taskByID(t, rep, 2); o.Outcome != "merged" || o.MergedAt == nil || !o.MergedAt.Equal(time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("t2 from the cache: %+v", o)
	}
	if o := taskByID(t, rep, 3); o.Outcome != "reverted" {
		t.Fatalf("a closed pull request counts as reverted: %+v", o)
	}
	if o := taskByID(t, rep, 4); o.Outcome != "waiting" {
		t.Fatalf("an open pull request is still waiting: %+v", o)
	}

	// A close never replaces a merge, a merge replaces a close.
	if err := codingroi.RecordOutcome(r.m, ctx, 2, "closed", at(1, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if err := codingroi.RecordOutcome(r.m, ctx, 3, "merged", at(1, 0, 0)); err != nil {
		t.Fatal(err)
	}
	rep = r.report("?date=2026-10-07")
	if taskByID(t, rep, 2).Outcome != "merged" || taskByID(t, rep, 3).Outcome != "merged" {
		t.Fatalf("outcomes: %+v %+v", taskByID(t, rep, 2), taskByID(t, rep, 3))
	}
}

func TestReviewTimeAllocation(t *testing.T) {
	base := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	minute := func(i int) int64 { return base.Add(time.Duration(i)*time.Minute).Unix() / 60 }
	var mins [][2]int64
	for i := 0; i < 60; i++ {
		coding := int64(0)
		if i < 30 {
			coding = 1
		}
		mins = append(mins, [2]int64{minute(i), coding})
	}
	win := func(from, to int) [2]time.Time {
		return [2]time.Time{base.Add(time.Duration(from) * time.Minute), base.Add(time.Duration(to) * time.Minute)}
	}

	// Alone in the window: every coding minute counts.
	got := codingroi.AllocateReviewForTest(map[int64][][2]time.Time{1: {win(0, 60)}}, mins)
	if got[1] != [2]float64{30, 1} {
		t.Fatalf("alone: %v", got)
	}
	// Two tasks over the same 20 coding minutes share them. The minutes after
	// minute 20 belong to task 1 only.
	got = codingroi.AllocateReviewForTest(map[int64][][2]time.Time{1: {win(0, 30)}, 2: {win(10, 20)}}, mins)
	if got[1] != [2]float64{25, 1} || got[2] != [2]float64{5, 1} {
		t.Fatalf("shared: %v", got)
	}
	// Windows of one task that overlap are not counted twice.
	got = codingroi.AllocateReviewForTest(map[int64][][2]time.Time{1: {win(0, 20), win(10, 30)}}, mins)
	if got[1] != [2]float64{30, 1} {
		t.Fatalf("overlap: %v", got)
	}
	// A window with computer time but no coding is 0 minutes. A window with
	// no computer time at all has no data.
	got = codingroi.AllocateReviewForTest(map[int64][][2]time.Time{1: {win(40, 50)}, 2: {win(70, 80)}}, mins)
	if got[1] != [2]float64{0, 1} || got[2] != [2]float64{0, 0} {
		t.Fatalf("none: %v", got)
	}
	// The end of a window is not included.
	got = codingroi.AllocateReviewForTest(map[int64][][2]time.Time{1: {win(0, 10)}}, mins)
	if got[1][0] != 10 {
		t.Fatalf("end: %v", got)
	}

	start := base
	if end, cut := codingroi.CapForTest(start, start.Add(10*24*time.Hour)); !cut || !end.Equal(start.Add(72*time.Hour)) {
		t.Fatalf("cap: %v %v", end, cut)
	}
	if end, cut := codingroi.CapForTest(start, start.Add(2*time.Hour)); cut || !end.Equal(start.Add(2*time.Hour)) {
		t.Fatalf("no cap: %v %v", end, cut)
	}
}

func TestOngoingReviewRunsToNow(t *testing.T) {
	r := setup(t)
	r.exec(`INSERT INTO agents (id, name, kind, token_hash, created_at) VALUES ('a1', '台式机', 'desktop', 'h1', ?)`, at(0, 0, 0))
	r.exec(`INSERT INTO coding_repos (id, agent_id, path, name, created_at) VALUES (1, 'a1', '/src/xc', 'x-console', ?)`, at(0, 0, 0))
	// Finished Saturday 10:00, two hours before "now".
	r.task(taskSeed{id: 1, repo: 1, executor: "claude", prompt: "p", status: "review",
		created: at(5, 9, 0), started: at(5, 9, 0), ended: at(5, 10, 0), runs: 1})
	r.screen(at(5, 10, 30), 20, "coding")
	r.screen(at(5, 13, 0), 20, "coding") // after now: must not count
	rep := r.report("?date=2026-10-10")
	task := taskByID(t, rep, 1)
	if task.ReviewState != "ongoing" || !near(task.ReviewMinutes, 20) {
		t.Fatalf("ongoing: %+v", task)
	}
}

func TestActionAndHiddenScreenTime(t *testing.T) {
	r := setup(t)
	r.seed()
	out, err := r.env.App.Deps.Actions.Run(context.Background(), "codingroi.weekly", json.RawMessage(`{"date":"2026-10-07","groupBy":"tool"}`))
	if err != nil {
		t.Fatal(err)
	}
	rep, ok := out.(api.CodingROIReport)
	if !ok || rep.GroupBy != "tool" || rep.Totals.Tasks != 6 || len(rep.Groups) != 2 {
		t.Fatalf("action output: %#v", out)
	}
	if _, err := r.env.App.Deps.Actions.Run(context.Background(), "codingroi.weekly", json.RawMessage(`{"groupBy":"color"}`)); err == nil {
		t.Fatal("a bad group must fail")
	}

	// With the screen time module hidden and locked, no review time is shown.
	env := r.env
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, nil)
	env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"screentime"}}, nil)
	if got := r.report("?date=2026-10-07"); !got.ScreenAvailable || taskByID(t, got, 1).ReviewMinutes == nil {
		t.Fatal("unlocked: review time is shown")
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	locked := r.report("?date=2026-10-07")
	if locked.ScreenAvailable || taskByID(t, locked, 1).ReviewMinutes != nil || taskByID(t, locked, 1).ReviewState != "no_data" {
		t.Fatalf("locked: %+v", taskByID(t, locked, 1))
	}
	// Hiding the whole coding area hides the report.
	env.MustDo(http.MethodPost, "/vault/unlock", map[string]string{"password": "secret-one"}, nil)
	env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"coding"}}, nil)
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	if status, _ := env.Do(http.MethodGet, "/codingroi/report", nil, nil); status != http.StatusNotFound {
		t.Fatalf("hidden coding: %d", status)
	}
}
