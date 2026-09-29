package projects_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/app"
	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
	"github.com/j0x3n/x-console/backend/internal/server/store"
)

func TestIssueCategoryDueFieldsAndLegacyDate(t *testing.T) {
	env := newEnv(t)
	project := createProject(t, env, "DUE")
	other := createProject(t, env, "OTH")
	var parent, child, foreign api.ProjectCategory
	env.MustDo("POST", fmt.Sprintf("/projects/%d/categories", project.Id), map[string]any{"name": "后端"}, &parent)
	env.MustDo("POST", fmt.Sprintf("/projects/%d/categories", project.Id), map[string]any{"name": "服务器", "parentId": parent.Id}, &child)
	env.MustDo("POST", fmt.Sprintf("/projects/%d/categories", other.Id), map[string]any{"name": "其他"}, &foreign)
	now := time.Now().UTC().Truncate(time.Minute)
	due := now.Add(2 * time.Hour)
	issue := createIssue(t, env, project.Id, map[string]any{"title": "新版截止", "categoryId": child.Id, "dueAt": due.Format(time.RFC3339), "dueRemind": "1h"})
	if issue.CategoryId == nil || *issue.CategoryId != child.Id || issue.DueAt == nil || !issue.DueAt.Equal(due) || issue.DueRemind == nil || *issue.DueRemind != "1h" {
		t.Fatalf("issue: %+v", issue)
	}
	if issue.ChecklistDone == nil || *issue.ChecklistDone != 0 || issue.ChecklistTotal == nil || *issue.ChecklistTotal != 0 {
		t.Fatalf("progress: %+v", issue)
	}
	items, _ := listIssues(t, env, fmt.Sprintf("projectId=%d&categoryId=%d", project.Id, parent.Id))
	if len(items) != 1 || items[0].Id != issue.Id {
		t.Fatalf("parent filter: %+v", items)
	}
	if status, raw := env.Do("PATCH", "/issues/"+issue.Key, map[string]any{"categoryId": foreign.Id}, nil); status != 400 || errCode(t, raw) != "invalid_category" {
		t.Fatalf("invalid category: %d %s", status, raw)
	}
	var changed api.Issue
	env.MustDo("PATCH", "/issues/"+issue.Key, map[string]any{"categoryId": nil, "dueAt": nil}, &changed)
	if changed.CategoryId != nil || changed.DueAt != nil || changed.DueDate != nil {
		t.Fatalf("cleared: %+v", changed)
	}
	items, _ = listIssues(t, env, fmt.Sprintf("projectId=%d&categoryId=0", project.Id))
	if len(items) != 1 || items[0].Id != issue.Id {
		t.Fatalf("uncategorized: %+v", items)
	}
	legacyDate := now.In(env.App.Deps.Config.Location).Format("2006-01-02")
	legacy := createIssue(t, env, project.Id, map[string]any{"title": "旧日期", "dueDate": legacyDate})
	expect, _ := time.ParseInLocation("2006-01-02 15:04", legacyDate+" 23:59", env.App.Deps.Config.Location)
	if legacy.DueAt == nil || !legacy.DueAt.Equal(expect.UTC()) || legacy.DueRemind == nil || *legacy.DueRemind != "at_due" {
		t.Fatalf("legacy: %+v", legacy)
	}
}

func TestLegacyDueDateBackfilledAtStartup(t *testing.T) {
	ctx := context.Background()
	conn, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	now := time.Now().UTC()
	if _, err = conn.ExecContext(ctx, "INSERT INTO projects(key,name,created_at,updated_at) VALUES('OLD','旧项目',?,?)", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, "INSERT INTO issues(project_id,number,title,due_date,created_at,updated_at) VALUES(1,1,'旧 Issue','2026-10-01',?,?)", now, now); err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	cfg := config.Config{MasterKey: bytes.Repeat([]byte{7}, 32), Dev: true, Location: loc, DataDir: t.TempDir()}
	if _, err = app.New(cfg, conn); err != nil {
		t.Fatal(err)
	}
	var dueAt string
	if err = conn.QueryRowContext(ctx, "SELECT due_at FROM issues WHERE project_id=1").Scan(&dueAt); err != nil {
		t.Fatal(err)
	}
	expect := time.Date(2026, 10, 1, 23, 59, 0, 0, loc).UTC().Format(time.RFC3339)
	if dueAt != expect {
		t.Fatalf("due_at=%q want %q", dueAt, expect)
	}
}

func TestIssueDueNotificationOnceAndReschedule(t *testing.T) {
	env := newEnv(t)
	project := createProject(t, env, "NTO")
	now := time.Now().UTC().Truncate(time.Second)
	issue := createIssue(t, env, project.Id, map[string]any{"title": "到期测试", "dueAt": now.Add(2 * time.Minute).Format(time.RFC3339), "dueRemind": "15m"})
	count := func() int {
		var n int
		if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM notifications WHERE kind='issue.due'").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := projects.RunDueForTest(env.App.Deps, now); err != nil {
		t.Fatal(err)
	}
	if err := projects.RunDueForTest(env.App.Deps, now); err != nil {
		t.Fatal(err)
	}
	if count() != 1 {
		t.Fatalf("duplicate notification: %d", count())
	}
	var link string
	if err := env.App.Deps.DB.QueryRow("SELECT link FROM notifications WHERE kind='issue.due' LIMIT 1").Scan(&link); err != nil || link != "/projects/NTO/1" {
		t.Fatalf("link=%q err=%v", link, err)
	}
	env.MustDo("PATCH", "/issues/"+issue.Key, map[string]any{"dueAt": now.Add(5 * time.Minute).Format(time.RFC3339)}, nil)
	if err := projects.RunDueForTest(env.App.Deps, now); err != nil {
		t.Fatal(err)
	}
	if count() != 2 {
		t.Fatalf("reschedule notifications: %d", count())
	}
	env.MustDo("PATCH", "/issues/"+issue.Key, map[string]any{"dueRemind": "none"}, nil)
	if err := projects.RunDueForTest(env.App.Deps, now); err != nil {
		t.Fatal(err)
	}
	if count() != 2 {
		t.Fatalf("disabled notifications: %d", count())
	}
}
