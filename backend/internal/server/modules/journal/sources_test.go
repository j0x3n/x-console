package journal_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/journal"
	"github.com/j0x3n/x-console/backend/internal/server/modules/journal/api"
)

func (r *rig) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := r.env.App.Deps.DB.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func byKind(items []api.JournalItem, kind api.JournalKind) []api.JournalItem {
	var out []api.JournalItem
	for _, it := range items {
		if it.Kind == kind {
			out = append(out, it)
		}
	}
	return out
}

// 每个模块真正的来源：造数据，采集，看当天的时间线。
func TestModulesFeedTheTimeline(t *testing.T) {
	r := setup(t)
	env := r.env
	ctx := context.Background()
	r.src.set(nil, nil)

	// 项目：完成的卡片算，没完成的不算
	var project struct{ Id int64 }
	env.MustDo(http.MethodPost, "/projects", map[string]any{"key": "XC", "name": "X Console"}, &project)
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/issues", project.Id), map[string]any{"title": "登录页改版"}, nil)
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/issues", project.Id), map[string]any{"title": "还没做"}, nil)
	env.MustDo(http.MethodPatch, "/issues/XC-1", map[string]any{"status": "done"}, nil)

	// 笔记
	env.MustDo(http.MethodPost, "/notes", map[string]any{"title": "周会记录", "body": "内容"}, nil)

	// 习惯：同一习惯一天合成一条，训练记录单独一条
	var habit struct{ Id int64 }
	env.MustDo(http.MethodPost, "/habits", map[string]any{"name": "喝水", "unit": "杯", "dailyTarget": 8}, &habit)
	env.MustDo(http.MethodPost, fmt.Sprintf("/habits/%d/checkin", habit.Id), nil, nil)
	env.MustDo(http.MethodPost, fmt.Sprintf("/habits/%d/checkin", habit.Id), map[string]any{"amount": 2}, nil)
	env.MustDo(http.MethodPost, "/workouts/logs", map[string]any{"durationMinutes": 30, "items": []map[string]any{{"name": "深蹲"}, {"name": "卧推"}}}, nil)

	// 专注：不到一分钟的不算
	var session struct{ Id int64 }
	env.MustDo(http.MethodPost, "/focus/start", map[string]any{"minutes": 25}, &session)
	env.MustDo(http.MethodPost, fmt.Sprintf("/focus/%d/stop", session.Id), map[string]any{}, nil)
	var short struct{ Id int64 }
	env.MustDo(http.MethodPost, "/focus/start", map[string]any{"minutes": 25}, &short)
	env.MustDo(http.MethodPost, fmt.Sprintf("/focus/%d/stop", short.Id), map[string]any{}, nil)
	r.exec(t, `UPDATE focus_sessions SET actual_seconds = 1500, completed = 1 WHERE id = ?`, session.Id)
	r.exec(t, `UPDATE focus_sessions SET actual_seconds = 20 WHERE id = ?`, short.Id)

	// 稍后阅读
	var item struct {
		Item struct{ Id int64 }
	}
	env.MustDo(http.MethodPost, "/readlater", map[string]any{"url": "https://example.com/articles/a"}, &item)
	env.MustDo(http.MethodPatch, fmt.Sprintf("/readlater/%d", item.Item.Id), map[string]any{"read": true}, nil)

	// 电脑时间：120 分钟，90 编码 + 30 网页
	start := r.at(0, 0, 1).Unix() / 60
	for i := int64(0); i < 120; i++ {
		category, app := "coding", "Code.exe"
		if i >= 90 {
			category, app = "web", "chrome.exe"
		}
		r.exec(t, `INSERT INTO screen_minutes (host_id, minute, app, category, title) VALUES ('pc1', ?, ?, ?, '')`, start+i, app, category)
	}

	// Agent 任务
	r.exec(t, `INSERT INTO agents (id, name, kind, token_hash, created_at) VALUES ('a1', '台式机', 'desktop', 'h1', ?)`, time.Now().UTC())
	r.exec(t, `INSERT INTO coding_repos (id, agent_id, path, name, created_at) VALUES (1, 'a1', '/src/xc', 'x-console', ?)`, time.Now().UTC())
	finished := r.at(0, 0, 30).UTC()
	r.exec(t, `INSERT INTO coding_tasks (id, repo_id, executor, prompt, status, commit_sha, pr_url, created_at, updated_at, finished_at)
		VALUES (7, 1, 'claude', ?, 'pr_opened', 'abcdef1234567', 'https://github.com/o/r/pull/3', ?, ?, ?)`, "修复移动端溢出\n详细说明", finished, finished, finished)
	r.exec(t, `INSERT INTO coding_tasks (id, repo_id, executor, prompt, status, created_at, updated_at, finished_at)
		VALUES (8, 1, 'codex', '还在跑的', 'running', ?, ?, NULL)`, finished, finished)

	// GitHub：自己的提交和 PR 算，别人的不算
	if err := env.App.Deps.Settings.Set(ctx, "github.login", "me"); err != nil {
		t.Fatal(err)
	}
	if err := env.App.Deps.Settings.Set(ctx, "github.watches", []map[string]any{{"connectionId": 0, "repo": "me/app"}}); err != nil {
		t.Fatal(err)
	}
	cache := func(kind, id, data string) {
		r.exec(t, `INSERT INTO github_cache_v2 (connection_id, repo, kind, object_id, data, updated_at) VALUES (0, 'me/app', ?, ?, ?, ?)`, kind, id, data, time.Now().UTC())
	}
	stamp := func(h, m int) string { return r.at(0, h, m).UTC().Format(time.RFC3339) }
	cache("commit", "c1", fmt.Sprintf(`{"sha":"1111111aaaa","author":"Me","committedAt":%q,"message":"修复导航栏\n\n正文","repo":"me/app","url":"https://github.com/me/app/commit/1111111aaaa"}`, stamp(0, 20)))
	cache("commit", "c2", fmt.Sprintf(`{"sha":"2222222bbbb","author":"someone","committedAt":%q,"message":"别人的提交","repo":"me/app","url":"u"}`, stamp(0, 21)))
	cache("commit", "c3", fmt.Sprintf(`{"sha":"3333333cccc","author":"me","committedAt":%q,"message":"昨天的提交","repo":"me/app","url":"u"}`, r.at(-1, 12, 0).UTC().Format(time.RFC3339)))
	cache("pull", "5", fmt.Sprintf(`{"number":5,"title":"加搜索","author":"me","url":"https://github.com/me/app/pull/5","state":"merged","createdAt":%q,"updatedAt":%q,"headSha":"x"}`, stamp(0, 5), stamp(0, 40)))
	cache("pull", "6", fmt.Sprintf(`{"number":6,"title":"别人的 PR","author":"someone","url":"u","state":"open","createdAt":%q,"updatedAt":%q,"headSha":"y"}`, stamp(0, 6), stamp(0, 6)))

	journal.CollectRecent(r.m, ctx)
	d := r.getDay(t, r.date(0))

	if cards := byKind(d.Items, api.Card); len(cards) != 1 || cards[0].Title != "完成 XC-1 登录页改版" || cards[0].Link != "/projects/XC/1" || cards[0].Module != "projects" {
		t.Fatalf("cards: %+v", cards)
	}
	if notes := byKind(d.Items, api.Note); len(notes) != 1 || notes[0].Title != "新建笔记：周会记录" || !strings.HasPrefix(notes[0].Link, "/notes/") {
		t.Fatalf("notes: %+v", notes)
	}
	habits := byKind(d.Items, api.Habit)
	if len(habits) != 1 || habits[0].Title != "习惯打卡：喝水" || habits[0].Detail != "打卡 2 次，共 3 杯" {
		t.Fatalf("habits: %+v", habits)
	}
	if w := byKind(d.Items, api.Workout); len(w) != 1 || w[0].Title != "训练 30 分钟" || w[0].Detail != "深蹲、卧推" || w[0].Minutes != 30 {
		t.Fatalf("workouts: %+v", w)
	}
	focus := byKind(d.Items, api.Focus)
	if len(focus) != 1 || focus[0].Title != "专注 25 分钟" || focus[0].Minutes != 25 || focus[0].Module != "calendar" {
		t.Fatalf("focus: %+v", focus)
	}
	links := byKind(d.Items, api.Link)
	if len(links) != 2 || !strings.HasPrefix(links[0].Title, "存了链接：") || !strings.HasPrefix(links[1].Title, "读完：") {
		t.Fatalf("links: %+v", links)
	}
	screen := byKind(d.Items, api.Screen)
	if len(screen) != 1 || screen[0].Title != "电脑用了 2 小时" || screen[0].Detail != "编码 1 小时 30 分 · 网页 30 分钟" || screen[0].Minutes != 120 || screen[0].Module != "screentime" {
		t.Fatalf("screen: %+v", screen)
	}
	tasks := byKind(d.Items, api.Task)
	if len(tasks) != 1 || tasks[0].Title != "Claude 跑完：修复移动端溢出" || tasks[0].Detail != "x-console · 已开 PR · 提交 abcdef1 · 有 PR" || tasks[0].Link != "/coding/7" {
		t.Fatalf("tasks: %+v", tasks)
	}
	commits := byKind(d.Items, api.Commit)
	if len(commits) != 1 || commits[0].Title != "修复导航栏" || commits[0].Detail != "me/app · 1111111" || commits[0].Module != "github" {
		t.Fatalf("commits: %+v", commits)
	}
	prs := byKind(d.Items, api.Pr)
	if len(prs) != 2 || prs[0].Title != "开了 PR #5 加搜索" || prs[1].Title != "合并了 PR #5 加搜索" {
		t.Fatalf("prs: %+v", prs)
	}
	// 昨天的提交在昨天
	if y := byKind(r.getDay(t, r.date(-1)).Items, api.Commit); len(y) != 1 || y[0].Title != "昨天的提交" {
		t.Fatalf("yesterday's commits: %+v", y)
	}
	// 采集第二次不会重复
	journal.CollectRecent(r.m, ctx)
	if again := r.getDay(t, r.date(0)); len(again.Items) != len(d.Items) {
		t.Fatalf("collecting twice changed the count: %d -> %d", len(d.Items), len(again.Items))
	}
	// 概要里的数字：专注 25 分钟，电脑 120 分钟
	for _, c := range d.Counts {
		switch c.Kind {
		case api.Focus:
			if c.Count != 1 || c.Minutes != 25 {
				t.Fatalf("focus count: %+v", c)
			}
		case api.Screen:
			if c.Minutes != 120 {
				t.Fatalf("screen count: %+v", c)
			}
		}
	}
}
