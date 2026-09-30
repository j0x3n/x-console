package projects_test

import (
	"fmt"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
)

func TestChecklistsAndConvert(t *testing.T) {
	env := newEnv(t)
	p := createProject(t, env, "CHK")
	issue := createIssue(t, env, p.Id, map[string]any{"title": "主任务"})
	base := "/issues/" + issue.Key + "/checklists"
	var first, second api.Checklist
	env.MustDo("POST", base, map[string]any{"title": " 第一组 "}, &first)
	env.MustDo("POST", base, map[string]any{"title": "第二组"}, &second)
	if first.Title != "第一组" {
		t.Fatalf("title: %+v", first)
	}
	var one, two, three api.ChecklistItem
	env.MustDo("POST", fmt.Sprintf("%s/%d/items", base, first.Id), map[string]any{"text": "任务一"}, &one)
	env.MustDo("POST", fmt.Sprintf("%s/%d/items", base, first.Id), map[string]any{"text": "任务二"}, &two)
	env.MustDo("POST", fmt.Sprintf("%s/%d/items", base, first.Id), map[string]any{"text": "任务三", "afterId": one.Id}, &three)
	var lists []api.Checklist
	env.MustDo("GET", base, nil, &lists)
	if len(lists) != 2 || len(lists[0].Items) != 3 || lists[0].Items[0].Id != one.Id || lists[0].Items[1].Id != three.Id || lists[0].Items[2].Id != two.Id {
		t.Fatalf("items: %+v", lists)
	}
	env.MustDo("PATCH", fmt.Sprintf("/issues/%s/checklist-items/%d", issue.Key, one.Id), map[string]any{"done": true}, &one)
	if !one.Done || one.DoneAt == nil {
		t.Fatalf("done: %+v", one)
	}
	var detail api.Issue
	env.MustDo("GET", "/issues/"+issue.Key, nil, &detail)
	if detail.ChecklistDone == nil || *detail.ChecklistDone != 1 || detail.ChecklistTotal == nil || *detail.ChecklistTotal != 3 {
		t.Fatalf("progress: %+v", detail)
	}
	oneID := one.Id
	one = api.ChecklistItem{}
	env.MustDo("PATCH", fmt.Sprintf("/issues/%s/checklist-items/%d", issue.Key, oneID), map[string]any{"done": false, "checklistId": second.Id}, &one)
	if one.Done || one.DoneAt != nil || one.ChecklistId != second.Id {
		t.Fatalf("move: %+v", one)
	}
	env.MustDo("PATCH", fmt.Sprintf("%s/%d", base, second.Id), map[string]any{"title": "改名"}, &second)
	if second.Title != "改名" {
		t.Fatalf("rename: %+v", second)
	}
	other := createIssue(t, env, p.Id, map[string]any{"title": "别的 Issue"})
	if status, _ := env.Do("PATCH", fmt.Sprintf("/issues/%s/checklist-items/%d", other.Key, one.Id), map[string]any{"text": "越权"}, nil); status != 404 {
		t.Fatalf("cross issue: %d", status)
	}
	var converted api.Issue
	env.MustDo("POST", fmt.Sprintf("/issues/%s/checklist-items/%d/convert", issue.Key, two.Id), map[string]any{}, &converted)
	if converted.Title != "任务二" || converted.ProjectId != p.Id || converted.Status != "todo" {
		t.Fatalf("converted: %+v", converted)
	}
	env.MustDo("GET", base, nil, &lists)
	if len(lists[0].Items) != 1 || lists[0].Items[0].Id != three.Id {
		t.Fatalf("after conversion: %+v", lists)
	}
	if status, _ := env.Do("DELETE", fmt.Sprintf("/issues/%s/checklist-items/%d", issue.Key, three.Id), nil, nil); status != 204 {
		t.Fatalf("delete item: %d", status)
	}
	if status, _ := env.Do("DELETE", fmt.Sprintf("%s/%d", base, second.Id), nil, nil); status != 204 {
		t.Fatalf("delete list: %d", status)
	}
	env.MustDo("GET", base, nil, &lists)
	if len(lists) != 1 || len(lists[0].Items) != 0 {
		t.Fatalf("remaining: %+v", lists)
	}
}
