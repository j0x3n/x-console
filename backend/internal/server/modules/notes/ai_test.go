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
	mu     sync.Mutex
	tasks  []func()
	delays []time.Duration
}

func (d *manualDelay) Schedule(delay time.Duration, fn func()) func() {
	if delay != 3*time.Second && delay != 0 {
		panic("unexpected AI delay")
	}
	d.mu.Lock()
	index := len(d.tasks)
	d.tasks = append(d.tasks, fn)
	d.delays = append(d.delays, delay)
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

// B67: a quick note gets its title and tags at once, even when short, and
// the tags are added instead of suggested.
func TestNoteAIQuick(t *testing.T) {
	env, delay, fake := setupNoteAI(t)
	short := "明天给王总回电话"
	env.MustDo("POST", "/notes", api.CreateNote{Body: &short}, nil)
	delay.Fire()
	if len(fake.Calls) != 0 {
		t.Fatalf("short note sent to AI: %d", len(fake.Calls))
	}

	fake.Queue(llm.Result{Text: `{"title":"给王总回电话","tags":["工作"]}`}, nil)
	quick := true
	var created api.Note
	env.MustDo("POST", "/notes", api.CreateNote{Body: &short, Quick: &quick}, &created)
	delay.mu.Lock()
	last := delay.delays[len(delay.delays)-1]
	delay.mu.Unlock()
	if last != 0 {
		t.Fatalf("quick note waited %s", last)
	}
	delay.Fire()
	var updated api.Note
	env.MustDo("GET", "/notes/"+formatID(created.Id), nil, &updated)
	if updated.Title != "给王总回电话" || len(updated.Tags) != 1 || updated.Tags[0] != "工作" || updated.SuggestedTags != nil {
		t.Fatalf("quick note: %+v", updated)
	}

	hidden := true
	unlockVault(t, env)
	env.MustDo("POST", "/notes", api.CreateNote{Body: &short, Quick: &quick, Hidden: &hidden}, nil)
	delay.Fire()
	if len(fake.Calls) != 1 {
		t.Fatalf("hidden quick note sent to AI: %d", len(fake.Calls))
	}
}

func formatID(id int64) string { return strconv.FormatInt(id, 10) }

func TestNoteAIToolsOnDemand(t *testing.T) {
	env, _, fake := setupNoteAI(t)
	fake.Queue(llm.Result{Text: "```markdown\n# 周报\n\n- 完成了接口\n```"}, nil)
	var polished struct{ Body string }
	env.MustDo(http.MethodPost, "/notes/ai/polish", map[string]any{"body": "周报 完成了接口", "prompt": "改成要点列表"}, &polished)
	if polished.Body != "# 周报\n\n- 完成了接口" {
		t.Fatalf("polish: %q", polished.Body)
	}
	call := fake.Calls[len(fake.Calls)-1]
	if !strings.Contains(call.System, "改成要点列表") || call.Messages[0].Content != "周报 完成了接口" {
		t.Fatalf("polish request: %+v", call)
	}
	fake.Queue(llm.Result{Text: `{"title":"本周工作总结和下周计划安排说明文档"}`}, nil)
	var title struct{ Title string }
	env.MustDo(http.MethodPost, "/notes/ai/title", map[string]any{"body": "完成了接口"}, &title)
	if title.Title == "" || len([]rune(title.Title)) > 20 {
		t.Fatalf("title: %q", title.Title)
	}
	fake.Queue(llm.Result{Text: `{"tags":["工作","周报","计划","多余"]}`}, nil)
	var tags struct{ Tags []string }
	env.MustDo(http.MethodPost, "/notes/ai/tags", map[string]any{"body": "完成了接口"}, &tags)
	if len(tags.Tags) != 3 || tags.Tags[0] != "工作" {
		t.Fatalf("tags: %+v", tags.Tags)
	}
	// Nothing was saved: the endpoints only answer.
	var page struct{ Items []api.NoteSummary }
	env.MustDo(http.MethodGet, "/notes", nil, &page)
	if len(page.Items) != 0 {
		t.Fatalf("notes created: %+v", page.Items)
	}
	if status, _ := env.Do(http.MethodPost, "/notes/ai/polish", map[string]any{"body": "   "}, nil); status != http.StatusBadRequest {
		t.Fatalf("empty body: %d", status)
	}
}
