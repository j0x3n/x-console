package projects_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
)

func TestBoardRepoSync(t *testing.T) {
	for _, kind := range []string{"github", "forgejo"} {
		t.Run(kind, func(t *testing.T) {
			env := newEnv(t)
			if status, _ := env.Do("PUT", "/boards/999/repo", map[string]any{"connectionId": 1, "fullName": "team/app"}, nil); status != 403 {
				t.Fatalf("missing elevation: %d", status)
			}
			env.Elevate()
			var mu sync.Mutex
			title, state, label := "仓库的卡片", "open", "bug"
			failWrite := false
			pages, single := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				scheme := "Bearer"
				if kind == "forgejo" {
					scheme = "token"
				}
				if r.Header.Get("Authorization") != scheme+" test-secret" {
					t.Errorf("auth missing")
					w.WriteHeader(401)
					return
				}
				path := strings.TrimPrefix(r.URL.Path, "/api/v1")
				switch {
				case path == "/user":
					_ = json.NewEncoder(w).Encode(map[string]any{"login": "me"})
				case path == "/repos/team/app":
					_ = json.NewEncoder(w).Encode(map[string]any{"full_name": "team/app", "html_url": "https://git.example/team/app", "clone_url": "https://git.example/team/app.git", "default_branch": "main"})
				case path == "/repos/team/app/issues" && r.Method == "GET":
					pages++
					rows := []map[string]any{}
					if r.URL.Query().Get("page") == "1" {
						rows = append(rows, map[string]any{"number": 1, "title": title, "body": "正文", "state": state, "html_url": "https://git.example/team/app/issues/1", "labels": []map[string]any{{"name": label}}})
						for n := 2; n <= 100; n++ {
							rows = append(rows, map[string]any{"number": n, "title": "PR", "pull_request": map[string]string{"url": "x"}})
						}
					} else {
						rows = append(rows, map[string]any{"number": 101, "title": "另一页", "state": "open"})
					}
					_ = json.NewEncoder(w).Encode(rows)
				case path == "/repos/team/app/issues/1" && r.Method == "GET":
					single++
					_ = json.NewEncoder(w).Encode(map[string]any{"number": 1, "title": title, "body": "正文", "state": state, "html_url": "https://git.example/team/app/issues/1", "labels": []map[string]any{{"name": label}}})
				case path == "/repos/team/app/issues/2" && r.Method == "GET":
					single++
					_ = json.NewEncoder(w).Encode(map[string]any{"number": 2, "title": "PR", "state": "open", "pull_request": map[string]string{"url": "x"}})
				case path == "/repos/team/app/issues/1" && r.Method == "PATCH":
					if failWrite {
						w.WriteHeader(403)
						_, _ = w.Write([]byte(`{"message":"no permission"}`))
						return
					}
					var in map[string]string
					_ = json.NewDecoder(r.Body).Decode(&in)
					title, state = in["title"], in["state"]
					_, _ = w.Write([]byte(`{}`))
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			var connection struct{ Connection struct{ ID int64 } }
			env.MustDo("POST", "/git-connections", map[string]any{"name": "测试", "kind": kind, "baseUrl": server.URL, "token": "test-secret"}, &connection)
			p := createProject(t, env, "XC")
			b := boards(t, env, p.Id)[0]
			path := fmt.Sprintf("/boards/%d/repo", b.Id)
			env.MustDo("PUT", path, map[string]any{"connectionId": connection.Connection.ID, "fullName": "team/app"}, &b)
			if b.Repo == nil || b.Repo.LastSyncedAt == nil || *b.Repo.SyncedCount != 2 {
				t.Fatalf("repo: %+v", b.Repo)
			}
			items, _ := listIssues(t, env, fmt.Sprintf("boardId=%d", b.Id))
			if len(items) != 2 {
				t.Fatalf("items: %+v", items)
			}
			var card api.Issue
			for _, i := range items {
				if i.ExternalId == "team/app#1" {
					card = i
				}
			}
			if card.ExternalUrl == nil || len(card.Labels) != 1 || card.ExternalSource != kind {
				t.Fatalf("card: %+v", card)
			}
			env.MustDo("POST", path+"/sync", nil, &b)
			items, _ = listIssues(t, env, fmt.Sprintf("boardId=%d", b.Id))
			if len(items) != 2 {
				t.Fatal("duplicate cards")
			}
			// A locked layout still accepts repository updates.
			env.MustDo("PATCH", fmt.Sprintf("/projects/%d", p.Id), map[string]any{"layoutLocked": true}, nil)
			mu.Lock()
			title, state, label = "远端改了", "closed", "fixed"
			pagesBefore := pages
			mu.Unlock()
			hooks, _ := module.Lookup[contracts.GitWebhookReceiver](env.App.Deps.Registry, contracts.BoardWebhookKey)
			for _, n := range []int{1, 2} {
				err := hooks.ReceiveGitWebhook(context.Background(), contracts.GitWebhook{ConnectionID: connection.Connection.ID, Event: "issues", Body: json.RawMessage(fmt.Sprintf(`{"repository":{"full_name":"team/app"},"issue":{"number":%d}}`, n))})
				if err != nil {
					t.Fatal(err)
				}
			}
			mu.Lock()
			if pages != pagesBefore || single != 2 {
				t.Errorf("webhook should read one issue: pages %d -> %d, single %d", pagesBefore, pages, single)
			}
			mu.Unlock()
			if items, _ := listIssues(t, env, fmt.Sprintf("boardId=%d", b.Id)); len(items) != 2 {
				t.Fatalf("pull request became a card: %+v", items)
			}
			card = getIssue(t, env, card.Key)
			if card.Title != "远端改了" || card.Status != "done" || card.Labels[0].Name != "fixed" {
				t.Fatalf("updated: %+v", card)
			}
			mu.Lock()
			state = "open"
			mu.Unlock()
			env.MustDo("POST", path+"/sync", nil, nil)
			card = getIssue(t, env, card.Key)
			if card.Status != "todo" {
				t.Fatal(card.Status)
			}
			// Local edits write back, but failure leaves the card saved.
			env.MustDo("PATCH", "/issues/"+card.Key, map[string]any{"title": "本地改了", "status": "done"}, nil)
			mu.Lock()
			if title != "本地改了" || state != "closed" {
				t.Error("writeback missing")
			}
			failWrite = true
			mu.Unlock()
			env.MustDo("PATCH", "/issues/"+card.Key, map[string]any{"title": "本地保存"}, nil)
			card = getIssue(t, env, card.Key)
			if card.Title != "本地保存" {
				t.Fatal(card.Title)
			}
			b = boards(t, env, p.Id)[0]
			if b.Repo.LastError == nil || *b.Repo.LastError == "" {
				t.Fatal("missing error")
			}
			p2 := createProject(t, env, "YY")
			other := boards(t, env, p2.Id)[0]
			if code, _ := env.Do("PUT", fmt.Sprintf("/boards/%d/repo", other.Id), map[string]any{"connectionId": connection.Connection.ID, "fullName": "team/app"}, nil); code != 409 {
				t.Fatalf("conflict: %d", code)
			}
			env.MustDo("DELETE", path, nil, nil)
			card = getIssue(t, env, card.Key)
			if card.ExternalId != "" || card.ExternalUrl != nil {
				t.Fatal("not detached")
			}
			if code, _ := env.Do("POST", path+"/sync", nil, nil); code != 404 {
				t.Fatal(code)
			}
			if pages < 4 {
				t.Fatal("pagination missing")
			}
			// Deleting the connection unbinds its boards instead of failing.
			mu.Lock()
			failWrite = false
			mu.Unlock()
			env.MustDo("PUT", path, map[string]any{"connectionId": connection.Connection.ID, "fullName": "team/app"}, nil)
			if code, _ := env.Do("DELETE", fmt.Sprintf("/git-connections/%d", connection.Connection.ID), nil, nil); code != 204 {
				t.Fatalf("delete connection: %d", code)
			}
			if b = boards(t, env, p.Id)[0]; b.Repo != nil {
				t.Fatalf("board still bound: %+v", b.Repo)
			}
			card = getIssue(t, env, card.Key)
			if card.ExternalId != "" {
				t.Fatal("card not detached after connection delete")
			}
		})
	}
}
