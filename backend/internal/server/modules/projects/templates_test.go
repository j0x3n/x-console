package projects_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
)

// B204: the indie game template fills a new project.
func TestIndieGameTemplate(t *testing.T) {
	env := newEnv(t)
	bad := api.CreateProjectTemplate("rpg")
	if status, _ := env.Do(http.MethodPost, "/projects", api.CreateProject{Key: "BAD", Name: "坏模板", Template: &bad}, nil); status != 400 {
		t.Fatalf("unknown template: %d", status)
	}
	var none []api.Project
	env.MustDo(http.MethodGet, "/projects", nil, &none)
	if len(none) != 0 {
		t.Fatalf("a project was left behind: %+v", none)
	}

	tpl := api.IndieGame
	var p api.Project
	env.MustDo(http.MethodPost, "/projects", api.CreateProject{Key: "GAME", Name: "我的游戏", Template: &tpl}, &p)
	if !strings.Contains(p.Description, "先验证再投入") || p.IssueCount != 10 {
		t.Fatalf("project: %+v", p)
	}
	var milestones []api.Milestone
	env.MustDo(http.MethodGet, fmt.Sprintf("/projects/%d/milestones", p.Id), nil, &milestones)
	if len(milestones) != 6 || milestones[0].Name != "1 概念验证" || milestones[5].Name != "6 正式发售" {
		t.Fatalf("milestones: %+v", milestones)
	}
	var labels []api.Label
	env.MustDo(http.MethodGet, fmt.Sprintf("/projects/%d/labels", p.Id), nil, &labels)
	if len(labels) < 8 {
		t.Fatalf("labels: %+v", labels)
	}
	issues, _ := listIssues(t, env, "project=GAME&limit=50")
	var doc api.Issue
	for _, i := range issues {
		if i.Title == "写一页设计文档" {
			doc = i
		}
	}
	if doc.Key == "" || doc.Status != api.IssueStatusTodo || doc.MilestoneId == nil || *doc.MilestoneId != milestones[0].Id || len(doc.Labels) != 1 {
		t.Fatalf("design doc card: %+v", doc)
	}
	var lists []api.Checklist
	env.MustDo(http.MethodGet, "/issues/"+doc.Key+"/checklists", nil, &lists)
	if len(lists) != 1 || lists[0].Title != "要写清楚" || len(lists[0].Items) != 6 || lists[0].Items[0].Text != "一句话说清这是什么游戏" {
		t.Fatalf("checklist: %+v", lists)
	}

	// A description given by the user is kept.
	desc := "自己的说明"
	var own api.Project
	env.MustDo(http.MethodPost, "/projects", api.CreateProject{Key: "MINE", Name: "第二个", Description: &desc, Template: &tpl}, &own)
	if own.Description != desc {
		t.Fatalf("description: %q", own.Description)
	}
}
