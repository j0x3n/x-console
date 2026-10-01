package projects_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// newEnv starts a server. The projects module is registered in
// app/modules.go, so it is not passed again (building it twice would register
// its actions twice).
func newEnv(t *testing.T) *testutil.Env {
	t.Helper()
	return testutil.New(t)
}

func createProject(t *testing.T, env *testutil.Env, key string) api.Project {
	t.Helper()
	var p api.Project
	env.MustDo(http.MethodPost, "/projects", api.CreateProject{Key: key, Name: "Project " + key}, &p)
	return p
}

func createIssue(t *testing.T, env *testutil.Env, projectID int64, body any) api.Issue {
	t.Helper()
	var issue api.Issue
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/issues", projectID), body, &issue)
	return issue
}

func errCode(t *testing.T, raw []byte) string {
	t.Helper()
	var e struct{ Code string }
	_ = json.Unmarshal(raw, &e)
	return e.Code
}

func listIssues(t *testing.T, env *testutil.Env, query string) ([]api.Issue, string) {
	t.Helper()
	var out struct {
		Items      []api.Issue
		NextCursor string
	}
	env.MustDo(http.MethodGet, "/issues?"+query, nil, &out)
	return out.Items, out.NextCursor
}

func keys(list []api.Issue) string {
	var ks []string
	for _, i := range list {
		ks = append(ks, i.Key)
	}
	return strings.Join(ks, ",")
}

func TestProjectsCRUD(t *testing.T) {
	env := newEnv(t)
	p := createProject(t, env, "XC")
	if p.Key != "XC" || p.IssueCount != 0 {
		t.Fatalf("created: %+v", p)
	}
	// Validation: key must be 2-5 capital letters, name required.
	if status, raw := env.Do(http.MethodPost, "/projects", api.CreateProject{Key: "x1", Name: "bad"}, nil); status != 400 || errCode(t, raw) != "validation_failed" {
		t.Fatalf("bad key: %d %s", status, raw)
	}
	if status, _ := env.Do(http.MethodPost, "/projects", api.CreateProject{Key: "XC", Name: "dup"}, nil); status != 409 {
		t.Fatalf("duplicate key: %d", status)
	}
	if status, _ := env.Do(http.MethodGet, "/projects/999", nil, nil); status != 404 {
		t.Fatalf("missing project: %d", status)
	}
	name := "X Console"
	env.MustDo(http.MethodPatch, fmt.Sprintf("/projects/%d", p.Id), api.UpdateProject{Name: &name}, &p)
	if p.Name != name {
		t.Fatalf("rename: %+v", p)
	}
	createProject(t, env, "HOME")
	var list []api.Project
	env.MustDo(http.MethodGet, "/projects", nil, &list)
	if len(list) != 2 {
		t.Fatalf("list: %+v", list)
	}
	// DELETE archives; the project moves to the archived list and can come back.
	env.MustDo(http.MethodDelete, fmt.Sprintf("/projects/%d", p.Id), nil, nil)
	env.MustDo(http.MethodGet, "/projects", nil, &list)
	if len(list) != 1 || list[0].Key != "HOME" {
		t.Fatalf("after archive: %+v", list)
	}
	env.MustDo(http.MethodGet, "/projects?archived=true", nil, &list)
	if len(list) != 1 || list[0].ArchivedAt == nil {
		t.Fatalf("archived list: %+v", list)
	}
	if status, _ := env.Do(http.MethodPost, fmt.Sprintf("/projects/%d/issues", p.Id), api.CreateIssue{Title: "x"}, nil); status != 400 {
		t.Fatalf("issue in archived project: %d", status)
	}
	unarchive := false
	env.MustDo(http.MethodPatch, fmt.Sprintf("/projects/%d", p.Id), api.UpdateProject{Archived: &unarchive}, &p)
	if p.ArchivedAt != nil {
		t.Fatalf("unarchive: %+v", p)
	}
}

func TestIssueKeysAndUpdates(t *testing.T) {
	env := newEnv(t)
	ch, cancel := env.App.Deps.Bus.Subscribe("issue.", 64)
	defer cancel()
	p := createProject(t, env, "XC")
	a := createIssue(t, env, p.Id, api.CreateIssue{Title: "First"})
	b := createIssue(t, env, p.Id, api.CreateIssue{Title: "Second"})
	if a.Key != "XC-1" || b.Key != "XC-2" || a.Status != "todo" {
		t.Fatalf("keys: %s %s %s", a.Key, b.Key, a.Status)
	}
	// Deleted numbers are never reused.
	env.MustDo(http.MethodDelete, "/issues/XC-2", nil, nil)
	if status, _ := env.Do(http.MethodGet, "/issues/XC-2", nil, nil); status != 404 {
		t.Fatalf("deleted issue: %d", status)
	}
	c := createIssue(t, env, p.Id, api.CreateIssue{Title: "Third"})
	if c.Key != "XC-3" {
		t.Fatalf("after delete: %s", c.Key)
	}
	// Keys are case-insensitive; unknown keys are 404.
	var got api.Issue
	env.MustDo(http.MethodGet, "/issues/xc-1", nil, &got)
	if got.Title != "First" {
		t.Fatalf("get: %+v", got)
	}
	for _, key := range []string{"XC-99", "NOPE-1", "garbage"} {
		if status, _ := env.Do(http.MethodGet, "/issues/"+key, nil, nil); status != 404 {
			t.Fatalf("%s: %d", key, status)
		}
	}
	if status, raw := env.Do(http.MethodPost, fmt.Sprintf("/projects/%d/issues", p.Id), api.CreateIssue{Title: "  "}, nil); status != 400 {
		t.Fatalf("empty title: %d %s", status, raw)
	}
	if status, _ := env.Do(http.MethodPost, fmt.Sprintf("/projects/%d/issues", p.Id), map[string]any{"title": "x", "priority": 9}, nil); status != 400 {
		t.Fatalf("bad priority: %d", status)
	}

	// Labels and milestones.
	var label api.Label
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/labels", p.Id), api.CreateLabel{Name: "bug"}, &label)
	var ms api.Milestone
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/milestones", p.Id), map[string]any{"name": "v1", "dueDate": "2026-12-31"}, &ms)
	other := createProject(t, env, "OT")
	var foreign api.Label
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/labels", other.Id), api.CreateLabel{Name: "theirs"}, &foreign)
	if status, _ := env.Do(http.MethodPatch, "/issues/XC-1", map[string]any{"labelIds": []int64{foreign.Id}}, nil); status != 400 {
		t.Fatalf("foreign label: %d", status)
	}

	// PATCH any field; done sets completedAt and emits status_changed.
	env.MustDo(http.MethodPatch, "/issues/XC-1", map[string]any{
		"title": "First!", "description": "# Hi", "priority": 1, "dueDate": "2026-10-01",
		"milestoneId": ms.Id, "labelIds": []int64{label.Id}, "status": "done",
	}, &got)
	if got.Title != "First!" || got.Priority != 1 || got.DueDate == nil || got.DueDate.String() != "2026-10-01" ||
		got.MilestoneId == nil || len(got.Labels) != 1 || got.Status != "done" || got.CompletedAt == nil {
		t.Fatalf("patch: %+v", got)
	}
	// null clears nullable fields.
	got = api.Issue{}
	env.MustDo(http.MethodPatch, "/issues/XC-1", map[string]any{"dueDate": nil, "milestoneId": nil, "labelIds": []int64{}, "status": "todo"}, &got)
	if got.DueDate != nil || got.MilestoneId != nil || len(got.Labels) != 0 || got.CompletedAt != nil {
		t.Fatalf("clear: %+v", got)
	}
	if status, _ := env.Do(http.MethodPatch, "/issues/XC-1", map[string]any{"status": "nope"}, nil); status != 400 {
		t.Fatalf("bad status: %d", status)
	}

	seen := map[string]bool{}
	timeout := time.After(time.Second)
	for !seen["issue.status_changed"] || !seen["issue.deleted"] || !seen["issue.created"] || !seen["issue.updated"] {
		select {
		case ev := <-ch:
			seen[ev.Topic] = true
			if ev.Topic == "issue.status_changed" {
				raw, _ := json.Marshal(ev.Data)
				if !strings.Contains(string(raw), `"to":"done"`) && !strings.Contains(string(raw), `"to":"todo"`) {
					t.Fatalf("status_changed payload: %s", raw)
				}
			}
		case <-timeout:
			t.Fatalf("events seen: %v", seen)
		}
	}
}

func TestBoardMove(t *testing.T) {
	env := newEnv(t)
	p := createProject(t, env, "XC")
	for _, title := range []string{"c", "b", "a"} { // new issues go on top, so the column reads a, b, c
		createIssue(t, env, p.Id, api.CreateIssue{Title: title})
	}
	q := fmt.Sprintf("projectId=%d&status=todo&sort=manual", p.Id)
	if list, _ := listIssues(t, env, q); keys(list) != "XC-3,XC-2,XC-1" {
		t.Fatalf("initial: %s", keys(list))
	}
	// Move c (XC-1) between a (XC-3) and b (XC-2).
	after, before := "XC-3", "XC-2"
	env.MustDo(http.MethodPost, "/issues/XC-1/move", api.MoveIssue{Status: st("todo"), AfterKey: &after, BeforeKey: &before}, nil)
	if list, _ := listIssues(t, env, q); keys(list) != "XC-3,XC-1,XC-2" {
		t.Fatalf("after move: %s", keys(list))
	}
	// Only afterKey: the server finds the next neighbour itself.
	env.MustDo(http.MethodPost, "/issues/XC-2/move", api.MoveIssue{Status: st("todo"), AfterKey: &after}, nil)
	if list, _ := listIssues(t, env, q); keys(list) != "XC-3,XC-2,XC-1" {
		t.Fatalf("after-only move: %s", keys(list))
	}
	// No anchors: end of another column; status changes and completedAt is set.
	var moved api.Issue
	env.MustDo(http.MethodPost, "/issues/XC-3/move", api.MoveIssue{Status: st("done")}, &moved)
	if moved.Status != "done" || moved.CompletedAt == nil {
		t.Fatalf("to done: %+v", moved)
	}
	// An anchor from another column is rejected.
	if status, _ := env.Do(http.MethodPost, "/issues/XC-1/move", api.MoveIssue{Status: st("todo"), AfterKey: &after}, nil); status != 400 {
		t.Fatalf("anchor in other column: %d", status)
	}

	// Swapping the two issues below the first one halves the gap under the
	// first issue every time; after ~30 swaps the column must be rebalanced.
	rb := createProject(t, env, "RB")
	for _, title := range []string{"c", "b", "a"} {
		createIssue(t, env, rb.Id, api.CreateIssue{Title: title})
	}
	// Column: RB-3, RB-2, RB-1.
	lower, upper := "RB-1", "RB-2"
	for i := 0; i < 60; i++ {
		env.MustDo(http.MethodPost, "/issues/"+lower+"/move", api.MoveIssue{Status: st("todo"), AfterKey: strPtr("RB-3"), BeforeKey: &upper}, nil)
		lower, upper = upper, lower
	}
	if list, _ := listIssues(t, env, fmt.Sprintf("projectId=%d&status=todo&sort=manual", rb.Id)); keys(list) != "RB-3,"+upper+","+lower {
		t.Fatalf("after many moves: %s (want RB-3,%s,%s)", keys(list), upper, lower)
	}
}

func TestRebalanceTinyGap(t *testing.T) {
	env := newEnv(t)
	p := createProject(t, env, "XC")
	createIssue(t, env, p.Id, api.CreateIssue{Title: "b"})
	createIssue(t, env, p.Id, api.CreateIssue{Title: "a"})
	createIssue(t, env, p.Id, api.CreateIssue{Title: "x", Status: ptr(api.IssueStatus("backlog"))})
	// Squeeze XC-2 and XC-1 to almost the same sort order.
	if _, err := env.App.Deps.DB.Exec(`UPDATE issues SET sort_order = 1.0 WHERE number = 2; UPDATE issues SET sort_order = 1.0000000001 WHERE number = 1`); err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodPost, "/issues/XC-3/move", api.MoveIssue{Status: st("todo"), AfterKey: strPtr("XC-2"), BeforeKey: strPtr("XC-1")}, nil)
	if list, _ := listIssues(t, env, fmt.Sprintf("projectId=%d&status=todo&sort=manual", p.Id)); keys(list) != "XC-2,XC-3,XC-1" {
		t.Fatalf("rebalanced: %s", keys(list))
	}
}

func ptr[T any](v T) *T       { return &v }
func strPtr(s string) *string { return &s }
func today(env *testutil.Env) time.Time {
	return time.Now().In(env.App.Deps.Config.Location)
}

func TestListFilters(t *testing.T) {
	env := newEnv(t)
	p := createProject(t, env, "XC")
	other := createProject(t, env, "OT")
	var label api.Label
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/labels", p.Id), api.CreateLabel{Name: "global", Global: ptr(true)}, &label)
	var ms api.Milestone
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/milestones", p.Id), api.CreateMilestone{Name: "v1"}, &ms)
	day := func(offset int) string { return today(env).AddDate(0, 0, offset).Format("2006-01-02") }
	createIssue(t, env, p.Id, map[string]any{"title": "Fix login bug", "priority": 1, "dueDate": day(0), "labelIds": []int64{label.Id}})
	createIssue(t, env, p.Id, map[string]any{"title": "Write docs", "priority": 3, "dueDate": day(-2), "milestoneId": ms.Id})
	createIssue(t, env, p.Id, map[string]any{"title": "Refactor", "priority": 0, "dueDate": day(5), "status": "in_progress"})
	createIssue(t, env, p.Id, map[string]any{"title": "Old done", "priority": 2, "dueDate": day(-3), "status": "done"})
	createIssue(t, env, other.Id, map[string]any{"title": "Other login", "priority": 4})
	// The global label is visible (and usable) in the other project.
	var labels []api.Label
	env.MustDo(http.MethodGet, fmt.Sprintf("/projects/%d/labels", other.Id), nil, &labels)
	if len(labels) != 1 || labels[0].ProjectId != nil {
		t.Fatalf("global label: %+v", labels)
	}

	pid := fmt.Sprintf("projectId=%d", p.Id)
	cases := map[string]string{
		pid + "&status=todo&status=in_progress&sort=priority": "XC-1,XC-2,XC-3",
		pid + "&priority=3":                         "XC-2",
		pid + fmt.Sprintf("&labelId=%d", label.Id):  "XC-1",
		pid + fmt.Sprintf("&milestoneId=%d", ms.Id): "XC-2",
		pid + "&due=today":                          "XC-1",
		pid + "&due=week&sort=due":                  "XC-1,XC-3",
		pid + "&due=overdue":                        "XC-2",
		pid + "&sort=due":                           "XC-4,XC-2,XC-1,XC-3",
		"q=login&sort=priority":                     "XC-1,OT-1",
		"q=" + url.QueryEscape("OT-1"):              "OT-1",
	}
	for q, want := range cases {
		if list, _ := listIssues(t, env, q); keys(list) != want {
			t.Errorf("%s: got %s want %s", q, keys(list), want)
		}
	}
	if status, _ := env.Do(http.MethodGet, "/issues?due=someday", nil, nil); status != 400 {
		t.Fatalf("bad due: %d", status)
	}
	// Paging with an offset cursor.
	page, next := listIssues(t, env, pid+"&sort=due&limit=3")
	if len(page) != 3 || next == "" {
		t.Fatalf("page 1: %s %q", keys(page), next)
	}
	page, next = listIssues(t, env, pid+"&sort=due&limit=3&cursor="+next)
	if keys(page) != "XC-3" || next != "" {
		t.Fatalf("page 2: %s %q", keys(page), next)
	}
	// Archived projects drop out of cross-project lists.
	env.MustDo(http.MethodDelete, fmt.Sprintf("/projects/%d", other.Id), nil, nil)
	if list, _ := listIssues(t, env, "q=login"); keys(list) != "XC-1" {
		t.Fatalf("archived project: %s", keys(list))
	}
}

func TestCommentsAndLinks(t *testing.T) {
	env := newEnv(t)
	p := createProject(t, env, "XC")
	createIssue(t, env, p.Id, api.CreateIssue{Title: "a"})
	var c api.Comment
	env.MustDo(http.MethodPost, "/issues/XC-1/comments", api.CreateComment{Body: "started"}, &c)
	var comments []api.Comment
	env.MustDo(http.MethodGet, "/issues/XC-1/comments", nil, &comments)
	if len(comments) != 1 || comments[0].Body != "started" {
		t.Fatalf("comments: %+v", comments)
	}
	if status, _ := env.Do(http.MethodPost, "/issues/XC-1/comments", api.CreateComment{Body: " "}, nil); status != 400 {
		t.Fatalf("empty comment: %d", status)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/issues/XC-1/comments/%d", c.Id), nil, nil)
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/issues/XC-1/comments/%d", c.Id), nil, nil); status != 404 {
		t.Fatalf("delete twice: %d", status)
	}

	var link api.IssueLink
	env.MustDo(http.MethodPost, "/issues/XC-1/links", api.CreateIssueLink{Kind: "url", Url: "https://example.com"}, &link)
	// Same kind and url is stored once.
	env.MustDo(http.MethodPost, "/issues/XC-1/links", api.CreateIssueLink{Kind: "url", Url: "https://example.com"}, nil)
	var links []api.IssueLink
	env.MustDo(http.MethodGet, "/issues/XC-1/links", nil, &links)
	if len(links) != 1 || links[0].Title != "https://example.com" {
		t.Fatalf("links: %+v", links)
	}
	if status, _ := env.Do(http.MethodPost, "/issues/XC-1/links", map[string]string{"kind": "weird", "url": "x"}, nil); status != 400 {
		t.Fatalf("bad kind: %d", status)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/issues/XC-1/links/%d", link.Id), nil, nil)
	if status, _ := env.Do(http.MethodGet, "/issues/XC-9/links", nil, nil); status != 404 {
		t.Fatalf("links of missing issue: %d", status)
	}

	// Milestone update and delete; deleting clears it from issues.
	var ms api.Milestone
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/milestones", p.Id), api.CreateMilestone{Name: "v1"}, &ms)
	env.MustDo(http.MethodPatch, "/issues/XC-1", map[string]any{"milestoneId": ms.Id}, nil)
	env.MustDo(http.MethodPatch, fmt.Sprintf("/projects/%d/milestones/%d", p.Id, ms.Id), map[string]any{"name": "v2", "dueDate": "2027-01-01"}, &ms)
	if ms.Name != "v2" || ms.DueDate == nil {
		t.Fatalf("milestone: %+v", ms)
	}
	msPath := fmt.Sprintf("/projects/%d/milestones/%d", p.Id, ms.Id)
	ms = api.Milestone{}
	env.MustDo(http.MethodPatch, msPath, map[string]any{"dueDate": nil}, &ms)
	if ms.DueDate != nil {
		t.Fatalf("milestone clear: %+v", ms)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/projects/%d/milestones/%d", p.Id, ms.Id), nil, nil)
	var got api.Issue
	env.MustDo(http.MethodGet, "/issues/XC-1", nil, &got)
	if got.MilestoneId != nil {
		t.Fatalf("milestone not cleared: %+v", got)
	}
	var label api.Label
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/labels", p.Id), api.CreateLabel{Name: "bug"}, &label)
	env.MustDo(http.MethodPatch, fmt.Sprintf("/projects/%d/labels/%d", p.Id, label.Id), api.UpdateLabel{Color: strPtr("#f00")}, &label)
	if label.Color != "#f00" {
		t.Fatalf("label: %+v", label)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/projects/%d/labels/%d", p.Id, label.Id), nil, nil)
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/projects/%d/labels/%d", p.Id, label.Id), nil, nil); status != 404 {
		t.Fatalf("label twice: %d", status)
	}
}

func TestIssuesContract(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	p := createProject(t, env, "XC")
	issues, ok := module.Lookup[contracts.Issues](env.App.Deps.Registry, contracts.IssuesKey)
	if !ok {
		t.Fatal("contracts.Issues not provided")
	}
	due := time.Now().Add(-48 * time.Hour)
	ref, err := issues.Create(ctx, contracts.CreateIssue{ProjectID: p.Id, Title: "From contract", Priority: 2, DueDate: &due})
	if err != nil || ref.Key != "XC-1" || ref.DueDate == nil || ref.Status != "todo" {
		t.Fatalf("create: %+v %v", ref, err)
	}
	if _, err := issues.Create(ctx, contracts.CreateIssue{ProjectID: p.Id, Title: "Later", DueDate: ptr(time.Now().AddDate(0, 0, 30))}); err != nil {
		t.Fatal(err)
	}
	list, err := issues.ListDue(ctx, time.Now())
	if err != nil || len(list) != 1 || list[0].Key != "XC-1" {
		t.Fatalf("list due: %+v %v", list, err)
	}
	if err := issues.SetStatus(ctx, "XC-1", "in_progress"); err != nil {
		t.Fatal(err)
	}
	if err := issues.AttachLink(ctx, "XC-1", contracts.IssueLink{Kind: "coding_task", Title: "Task 3", URL: "/coding/3", Ref: "3"}); err != nil {
		t.Fatal(err)
	}
	got, err := issues.Get(ctx, "XC-1")
	if err != nil || got.Status != "in_progress" {
		t.Fatalf("get: %+v %v", got, err)
	}
	var links []api.IssueLink
	env.MustDo(http.MethodGet, "/issues/XC-1/links", nil, &links)
	if len(links) != 1 || links[0].Kind != "coding_task" {
		t.Fatalf("links: %+v", links)
	}
	if _, err := issues.Get(ctx, "XC-42"); err == nil {
		t.Fatal("missing issue should fail")
	}
	if err := issues.SetStatus(ctx, "XC-1", "bogus"); err == nil {
		t.Fatal("bad status should fail")
	}
}

func TestIssueSyncContract(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	p := createProject(t, env, "XC")
	sync, ok := module.Lookup[contracts.IssueSync](env.App.Deps.Registry, contracts.IssueSyncKey)
	if !ok {
		t.Fatal("contracts.IssueSync not provided")
	}
	ch, cancel := env.App.Deps.Bus.Subscribe("issue.", 64)
	defer cancel()

	remote := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	created, err := sync.Upsert(ctx, contracts.SyncIssue{ProjectID: p.Id, Title: "From Linear", Status: "in_progress",
		Priority: 2, ExternalSource: "linear", ExternalID: "LIN-1", UpdatedAt: remote})
	if err != nil || created.Key != "XC-1" || !created.UpdatedAt.Equal(remote) {
		t.Fatalf("upsert create: %+v %v", created, err)
	}
	found, ok, err := sync.FindByExternal(ctx, "linear", "LIN-1")
	if err != nil || !ok || found.Key != "XC-1" || found.Status != "in_progress" {
		t.Fatalf("find: %+v %v %v", found, ok, err)
	}
	if _, ok, _ := sync.FindByExternal(ctx, "linear", "LIN-404"); ok {
		t.Fatal("unknown external id found")
	}
	remote2 := remote.Add(time.Hour)
	updated, err := sync.Upsert(ctx, contracts.SyncIssue{Title: "Renamed", Status: "done", ExternalSource: "linear",
		ExternalID: "LIN-1", UpdatedAt: remote2})
	if err != nil || updated.Key != "XC-1" || updated.Title != "Renamed" || !updated.UpdatedAt.Equal(remote2) {
		t.Fatalf("upsert update: %+v %v", updated, err)
	}
	// Upserts publish issue.synced only, never issue.created/updated.
	drain := func() map[string]int {
		seen := map[string]int{}
		for {
			select {
			case ev := <-ch:
				seen[ev.Topic]++
			case <-time.After(50 * time.Millisecond):
				return seen
			}
		}
	}
	if seen := drain(); seen["issue.synced"] != 2 || seen["issue.created"] != 0 || seen["issue.updated"] != 0 {
		t.Fatalf("sync events: %v", seen)
	}

	// A local issue changed after the last sync shows up; synced ones with
	// older remote times do not.
	since := time.Now().UTC().Add(-time.Minute)
	createIssue(t, env, p.Id, api.CreateIssue{Title: "Local"})
	changed, err := sync.ChangedSince(ctx, []int64{p.Id}, since)
	if err != nil || len(changed) != 1 || changed[0].Key != "XC-2" || changed[0].ExternalID != "" {
		t.Fatalf("changed since: %+v %v", changed, err)
	}
	if err := sync.Link(ctx, "XC-2", "linear", "LIN-2"); err != nil {
		t.Fatal(err)
	}
	if found, ok, _ := sync.FindByExternal(ctx, "linear", "LIN-2"); !ok || found.Key != "XC-2" {
		t.Fatalf("link: %+v", found)
	}
	if err := sync.Link(ctx, "XC-2", "linear", "LIN-1"); err == nil {
		t.Fatal("linking an external id used by another issue should fail")
	}
	if got, _ := sync.ChangedSince(ctx, nil, since); len(got) != 0 {
		t.Fatalf("no projects: %+v", got)
	}
}

func TestActions(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	createProject(t, env, "XC")
	run := func(name, input string) any {
		t.Helper()
		out, err := env.App.Deps.Actions.Run(ctx, name, json.RawMessage(input))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	if list := run("projects.list", `{}`).([]api.Project); len(list) != 1 {
		t.Fatalf("projects.list: %+v", list)
	}
	issue := run("projects.create_issue", `{"projectKey":"XC","title":"From AI","priority":2,"dueDate":"2026-10-10"}`).(api.Issue)
	if issue.Key != "XC-1" || issue.DueDate == nil {
		t.Fatalf("create_issue: %+v", issue)
	}
	issue = run("projects.update_issue", `{"key":"XC-1","status":"in_review","dueDate":""}`).(api.Issue)
	if issue.Status != "in_review" || issue.DueDate != nil {
		t.Fatalf("update_issue: %+v", issue)
	}
	if list := run("projects.list_issues", `{"projectKey":"XC","status":["in_review"]}`).([]api.Issue); len(list) != 1 {
		t.Fatalf("list_issues: %+v", list)
	}
	got := run("projects.get_issue", `{"key":"XC-1"}`).(map[string]any)
	if got["issue"].(api.Issue).Title != "From AI" {
		t.Fatalf("get_issue: %+v", got)
	}
	if _, err := env.App.Deps.Actions.Run(ctx, "projects.create_issue", json.RawMessage(`{"title":"no project"}`)); err == nil {
		t.Fatal("create_issue without project should fail")
	}
}

func TestManyIssues(t *testing.T) {
	env := newEnv(t)
	p := createProject(t, env, "XC")
	issues, _ := module.Lookup[contracts.Issues](env.App.Deps.Registry, contracts.IssuesKey)
	for i := 0; i < 500; i++ {
		if _, err := issues.Create(context.Background(), contracts.CreateIssue{ProjectID: p.Id, Title: fmt.Sprintf("Issue %d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	list, next := listIssues(t, env, fmt.Sprintf("projectId=%d&sort=manual&limit=1000", p.Id))
	if len(list) != 500 || next != "" {
		t.Fatalf("got %d issues, next %q", len(list), next)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("listing 500 issues took %v", d)
	}
}

func TestIssueToolsListedOnce(t *testing.T) {
	env := testutil.New(t)
	names := map[string]bool{}
	for _, a := range env.App.Deps.Actions.List(context.Background()) {
		names[a.Name] = true
	}
	for _, alias := range []string{"issues.list", "issues.get", "issues.create", "issues.update"} {
		if names[alias] {
			t.Fatalf("%s is listed next to its projects.* twin", alias)
		}
		if _, ok := env.App.Deps.Actions.Get(context.Background(), alias); !ok {
			t.Fatalf("%s no longer runs for saved automation rules", alias)
		}
	}
	for _, name := range []string{"projects.list_issues", "projects.get_issue", "projects.create_issue", "projects.update_issue", "issues.move", "issues.delete"} {
		if !names[name] {
			t.Fatalf("%s missing", name)
		}
	}
}

func st(s string) *api.IssueStatus {
	v := api.IssueStatus(s)
	return &v
}
