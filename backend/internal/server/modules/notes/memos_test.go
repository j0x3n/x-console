package notes_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestMemoFilteringSearchCountsAndConversion(t *testing.T) {
	env := testutil.New(t)
	n := createNote(t, env, api.CreateNote{Title: str("普通数据库"), Body: str("数据库索引"), Tags: &[]string{"共同"}, Pinned: ptr(true)})
	memo := createNote(t, env, api.CreateNote{Title: str("便签数据库"), Body: str(strings.Repeat("索引数据库", 1001)), Tags: &[]string{"共同", "便签"}, Kind: ptr(api.NoteKindMemo), Pinned: ptr(true), Color: ptr(api.NoteColor("teal"))})
	archived := createNote(t, env, api.CreateNote{Title: str("归档便签"), Kind: ptr(api.NoteKindMemo)})
	env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", archived.Id), map[string]bool{"archived": true}, nil)
	if n.Kind == nil || *n.Kind != api.NoteKindNote || memo.Kind == nil || *memo.Kind != api.NoteKindMemo || memo.Color == nil || *memo.Color != "teal" {
		t.Fatalf("created: %+v %+v", n, memo)
	}
	for query, want := range map[string]string{"kind=note": "普通数据库", "kind=memo": "便签数据库", "kind=memo&pinned=true": "便签数据库", "kind=memo&archived=true": "归档便签", "kind=note&q=数据库": "普通数据库", "kind=memo&q=数据库": "便签数据库", "kind=note&q=索引": "普通数据库", "kind=memo&q=索引": "便签数据库", "kind=memo&tag=共同": "便签数据库"} {
		if got := titles(search(t, env, query)); got != want {
			t.Errorf("%s: %s", query, got)
		}
	}
	p := search(t, env, "kind=memo")
	if len(p.Items) != 1 || p.Items[0].Body == nil || utf8.RuneCountInString(*p.Items[0].Body) != 4000 || p.Items[0].BodyTruncated == nil || !*p.Items[0].BodyTruncated {
		t.Fatalf("memo body: %+v", p.Items)
	}
	for _, query := range []string{"", "kind=note", "q=数据库"} {
		for _, summary := range search(t, env, query).Items {
			if summary.Body != nil || summary.BodyTruncated != nil {
				t.Fatalf("heavy default list: %s %+v", query, summary)
			}
		}
	}
	var tags []api.TagCount
	env.MustDo(http.MethodGet, "/notes/tags", nil, &tags)
	if len(tags) != 2 || tags[0].Tag != "共同" || tags[0].Count != 2 || tags[0].MemoCount == nil || *tags[0].MemoCount != 1 {
		t.Fatalf("tag counts: %+v", tags)
	}
	var counts api.NoteCounts
	env.MustDo(http.MethodGet, "/notes/counts", nil, &counts)
	if counts.Notes != 1 || counts.Memos != 1 || counts.Pinned != 1 || counts.Archived != 1 {
		t.Fatalf("counts: %+v", counts)
	}
	var converted api.Note
	env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", memo.Id), map[string]string{"kind": "note"}, &converted)
	if converted.Body != memo.Body || converted.Kind == nil || *converted.Kind != api.NoteKindNote || converted.Color == nil || *converted.Color != "teal" {
		t.Fatalf("convert: %+v", converted)
	}
	if _, err := env.App.Deps.DB.Exec("INSERT INTO notes (title,body,pinned,hidden,created_at,updated_at) VALUES ('旧数据','',0,0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if got := titles(search(t, env, "kind=note")); !strings.Contains(got, "旧数据") {
		t.Fatalf("default kind: %s", got)
	}
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		path := "/notes"
		if method == http.MethodPatch {
			path = fmt.Sprintf("/notes/%d", n.Id)
		}
		if code, _ := env.Do(method, path, map[string]string{"kind": "invalid"}, nil); code != 400 {
			t.Fatalf("invalid kind %s: %d", method, code)
		}
	}
	if code, _ := env.Do(http.MethodGet, "/notes?kind=invalid", nil, nil); code != 400 {
		t.Fatalf("invalid filter: %d", code)
	}
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	createNote(t, env, api.CreateNote{Kind: ptr(api.NoteKindMemo), Hidden: ptr(true), Pinned: ptr(true)})
	env.MustDo(http.MethodGet, "/notes/counts", nil, &counts)
	if counts.Memos != 0 || counts.Notes != 3 || counts.Pinned != 2 || counts.Archived != 1 {
		t.Fatalf("hidden counts: %+v", counts)
	}
}

func TestMemoActionsAndMCP(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	out, err := env.App.Deps.Actions.Run(ctx, "notes.create", json.RawMessage(`{"kind":"memo","body":"MCP便签数据库","tags":["memo"]}`))
	if err != nil {
		t.Fatal(err)
	}
	memo := out.(api.Note)
	for _, name := range []string{"notes.list", "notes.search"} {
		out, err = env.App.Deps.Actions.Run(ctx, name, json.RawMessage(`{"kind":"memo","q":"数据库"}`))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		items := out.([]api.NoteSummary)
		if len(items) != 1 || items[0].Id != memo.Id || items[0].Body == nil {
			t.Fatalf("%s: %+v", name, items)
		}
	}
	out, err = env.App.Deps.Actions.Run(ctx, "notes.update", json.RawMessage(fmt.Sprintf(`{"id":%d,"kind":"note"}`, memo.Id)))
	if err != nil || out.(api.Note).Kind == nil || *out.(api.Note).Kind != api.NoteKindNote {
		t.Fatalf("update: %+v %v", out, err)
	}
	env.Elevate()
	var token struct{ Secret string }
	env.MustDo(http.MethodPost, "/api-tokens", map[string]any{"name": "memo-test", "access": "write", "modules": []string{"notes"}}, &token)
	call := func(method string, params any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
		req, _ := http.NewRequest(http.MethodPost, env.URL("/mcp"), bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token.Secret)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		var out map[string]any
		if err := json.Unmarshal(data, &out); err != nil || resp.StatusCode != 200 {
			t.Fatalf("rpc: %d %s", resp.StatusCode, data)
		}
		return out
	}
	listed := call("tools/list", nil)
	tools := listed["result"].(map[string]any)["tools"].([]any)
	seen := map[string]bool{}
	for _, tool := range tools {
		item := tool.(map[string]any)
		name := item["name"].(string)
		if name == "notes_create" || name == "notes_list" {
			schema := item["inputSchema"].(map[string]any)
			seen[name] = schema["properties"].(map[string]any)["kind"] != nil
		}
	}
	if !seen["notes_create"] || !seen["notes_list"] {
		t.Fatalf("schemas: %+v", seen)
	}
	created := call("tools/call", map[string]any{"name": "notes_create", "arguments": map[string]any{"body": "来自MCP", "kind": "memo"}})
	if created["result"].(map[string]any)["isError"] == true {
		t.Fatalf("create: %+v", created)
	}
	listed = call("tools/call", map[string]any{"name": "notes_list", "arguments": map[string]any{"kind": "memo"}})
	if listed["result"].(map[string]any)["isError"] == true || len(search(t, env, "kind=memo").Items) != 1 {
		t.Fatalf("list: %+v", listed)
	}
}

func TestQuickMemoAI(t *testing.T) {
	env, delay, fake := setupNoteAI(t)
	fake.Queue(llm.Result{Text: `{"title":"便签标题","tags":["便签标签"]}`}, nil)
	memo := createNote(t, env, api.CreateNote{Body: str("短便签"), Quick: ptr(true)})
	if memo.Kind == nil || *memo.Kind != api.NoteKindMemo {
		t.Fatalf("quick kind: %+v", memo)
	}
	delay.Fire()
	var got api.Note
	env.MustDo(http.MethodGet, fmt.Sprintf("/notes/%d", memo.Id), nil, &got)
	if got.Title != "便签标题" || strings.Join(got.Tags, ",") != "便签标签" || got.Kind == nil || *got.Kind != api.NoteKindMemo {
		t.Fatalf("AI memo: %+v", got)
	}
}
