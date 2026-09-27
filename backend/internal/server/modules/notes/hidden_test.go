package notes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func unlockVault(t *testing.T, env *testutil.Env) {
	t.Helper()
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "vault-secret"}, nil)
}

func noteID(id int64) string { return strconv.FormatInt(id, 10) }

func assertHiddenEvent(t *testing.T, ev events.Event, id int64) {
	t.Helper()
	raw, err := json.Marshal(ev.Data)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["id"] != float64(id) || payload["hidden"] != true || len(payload) != 2 {
		t.Fatalf("hidden event %s: %s", ev.Topic, raw)
	}
}

func TestHiddenNotesStayInvisible(t *testing.T) {
	env := testutil.New(t)
	createNote(t, env, api.CreateNote{Title: str("购物清单"), Body: str("牛奶和鸡蛋"), Tags: &[]string{"生活"}, Pinned: ptr(true)})
	status, _ := env.Do(http.MethodPost, "/notes", api.CreateNote{Title: str("保险箱"), Body: str("机密口令藏在这里"), Tags: &[]string{"私人"}, Hidden: ptr(true)}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("create hidden while locked: %d", status)
	}
	unlockVault(t, env)
	ch, cancel := env.App.Deps.Bus.Subscribe("note.", 8)
	defer cancel()
	hiddenNote := createNote(t, env, api.CreateNote{Title: str("保险箱"), Body: str("机密口令藏在这里"), Tags: &[]string{"私人"}, Pinned: ptr(true), Hidden: ptr(true)})
	if hiddenNote.Hidden == nil || !*hiddenNote.Hidden || !hiddenNote.Pinned || len(hiddenNote.Tags) != 1 {
		t.Fatalf("created hidden note: %+v", hiddenNote)
	}
	select {
	case ev := <-ch:
		if ev.Topic != "note.created" {
			t.Fatalf("event topic: %s", ev.Topic)
		}
		assertHiddenEvent(t, ev, hiddenNote.Id)
	case <-time.After(time.Second):
		t.Fatal("missing create event")
	}
	if hits := ftsHits(t, env, hiddenNote.Id); hits != 0 {
		t.Fatalf("hidden note indexed: %d", hits)
	}
	if page := search(t, env, "q="+url.QueryEscape("机密口令")); len(page.Items) != 0 {
		t.Fatalf("normal search: %+v", page.Items)
	}
	if page := search(t, env, ""); titles(page) != "购物清单" {
		t.Fatalf("normal list: %s", titles(page))
	}
	if page := search(t, env, "hidden=true"); len(page.Items) != 1 || page.Items[0].Id != hiddenNote.Id {
		t.Fatalf("hidden list: %+v", page.Items)
	}
	var tags []api.TagCount
	env.MustDo(http.MethodGet, "/notes/tags", nil, &tags)
	if len(tags) != 1 || tags[0].Tag != "生活" {
		t.Fatalf("normal tags: %+v", tags)
	}
	env.MustDo(http.MethodGet, "/notes/tags?hidden=true", nil, &tags)
	if len(tags) != 1 || tags[0].Tag != "私人" || tags[0].Count != 1 {
		t.Fatalf("hidden tags: %+v", tags)
	}
	past := time.Now().UTC().Add(-time.Second)
	if _, err := env.App.Deps.DB.Exec(`UPDATE sessions SET vault_until = ?`, past); err != nil {
		t.Fatal(err)
	}
	status, _ = env.Do(http.MethodGet, "/notes/"+noteID(hiddenNote.Id), nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("expired get: %d", status)
	}
	if page := search(t, env, "hidden=true"); len(page.Items) != 0 {
		t.Fatalf("expired hidden list: %+v", page.Items)
	}
	env.MustDo(http.MethodPost, "/vault/unlock", map[string]string{"password": "vault-secret"}, nil)
	body := "追加后的机密口令"
	env.MustDo(http.MethodPatch, "/notes/"+noteID(hiddenNote.Id), api.UpdateNote{Body: &body}, nil)
	if hits := ftsHits(t, env, hiddenNote.Id); hits != 0 {
		t.Fatalf("updated hidden note indexed: %d", hits)
	}
	status, image := uploadFile(t, env, hiddenNote.Id, "secret.txt", []byte("secret-file"))
	if status != http.StatusCreated {
		t.Fatalf("upload while unlocked: %d", status)
	}
	for {
		select {
		case <-ch:
		default:
			goto locked
		}
	}
locked:
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	status, _ = env.Do(http.MethodGet, "/notes/"+noteID(hiddenNote.Id), nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("locked get: %d", status)
	}
	if page := search(t, env, "q="+url.QueryEscape("机密口令")); len(page.Items) != 0 {
		t.Fatalf("locked search: %+v", page.Items)
	}
	resp, err := env.Client.Get(env.Server.URL + image.Url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("locked attachment: %d", resp.StatusCode)
	}
	status, _ = env.Do(http.MethodPost, "/notes/"+noteID(hiddenNote.Id)+"/to-issue", map[string]int64{"projectId": 1}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("locked to-issue: %d", status)
	}
	env.MustDo(http.MethodPost, "/vault/unlock", map[string]string{"password": "vault-secret"}, nil)
	resp, err = env.Client.Get(env.Server.URL + image.Url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("unlocked attachment: %d %s", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	status, _ = env.Do(http.MethodPost, "/notes/"+noteID(hiddenNote.Id)+"/to-issue", map[string]int64{"projectId": 1}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unlocked to-issue: %d", status)
	}
	until := time.Now().Add(time.Minute)
	ctx := auth.WithSession(context.Background(), &auth.Session{Username: testutil.Username, VaultUntil: &until})
	out, err := env.App.Deps.Actions.Run(ctx, "notes.search", json.RawMessage(`{"q":"机密口令"}`))
	if err != nil {
		t.Fatal(err)
	}
	found, ok := out.([]api.NoteSummary)
	if !ok || len(found) != 0 {
		t.Fatalf("action search: %#v", out)
	}
	if _, err := env.App.Deps.Actions.Run(ctx, "notes.append", json.RawMessage(`{"id":`+noteID(hiddenNote.Id)+`,"text":"leak"}`)); err == nil {
		t.Fatal("action append changed a hidden note")
	}
	restored := false
	env.MustDo(http.MethodPatch, "/notes/"+noteID(hiddenNote.Id), api.UpdateNote{Hidden: &restored}, nil)
	select {
	case ev := <-ch:
		assertHiddenEvent(t, ev, hiddenNote.Id)
	case <-time.After(time.Second):
		t.Fatal("missing restore event")
	}
	var restoredNote api.Note
	env.MustDo(http.MethodGet, "/notes/"+noteID(hiddenNote.Id), nil, &restoredNote)
	if restoredNote.Hidden != nil || !restoredNote.Pinned || len(restoredNote.Tags) != 1 || restoredNote.Tags[0] != "私人" || restoredNote.Body != body {
		t.Fatalf("restored: %+v", restoredNote)
	}
	if hits := ftsHits(t, env, hiddenNote.Id); hits != 1 {
		t.Fatalf("restored note index: %d", hits)
	}
	var audits int
	if err := env.App.Deps.DB.QueryRow(`SELECT count(*) FROM audit_log WHERE action IN ('note.hide', 'note.restore') AND target = ? AND detail NOT LIKE '%保险箱%' AND detail NOT LIKE '%机密%'`, noteID(hiddenNote.Id)).Scan(&audits); err != nil || audits < 2 {
		t.Fatalf("audits: %d %v", audits, err)
	}
}

func ftsHits(t *testing.T, env *testutil.Env, id int64) int {
	t.Helper()
	var n int
	if err := env.App.Deps.DB.QueryRow(`SELECT count(*) FROM notes_fts WHERE notes_fts MATCH '"机密口令"' AND rowid = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
