package notes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type fakeNoteLLM struct{ *llm.Fake }

func (f *fakeNoteLLM) Available(context.Context) bool { return true }
func (f *fakeNoteLLM) CompleteText(ctx context.Context, purpose, system, user string) (string, error) {
	result, err := f.Complete(ctx, llm.Request{Purpose: purpose, System: system, Messages: []llm.Message{{Role: "user", Content: user}}})
	return result.Text, err
}
func (f *fakeNoteLLM) CompleteJSON(ctx context.Context, purpose, system, user string, schema json.RawMessage, out any) error {
	result, err := f.Complete(ctx, llm.Request{Purpose: purpose, System: system, Messages: []llm.Message{{Role: "user", Content: user}}, JSONSchema: schema})
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(result.Text), out)
}

type manualDelay struct {
	mu    sync.Mutex
	tasks []func()
}

func (d *manualDelay) Schedule(delay time.Duration, fn func()) func() {
	if delay != 10*time.Second {
		panic("unexpected AI delay")
	}
	d.mu.Lock()
	index := len(d.tasks)
	d.tasks = append(d.tasks, fn)
	d.mu.Unlock()
	return func() { d.mu.Lock(); d.tasks[index] = nil; d.mu.Unlock() }
}

func (d *manualDelay) Fire() {
	d.mu.Lock()
	tasks := append([]func(){}, d.tasks...)
	d.tasks = nil
	d.mu.Unlock()
	for _, task := range tasks {
		if task != nil {
			task()
		}
	}
}

func setupNoteAI(t *testing.T) (*testutil.Env, *manualDelay, *fakeNoteLLM) {
	t.Helper()
	env := testutil.New(t)
	mod, ok := module.Lookup[*notes.Module](env.App.Deps.Registry, "notes.module")
	if !ok {
		t.Fatal("notes module missing")
	}
	delay := &manualDelay{}
	mod.SetAIDelayForTest(delay.Schedule)
	fake := &fakeNoteLLM{Fake: llm.NewFake()}
	module.Provide[contracts.LLM](env.App.Deps.Registry, contracts.LLMKey, fake)
	return env, delay, fake
}

func TestNoteAITitleAndSuggestions(t *testing.T) {
	env, delay, fake := setupNoteAI(t)
	body := strings.Repeat("这是一篇需要自动整理标题和标签的笔记。", 8)
	fake.Queue(llm.Result{Text: `{"title":"自动标题","tags":["工作","计划"]}`}, nil)
	var created api.Note
	env.MustDo("POST", "/notes", api.CreateNote{Body: &body}, &created)
	if created.Title != "" {
		t.Fatal("title appeared before delay")
	}
	delay.Fire()
	var updated api.Note
	env.MustDo("GET", "/notes/"+formatID(created.Id), nil, &updated)
	if updated.Title != "自动标题" || updated.SuggestedTags == nil || len(*updated.SuggestedTags) != 2 || len(updated.Tags) != 0 || len(fake.Calls) != 1 || fake.Calls[0].Purpose != "fast" {
		t.Fatalf("AI result: %+v calls=%+v", updated, fake.Calls)
	}
	env.MustDo("PATCH", "/notes/"+formatID(created.Id), map[string]any{"tags": []string{"工作"}}, &updated)
	if updated.SuggestedTags == nil || len(*updated.SuggestedTags) != 1 || (*updated.SuggestedTags)[0] != "计划" {
		t.Fatalf("selected suggestion was not removed: %+v", updated.SuggestedTags)
	}
	env.MustDo("DELETE", "/notes/"+formatID(created.Id)+"/suggested-tags", nil, nil)
	updated = api.Note{}
	env.MustDo("GET", "/notes/"+formatID(created.Id), nil, &updated)
	if updated.SuggestedTags != nil {
		t.Fatalf("dismissed: %+v", updated.SuggestedTags)
	}
	delay.Fire()
	if len(fake.Calls) != 1 {
		t.Fatalf("repeated AI calls: %d", len(fake.Calls))
	}
}

func TestNoteAIHiddenAndUserTitle(t *testing.T) {
	env, delay, fake := setupNoteAI(t)
	body := strings.Repeat("笔记正文足够长，可以触发自动标签，但不能覆盖用户标题。", 7)
	fake.Queue(llm.Result{Text: `{"title":"模型标题","tags":["已知标签"]}`}, nil)
	var created api.Note
	env.MustDo("POST", "/notes", api.CreateNote{Title: str("用户标题"), Body: &body}, &created)
	delay.Fire()
	var updated api.Note
	env.MustDo("GET", "/notes/"+formatID(created.Id), nil, &updated)
	if updated.Title != "用户标题" || updated.SuggestedTags == nil || len(fake.Calls) != 1 {
		t.Fatalf("user title: %+v", updated)
	}
	env.MustDo("PATCH", "/notes/"+formatID(created.Id), map[string]any{"body": body + "小修改"}, nil)
	delay.Fire()
	if len(fake.Calls) != 1 {
		t.Fatalf("minor edit retriggered: %d", len(fake.Calls))
	}
	unlockVault(t, env)
	env.MustDo("PATCH", "/notes/"+formatID(created.Id), map[string]any{"hidden": true}, nil)
	delay.Fire()
	if len(fake.Calls) != 1 {
		t.Fatal("hidden note sent to AI")
	}
	hidden := true
	env.MustDo("POST", "/notes", api.CreateNote{Body: &body, Hidden: &hidden}, nil)
	delay.Fire()
	if len(fake.Calls) != 1 {
		t.Fatal("new hidden note sent to AI")
	}
}

func TestNoteAIApplySettings(t *testing.T) {
	env, delay, fake := setupNoteAI(t)
	var settings api.NoteAiSettings
	env.MustDo("GET", "/notes/ai-settings", nil, &settings)
	if !settings.Available || !settings.AutoTitle || !settings.AutoTags || settings.TagMode != "suggest" {
		t.Fatalf("defaults: %+v", settings)
	}
	env.MustDo("PUT", "/notes/ai-settings", map[string]any{"autoTitle": false, "tagMode": "apply"}, &settings)
	body := strings.Repeat("内容需要自动贴标签，设置成直接应用。", 8)
	fake.Queue(llm.Result{Text: `{"title":"不会使用","tags":["直接标签"]}`}, nil)
	var created api.Note
	env.MustDo("POST", "/notes", api.CreateNote{Body: &body}, &created)
	delay.Fire()
	var updated api.Note
	env.MustDo("GET", "/notes/"+formatID(created.Id), nil, &updated)
	if updated.Title != "" || len(updated.Tags) != 1 || updated.Tags[0] != "直接标签" || updated.SuggestedTags != nil {
		t.Fatalf("apply mode: %+v", updated)
	}
	if status, _ := env.Do(http.MethodPut, "/notes/ai-settings", map[string]any{"tagMode": "invalid"}, nil); status != 400 {
		t.Fatalf("invalid mode: %d", status)
	}
}

func formatID(id int64) string { return strconv.FormatInt(id, 10) }
