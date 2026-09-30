package projects_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func boards(t *testing.T, env *testutil.Env, projectID int64) []api.Board {
	t.Helper()
	var out []api.Board
	env.MustDo(http.MethodGet, fmt.Sprintf("/projects/%d/boards", projectID), nil, &out)
	return out
}

func listNamed(t *testing.T, b api.Board, name string) api.BoardList {
	t.Helper()
	for _, l := range b.Lists {
		if l.Name == name {
			return l
		}
	}
	t.Fatalf("no list %q in board %q", name, b.Name)
	return api.BoardList{}
}

func getIssue(t *testing.T, env *testutil.Env, key string) api.Issue {
	t.Helper()
	var i api.Issue
	env.MustDo(http.MethodGet, "/issues/"+key, nil, &i)
	return i
}

// B46：新项目有默认看板；新看板、列表对应状态、拖动改状态、外部改状态跟着换列表。
func TestBoardsListsAndStatus(t *testing.T) {
	env := newEnv(t)
	p := createProject(t, env, "XC")
	bs := boards(t, env, p.Id)
	if len(bs) != 1 || len(bs[0].Lists) != 6 || bs[0].Lists[3].Name != "待审核" {
		t.Fatalf("default board: %+v", bs)
	}

	var game api.Board
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/boards", p.Id),
		map[string]any{"name": "游戏设计", "icon": "💡", "preset": "simple"}, &game)
	if len(game.Lists) != 3 || game.Icon != "💡" {
		t.Fatalf("simple board: %+v", game)
	}
	doing := listNamed(t, game, "进行中")
	card := createIssue(t, env, p.Id, map[string]any{"title": "关卡草图", "listId": doing.Id})
	if card.Status != "in_progress" || card.BoardId == nil || *card.BoardId != game.Id || *card.ListId != doing.Id {
		t.Fatalf("created in list: %+v", card)
	}

	// Drag into “已完成”: status follows the list.
	done := listNamed(t, game, "已完成")
	var moved api.Issue
	env.MustDo(http.MethodPost, "/issues/"+card.Key+"/move", map[string]any{"listId": done.Id}, &moved)
	if moved.Status != "done" || moved.CompletedAt == nil {
		t.Fatalf("moved: %+v", moved)
	}

	// Status changed from outside: the card moves to the matching list.
	env.MustDo(http.MethodPatch, "/issues/"+card.Key, map[string]any{"status": "todo"}, nil)
	got := getIssue(t, env, card.Key)
	if *got.ListId != listNamed(t, game, "待处理").Id {
		t.Fatalf("followed status: %+v", got)
	}

	// A list without a status keeps the card's status.
	var ideas api.BoardList
	env.MustDo(http.MethodPost, fmt.Sprintf("/boards/%d/lists", game.Id), map[string]any{"name": "想法"}, &ideas)
	env.MustDo(http.MethodPost, "/issues/"+card.Key+"/move", map[string]any{"listId": ideas.Id}, &moved)
	if moved.Status != "todo" || *moved.ListId != ideas.Id {
		t.Fatalf("no-status list: %+v", moved)
	}
	// No list for this status on the board: the card stays.
	env.MustDo(http.MethodPatch, "/issues/"+card.Key, map[string]any{"status": "backlog"}, nil)
	if got := getIssue(t, env, card.Key); *got.ListId != ideas.Id || got.Status != "backlog" {
		t.Fatalf("stay without list: %+v", got)
	}

	// Rename, WIP limit, reorder lists.
	var renamed api.BoardList
	env.MustDo(http.MethodPatch, fmt.Sprintf("/lists/%d", ideas.Id), map[string]any{"name": "点子", "wipLimit": 3, "afterId": 0}, &renamed)
	if renamed.Name != "点子" || renamed.WipLimit != 3 || renamed.CardCount != 1 {
		t.Fatalf("renamed: %+v", renamed)
	}
	game = boards(t, env, p.Id)[1]
	if game.Lists[0].Id != ideas.Id {
		t.Fatalf("reorder: %+v", game.Lists)
	}
	// A list with cards cannot be deleted.
	if s, raw := env.Do(http.MethodDelete, fmt.Sprintf("/lists/%d", ideas.Id), nil, nil); s != 409 {
		t.Fatalf("delete non-empty list: %d %s", s, raw)
	}

	// Filter by board.
	items, _ := listIssues(t, env, fmt.Sprintf("boardId=%d", game.Id))
	if keys(items) != card.Key {
		t.Fatalf("by board: %s", keys(items))
	}

	// Star the board; it shows up in the sidebar list.
	env.MustDo(http.MethodPatch, fmt.Sprintf("/boards/%d", game.Id), map[string]any{"starred": true}, nil)
	var starred []struct{ ID int64 }
	env.MustDo(http.MethodGet, "/boards/starred", nil, &starred)
	if len(starred) != 1 || starred[0].ID != game.Id {
		t.Fatalf("starred: %+v", starred)
	}
}

// B46：移动保留截止时间；跨项目移动换编号，原项目编号不复用。
func TestMoveKeepsDueAndCrossProject(t *testing.T) {
	env := newEnv(t)
	xc := createProject(t, env, "XC")
	bl := createProject(t, env, "BL")
	due := time.Date(2026, 10, 1, 10, 30, 0, 0, time.UTC)
	a := createIssue(t, env, xc.Id, map[string]any{"title": "A", "dueAt": due})
	b := createIssue(t, env, xc.Id, map[string]any{"title": "B"})
	board := boards(t, env, xc.Id)[0]
	var moved api.Issue
	env.MustDo(http.MethodPost, "/issues/"+a.Key+"/move", map[string]any{"listId": listNamed(t, board, "进行中").Id}, &moved)
	if moved.DueAt == nil || !moved.DueAt.Equal(due) {
		t.Fatalf("due lost on move: %+v", moved.DueAt)
	}
	blList := listNamed(t, boards(t, env, bl.Id)[0], "待办")
	env.MustDo(http.MethodPost, "/issues/"+b.Key+"/move", map[string]any{"listId": blList.Id}, &moved)
	if moved.Key != "BL-1" || moved.ProjectId != bl.Id || moved.Status != "todo" {
		t.Fatalf("cross project: %+v", moved)
	}
	c := createIssue(t, env, xc.Id, map[string]any{"title": "C"})
	if c.Key != "XC-3" {
		t.Fatalf("number reused: %s", c.Key)
	}
	if s, _ := env.Do(http.MethodGet, "/issues/XC-2", nil, nil); s != 404 {
		t.Fatalf("old key still found: %d", s)
	}
}

// B46：归档、恢复、复制、成员、活动记录、删看板。
func TestArchiveCopyMembersActivity(t *testing.T) {
	env := newEnv(t)
	p := createProject(t, env, "XC")
	board := boards(t, env, p.Id)[0]
	card := createIssue(t, env, p.Id, map[string]any{"title": "写方案", "status": "in_progress"})
	var cl struct{ Id int64 }
	env.MustDo(http.MethodPost, "/issues/"+card.Key+"/checklists", map[string]any{"title": "步骤"}, &cl)
	env.MustDo(http.MethodPost, fmt.Sprintf("/issues/%s/checklists/%d/items", card.Key, cl.Id), map[string]any{"text": "第一步"}, nil)

	var archived api.Issue
	env.MustDo(http.MethodPost, "/issues/"+card.Key+"/archive", nil, &archived)
	if archived.ArchivedAt == nil {
		t.Fatalf("archive: %+v", archived)
	}
	if items, _ := listIssues(t, env, fmt.Sprintf("boardId=%d", board.Id)); len(items) != 0 {
		t.Fatalf("archived card on board: %s", keys(items))
	}
	var arch struct{ Issues []api.Issue }
	env.MustDo(http.MethodGet, fmt.Sprintf("/boards/%d/archive", board.Id), nil, &arch)
	if len(arch.Issues) != 1 {
		t.Fatalf("archive list: %+v", arch)
	}
	var restored api.Issue
	env.MustDo(http.MethodPost, "/issues/"+card.Key+"/restore", nil, &restored)
	if restored.ArchivedAt != nil || *restored.ListId != *card.ListId {
		t.Fatalf("restore: %+v", restored)
	}

	var copied api.Issue
	env.MustDo(http.MethodPost, "/issues/"+card.Key+"/copy", nil, &copied)
	if copied.Title != "写方案（副本）" || copied.ChecklistTotal == nil || *copied.ChecklistTotal != 1 || *copied.ListId != *card.ListId {
		t.Fatalf("copy: %+v", copied)
	}

	var withMembers api.Issue
	env.MustDo(http.MethodPut, "/issues/"+card.Key+"/members", map[string]any{"members": []map[string]string{{"kind": "me", "id": ""}, {"kind": "agent", "id": "7"}}}, &withMembers)
	if withMembers.Members == nil || len(*withMembers.Members) != 2 {
		t.Fatalf("members: %+v", withMembers.Members)
	}

	var acts []api.IssueActivity
	env.MustDo(http.MethodGet, "/issues/"+card.Key+"/activity", nil, &acts)
	kinds := map[string]bool{}
	for _, a := range acts {
		kinds[a.Kind] = true
		if a.Actor == "" {
			t.Fatalf("empty actor: %+v", a)
		}
	}
	for _, k := range []string{"created", "archived", "restored", "members"} {
		if !kinds[k] {
			t.Fatalf("missing activity %q: %+v", k, acts)
		}
	}

	// Deleting a board moves its cards to another board.
	var second api.Board
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/boards", p.Id), map[string]any{"name": "第二"}, &second)
	env.MustDo(http.MethodPost, "/issues/"+card.Key+"/move", map[string]any{"listId": listNamed(t, second, "进行中").Id}, nil)
	env.MustDo(http.MethodDelete, fmt.Sprintf("/boards/%d", second.Id), nil, nil)
	if got := getIssue(t, env, card.Key); *got.BoardId != board.Id || got.Status != "in_progress" {
		t.Fatalf("after board delete: %+v", got)
	}
	if s, _ := env.Do(http.MethodDelete, fmt.Sprintf("/boards/%d", board.Id), nil, nil); s != 400 {
		t.Fatalf("delete last board: %d", s)
	}
}

// B46：看板动作（AI 和远程 AI 用）。
func TestBoardActions(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	p := createProject(t, env, "XC")
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/boards", p.Id), map[string]any{"name": "游戏设计", "preset": "simple"}, nil)
	run := func(name, input string) any {
		t.Helper()
		out, err := env.App.Deps.Actions.Run(ctx, name, json.RawMessage(input))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	card := run("projects.create_card", `{"projectKey":"XC","boardName":"游戏设计","listName":"进行中","title":"关卡","dueAt":"2026-10-01T18:00:00+08:00"}`).(api.Issue)
	if card.Status != "in_progress" || card.DueAt == nil {
		t.Fatalf("create_card: %+v", card)
	}
	moved := run("projects.move_card", `{"key":"`+card.Key+`","listName":"已完成"}`).(api.Issue)
	if moved.Status != "done" {
		t.Fatalf("move_card: %+v", moved)
	}
	run("projects.comment", `{"key":"`+card.Key+`","body":"做完了"}`)
	item := run("projects.add_checklist_item", `{"key":"`+card.Key+`","text":"画草图"}`).(api.ChecklistItem)
	checked := run("projects.check_item", fmt.Sprintf(`{"key":%q,"itemId":%d,"done":true}`, card.Key, item.Id)).(api.ChecklistItem)
	if !checked.Done {
		t.Fatalf("check_item: %+v", checked)
	}
	raw, _ := json.Marshal(run("projects.get_board", `{"projectKey":"XC","boardName":"游戏设计"}`))
	if !strings.Contains(string(raw), card.Key) || !strings.Contains(string(raw), "已完成") {
		t.Fatalf("get_board: %s", raw)
	}
	if _, err := env.App.Deps.Actions.Run(ctx, "projects.move_card", json.RawMessage(`{"key":"`+card.Key+`","listName":"没有"}`)); err == nil || !strings.Contains(err.Error(), "待处理") {
		t.Fatalf("unknown list should name the lists: %v", err)
	}
	if got := getIssue(t, env, card.Key); got.CommentCount == nil || *got.CommentCount != 1 {
		t.Fatalf("comment count: %+v", got.CommentCount)
	}
}
