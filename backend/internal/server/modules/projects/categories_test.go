package projects_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
)

func TestProjectCategoriesTreeOrderAndDeletion(t *testing.T) {
	env := newEnv(t)
	var foreignKeys int
	if err := env.App.Deps.DB.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatalf("foreign keys=%d err=%v", foreignKeys, err)
	}
	project := createProject(t, env, "CAT")
	other := createProject(t, env, "OTH")
	base := fmt.Sprintf("/projects/%d/categories", project.Id)
	var backend, frontend, servers api.ProjectCategory
	env.MustDo("POST", base, map[string]any{"name": " 后端 "}, &backend)
	env.MustDo("POST", base, map[string]any{"name": "前端"}, &frontend)
	env.MustDo("POST", base, map[string]any{"name": "服务器", "parentId": backend.Id}, &servers)
	if servers.ParentId == nil || *servers.ParentId != backend.Id || backend.Name != "后端" {
		t.Fatalf("tree: %+v %+v", backend, servers)
	}
	if status, raw := env.Do("POST", base, map[string]any{"name": "第三级", "parentId": servers.Id}, nil); status != 400 || errCode(t, raw) != "invalid_parent" {
		t.Fatalf("third level %d: %s", status, raw)
	}
	if status, raw := env.Do("POST", fmt.Sprintf("/projects/%d/categories", other.Id), map[string]any{"name": "跨项目", "parentId": backend.Id}, nil); status != 400 || errCode(t, raw) != "invalid_parent" {
		t.Fatalf("cross project %d: %s", status, raw)
	}
	var listed []api.ProjectCategory
	env.MustDo("GET", base, nil, &listed)
	if len(listed) != 3 || listed[0].Id != backend.Id || listed[1].Id != servers.Id || listed[2].Id != frontend.Id {
		t.Fatalf("order: %+v", listed)
	}
	issue := createIssue(t, env, project.Id, map[string]any{"title": "测试分类"})
	if _, err := env.App.Deps.DB.Exec("UPDATE issues SET category_id=? WHERE id=?", servers.Id, issue.Id); err != nil {
		t.Fatal(err)
	}
	env.MustDo("GET", base, nil, &listed)
	if listed[1].IssueCount != 1 || listed[0].IssueCount != 0 {
		t.Fatalf("direct count: %+v", listed)
	}
	env.MustDo("PATCH", fmt.Sprintf("%s/%d", base, frontend.Id), map[string]any{"name": "界面", "beforeId": backend.Id}, &frontend)
	env.MustDo("GET", base, nil, &listed)
	if listed[0].Id != frontend.Id || listed[1].Id != backend.Id || listed[2].Id != servers.Id {
		t.Fatalf("reorder: %+v", listed)
	}
	if status, _ := env.Do("PATCH", fmt.Sprintf("%s/%d", base, frontend.Id), map[string]any{"afterId": servers.Id}, nil); status != 400 {
		t.Fatalf("cross-level reorder: %d", status)
	}
	if status, _ := env.Do("DELETE", fmt.Sprintf("%s/%d", base, backend.Id), nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	env.MustDo("GET", base, nil, &listed)
	if len(listed) != 1 || listed[0].Id != frontend.Id {
		t.Fatalf("delete children: %+v", listed)
	}
	var categoryID *int64
	if err := env.App.Deps.DB.QueryRow("SELECT category_id FROM issues WHERE id=?", issue.Id).Scan(&categoryID); err != nil || categoryID != nil {
		t.Fatalf("issue category=%v err=%v", categoryID, err)
	}
	if status, _ := env.Do("DELETE", fmt.Sprintf("%s/%d", base, backend.Id), nil, nil); status != 404 {
		t.Fatalf("deleted category: %d", status)
	}
	env.MustDo("DELETE", fmt.Sprintf("/projects/%d", project.Id), nil, nil)
	if status, raw := env.Do("POST", base, map[string]any{"name": "不能新增"}, nil); status != 409 || errCode(t, raw) != "project_archived" {
		t.Fatalf("archive: %d %s", status, raw)
	}
}
