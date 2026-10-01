package actions_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestAssistantActionWorkflows(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	run := func(name string, input map[string]any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(input)
		result, err := env.App.Deps.Actions.Run(ctx, name, raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		encoded, _ := json.Marshal(result)
		var out map[string]any
		_ = json.Unmarshal(encoded, &out)
		return out
	}
	if p := run("projects.create", map[string]any{"key": "ACT", "name": "动作测试"}); p["key"] != "ACT" {
		t.Fatalf("project: %v", p)
	}
	if p := run("projects.get", map[string]any{"key": "ACT"}); p["name"] != "动作测试" {
		t.Fatalf("project get: %v", p)
	}
	run("projects.update", map[string]any{"key": "ACT", "name": "改名"})
	run("milestones.create", map[string]any{"projectKey": "ACT", "name": "一期", "dueDate": "2026-10-01"})
	issue := run("issues.create", map[string]any{"projectKey": "ACT", "title": "动作 Issue"})
	key, ok := issue["key"].(string)
	if !ok {
		t.Fatalf("issue: %v", issue)
	}
	if got := run("issues.get", map[string]any{"key": key}); got["issue"] == nil {
		t.Fatalf("get issue: %v", got)
	}
	run("issues.list", map[string]any{"projectKey": "ACT"})
	run("issues.update", map[string]any{"key": key, "title": "更新 Issue"})
	run("issues.move", map[string]any{"key": key, "status": "in_progress"})
	run("issues.comment", map[string]any{"key": key, "body": "进度正常"})
	run("issues.delete", map[string]any{"key": key})
	run("projects.archive", map[string]any{"key": "ACT"})
	note := run("notes.create", map[string]any{"title": "测试笔记", "body": "原文"})
	noteID := int64(note["id"].(float64))
	if got := run("notes.get", map[string]any{"id": noteID}); got["body"] != "原文" {
		t.Fatalf("note: %v", got)
	}
	run("notes.search", map[string]any{"q": "测试笔记"})
	run("notes.update", map[string]any{"id": noteID, "body": "正文"})
	run("notes.append", map[string]any{"id": noteID, "text": "追加"})
	run("notes.delete", map[string]any{"id": noteID})
	at := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	reminder := run("reminders.create", map[string]any{"title": "测试提醒", "at": at})
	reminderID := int64(reminder["id"].(float64))
	run("reminders.list", map[string]any{"range": "upcoming"})
	run("reminders.update", map[string]any{"id": reminderID, "title": "更新提醒"})
	run("reminders.done", map[string]any{"id": reminderID})
	run("reminders.delete", map[string]any{"id": reminderID})
	habit := run("habits.create", map[string]any{"name": "动作习惯", "dailyTarget": 1.0})
	habitID := int64(habit["id"].(float64))
	run("habits.today", map[string]any{})
	run("habits.stats", map[string]any{"id": habitID, "days": 7})
	run("habits.checkin", map[string]any{"habitId": habitID})
	run("habits.delete", map[string]any{"id": habitID})
	for _, name := range []string{"projects.get", "issues.move", "notes.get", "reminders.update", "habits.stats"} {
		if _, ok := env.App.Deps.Actions.Get(ctx, name); !ok {
			t.Fatal(fmt.Sprintf("missing %s", name))
		}
	}
}
