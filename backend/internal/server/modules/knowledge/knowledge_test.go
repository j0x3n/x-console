package knowledge_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/knowledge"
	"github.com/j0x3n/x-console/backend/internal/server/modules/knowledge/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// words are the dimensions of the fake embedding: a text is a count of these.
var words = []string{"证书", "端口", "咖啡", "跑步", "数据库", "旅行"}

// fakeAI is both the embedding and the answering boundary.
type fakeAI struct {
	mu        sync.Mutex
	available bool
	err       error
	inputs    [][]string // one entry per Embed call
	models    []string
	providers []int64        // the provider of each Embed call
	known     map[int64]bool // providers other than 0 that exist
	prompts   []string
	answer    string
	answerErr error
}

func (f *fakeAI) Available(context.Context) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.available
}

// fakeEmbedder is fakeAI seen as a contracts.Embedder. Its Available takes a
// provider, unlike the LLM's.
type fakeEmbedder struct{ *fakeAI }

func (f fakeEmbedder) Available(_ context.Context, provider int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.available && (provider == 0 || f.known[provider])
}

func (f fakeEmbedder) Embed(_ context.Context, provider int64, model string, inputs []string) ([][]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs = append(f.inputs, inputs)
	f.models = append(f.models, model)
	f.providers = append(f.providers, provider)
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(inputs))
	for i, in := range inputs {
		v := make([]float32, len(words)+1)
		for j, w := range words {
			v[j] = float32(strings.Count(in, w))
		}
		v[len(words)] = 0.01 // never a zero vector
		out[i] = v
	}
	return out, nil
}

func (f *fakeAI) CompleteText(_ context.Context, _, _, user string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompts = append(f.prompts, user)
	return f.answer, f.answerErr
}

func (f *fakeAI) CompleteJSON(context.Context, string, string, string, json.RawMessage, any) error {
	return errors.New("unused")
}

// embedded counts the texts sent to the embedding endpoint. A search sends the question too.
func (f *fakeAI) embedded() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, in := range f.inputs {
		n += len(in)
	}
	return n
}

func (f *fakeAI) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs, f.models, f.providers, f.prompts = nil, nil, nil, nil
}

type hiddenSet struct{ set map[string]bool }

func (h hiddenSet) Hidden(_ context.Context, m string) bool { return h.set[m] }

type fixture struct {
	env *testutil.Env
	m   *knowledge.Module
	ai  *fakeAI
}

func setup(t *testing.T) *fixture {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*knowledge.Module](env.App.Deps.Registry, knowledge.ServiceKey)
	if !ok {
		t.Fatal("knowledge module not registered")
	}
	ai := &fakeAI{available: true, answer: "把 80 端口让出来。[1]", known: map[int64]bool{}}
	module.Provide[contracts.Embedder](env.App.Deps.Registry, contracts.EmbedderKey, fakeEmbedder{ai})
	module.Provide[contracts.LLM](env.App.Deps.Registry, contracts.LLMKey, ai)
	return &fixture{env: env, m: m, ai: ai}
}

func (f *fixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.env.App.Deps.DB.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) note(t *testing.T, title, body string, hidden bool) int64 {
	t.Helper()
	now := time.Now().UTC()
	h := 0
	if hidden {
		h = 1
	}
	res, err := f.env.App.Deps.DB.Exec(`INSERT INTO notes (title, body, hidden, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, title, body, h, now, now)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func (f *fixture) enable(t *testing.T, sources ...string) {
	t.Helper()
	if sources == nil {
		sources = []string{"notes", "readlater", "journal", "cards"}
	}
	f.env.MustDo(http.MethodPut, "/knowledge/settings", api.KnowledgeSettings{Enabled: true, Sources: sources}, nil)
}

func (f *fixture) sync() { knowledge.Sync(f.m, context.Background()) }

func (f *fixture) status(t *testing.T) api.KnowledgeStatus {
	t.Helper()
	var s api.KnowledgeStatus
	f.env.MustDo(http.MethodGet, "/knowledge/status", nil, &s)
	return s
}

func (f *fixture) search(t *testing.T, q string) []api.KnowledgeHit {
	t.Helper()
	var out struct{ Items []api.KnowledgeHit }
	f.env.MustDo(http.MethodGet, "/knowledge/search?q="+q, nil, &out)
	return out.Items
}

func titles(hits []api.KnowledgeHit) string {
	var s []string
	for _, h := range hits {
		s = append(s, h.Title)
	}
	return strings.Join(s, ",")
}

func TestOffByDefault(t *testing.T) {
	f := setup(t)
	f.note(t, "Caddy", "证书续不上", false)
	f.sync()
	if n := f.ai.embedded(); n != 0 || len(f.ai.inputs) != 0 {
		t.Fatalf("sent %d passages while off", n)
	}
	var s api.KnowledgeSettings
	f.env.MustDo(http.MethodGet, "/knowledge/settings", nil, &s)
	if s.Enabled || len(s.Sources) != 4 {
		t.Fatalf("defaults: %+v", s)
	}
	for _, c := range []struct{ method, path string }{{http.MethodGet, "/knowledge/search?q=证书"}, {http.MethodPost, "/knowledge/sync"}} {
		if code, _ := f.env.Do(c.method, c.path, nil, nil); code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
			t.Errorf("%s while off: %d", c.path, code)
		}
	}
	if code, _ := f.env.Do(http.MethodPost, "/knowledge/ask", map[string]any{"question": "证书"}, nil); code == http.StatusOK {
		t.Error("ask while off")
	}
}

func TestSettingsValidation(t *testing.T) {
	f := setup(t)
	bad := []api.KnowledgeSettings{
		{Enabled: true, Sources: []string{"mail"}},
		{Enabled: true, Model: strings.Repeat("m", 101), Sources: []string{}},
	}
	for i, b := range bad {
		if code, _ := f.env.Do(http.MethodPut, "/knowledge/settings", b, nil); code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
			t.Errorf("case %d: %d", i, code)
		}
	}
	var got api.KnowledgeSettings
	f.env.MustDo(http.MethodPut, "/knowledge/settings", api.KnowledgeSettings{Enabled: true, Model: " m1 ", Sources: []string{"cards", "notes", "notes"}}, &got)
	if got.Model != "m1" || strings.Join(got.Sources, ",") != "notes,cards" {
		t.Fatalf("saved: %+v", got)
	}
}

func TestIndexAndAsk(t *testing.T) {
	f := setup(t)
	f.note(t, "Caddy 证书", "Caddy 续证书失败，原因是 80 端口被占用，证书验证走 80 端口。", false)
	f.note(t, "咖啡", "每天一杯咖啡，不超过两杯。", false)
	f.note(t, "秘密", "证书私钥放在这里，端口也在这里。", true)
	f.enable(t)
	f.sync()

	st := f.status(t)
	if !st.Enabled || !st.Available || st.Docs != 2 || st.Chunks != 2 || st.LastSyncAt == nil || st.LastError != nil || st.Syncing {
		t.Fatalf("status: %+v", st)
	}
	if len(st.Sources) != 1 || st.Sources[0].Source != "notes" || st.Sources[0].Docs != 2 {
		t.Fatalf("sources: %+v", st.Sources)
	}
	// The title goes into the text that is embedded, and the model is the default one.
	if got := f.ai.inputs[0][0]; !strings.HasPrefix(got, "Caddy 证书\n") {
		t.Errorf("input: %q", got)
	}

	var ans api.KnowledgeAnswer
	f.env.MustDo(http.MethodPost, "/knowledge/ask", map[string]any{"question": "证书和端口的问题"}, &ans)
	if ans.Answer == nil || !strings.Contains(*ans.Answer, "[1]") || len(ans.Sources) != 1 || ans.Sources[0].Title != "Caddy 证书" || ans.Sources[0].Score < 0.5 {
		t.Fatalf("answer: %+v", ans)
	}
	if !strings.Contains(ans.Sources[0].Link, "/notes/") || !strings.Contains(ans.Sources[0].Snippet, "80 端口被占用") {
		t.Errorf("source: %+v", ans.Sources[0])
	}
	// Only the matching passage and never the hidden note reach the answering AI.
	prompt := f.ai.prompts[len(f.ai.prompts)-1]
	if !strings.Contains(prompt, "[1] Caddy 证书") || strings.Contains(prompt, "私钥") || strings.Contains(prompt, "每天一杯") {
		t.Errorf("prompt: %q", prompt)
	}
	if hits := f.search(t, "证书"); titles(hits) != "Caddy 证书" {
		t.Errorf("search: %s", titles(hits))
	}
	// A question that matches nothing says so without asking the AI.
	f.ai.reset()
	f.env.MustDo(http.MethodPost, "/knowledge/ask", map[string]any{"question": "旅行计划"}, &ans)
	if ans.Answer == nil || !strings.Contains(*ans.Answer, "没有找到") || len(ans.Sources) != 0 || len(f.ai.prompts) != 0 {
		t.Fatalf("no match: %+v prompts %d", ans, len(f.ai.prompts))
	}
	// Without an answering AI the passages are still returned.
	f.ai.mu.Lock()
	f.ai.answer = ""
	f.ai.mu.Unlock()
	ans = api.KnowledgeAnswer{}
	f.env.MustDo(http.MethodPost, "/knowledge/ask", map[string]any{"question": "咖啡"}, &ans)
	if ans.Answer != nil || len(ans.Sources) != 1 || ans.Sources[0].Title != "咖啡" {
		t.Fatalf("empty answer: %+v", ans)
	}
	f.ai.mu.Lock()
	f.ai.answerErr = errors.New("boom")
	f.ai.mu.Unlock()
	ans = api.KnowledgeAnswer{}
	f.env.MustDo(http.MethodPost, "/knowledge/ask", map[string]any{"question": "咖啡"}, &ans)
	if ans.Answer != nil || len(ans.Sources) != 1 {
		t.Fatalf("answer error: %+v", ans)
	}
}

func TestQuestionValidation(t *testing.T) {
	f := setup(t)
	f.enable(t)
	for _, q := range []string{"", "   ", strings.Repeat("问", 501)} {
		if code, _ := f.env.Do(http.MethodPost, "/knowledge/ask", map[string]any{"question": q}, nil); code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
			t.Errorf("question %q: %d", q[:min(len(q), 6)], code)
		}
	}
	f.ai.mu.Lock()
	f.ai.available = false
	f.ai.mu.Unlock()
	if code, _ := f.env.Do(http.MethodGet, "/knowledge/search?q=证书", nil, nil); code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
		t.Errorf("no provider: %d", code)
	}
	if st := f.status(t); st.Available {
		t.Error("available without a provider")
	}
}

func TestIncrementalSync(t *testing.T) {
	f := setup(t)
	a := f.note(t, "甲", "证书一", false)
	f.note(t, "乙", "端口二", false)
	f.enable(t)
	f.sync()
	if n := f.ai.embedded(); n != 2 {
		t.Fatalf("first sync embedded %d", n)
	}
	// Nothing changed: nothing is sent.
	f.ai.reset()
	f.sync()
	if n := f.ai.embedded(); n != 0 {
		t.Fatalf("second sync embedded %d", n)
	}
	// One note changed: only that one.
	f.exec(t, `UPDATE notes SET body = ? WHERE id = ?`, "证书一，证书续期", a)
	f.sync()
	if n := f.ai.embedded(); n != 1 {
		t.Fatalf("after edit embedded %d", n)
	}
	// A title change alone is a content change too, because the title is part of the text.
	f.ai.reset()
	f.exec(t, `UPDATE notes SET title = '甲甲' WHERE id = ?`, a)
	f.sync()
	if n := f.ai.embedded(); n != 1 {
		t.Fatalf("after retitle embedded %d", n)
	}
	// Deleted, archived and hidden notes leave the index.
	f.exec(t, `UPDATE notes SET archived_at = ? WHERE id = ?`, time.Now().UTC(), a)
	f.sync()
	if st := f.status(t); st.Docs != 1 {
		t.Fatalf("after archive: %+v", st)
	}
	f.exec(t, `UPDATE notes SET archived_at = NULL, hidden = 1 WHERE id = ?`, a)
	f.sync()
	if st := f.status(t); st.Docs != 1 || titles(f.search(t, "证书")) != "" {
		t.Fatalf("after hide: %+v", st)
	}
	f.exec(t, `DELETE FROM notes`)
	f.sync()
	if st := f.status(t); st.Docs != 0 || st.Chunks != 0 {
		t.Fatalf("after delete: %+v", st)
	}
	var left int
	if err := f.env.App.Deps.DB.QueryRow(`SELECT count(*) FROM knowledge_chunks`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("chunks left: %d %v", left, err)
	}
	// An empty note is not indexed.
	f.note(t, "", "   ", false)
	f.sync()
	if st := f.status(t); st.Docs != 0 {
		t.Fatalf("empty note: %+v", st)
	}
}

func TestModelChangeAndSourcesOff(t *testing.T) {
	f := setup(t)
	f.note(t, "甲", "证书", false)
	f.exec(t, `INSERT INTO journal_diary (day, body, updated_at) VALUES ('2026-11-11', '今天去跑步，咖啡很好', ?)`, time.Now().UTC())
	f.enable(t)
	f.sync()
	if st := f.status(t); st.Docs != 2 || len(st.Sources) != 2 {
		t.Fatalf("status: %+v", st)
	}
	// Another model rebuilds everything, and the old model's passages are not searched meanwhile.
	f.env.MustDo(http.MethodPut, "/knowledge/settings", api.KnowledgeSettings{Enabled: true, Model: "other", Sources: []string{"notes", "journal"}}, nil)
	f.ai.reset()
	if hits := f.search(t, "证书"); len(hits) != 0 {
		t.Errorf("old model searched: %s", titles(hits))
	}
	f.ai.reset()
	f.sync()
	if n := f.ai.embedded(); n != 2 {
		t.Fatalf("rebuild embedded %d", n)
	}
	for _, m := range f.ai.models {
		if m != "other" {
			t.Errorf("model %q", m)
		}
	}
	if hits := f.search(t, "证书"); titles(hits) != "甲" {
		t.Errorf("after rebuild: %s", titles(hits))
	}
	// Switching a source off removes its index at the next sync.
	f.env.MustDo(http.MethodPut, "/knowledge/settings", api.KnowledgeSettings{Enabled: true, Model: "other", Sources: []string{"notes"}}, nil)
	if hits := f.search(t, "咖啡"); len(hits) != 0 {
		t.Errorf("switched-off source still searched: %s", titles(hits))
	}
	f.sync()
	if st := f.status(t); st.Docs != 1 || st.Sources[0].Source != "notes" {
		t.Fatalf("after switch off: %+v", st)
	}
	// Switching the whole thing off stops sending.
	f.env.MustDo(http.MethodPut, "/knowledge/settings", api.KnowledgeSettings{Enabled: false, Sources: []string{"notes"}}, nil)
	f.ai.reset()
	f.note(t, "新", "端口", false)
	f.sync()
	if len(f.ai.inputs) != 0 {
		t.Fatal("sent while off")
	}
}

// B197: embeddings can use another provider than fast AI, for example Google.
func TestProviderChoice(t *testing.T) {
	f := setup(t)
	seven := int64(7)
	// A provider that does not exist is refused.
	if code, _ := f.env.Do(http.MethodPut, "/knowledge/settings", api.KnowledgeSettings{Enabled: true, ProviderId: &seven, Sources: []string{"notes"}}, nil); code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown provider: %d", code)
	}
	// 0 means the fast AI provider and is saved as empty.
	zero := int64(0)
	var got api.KnowledgeSettings
	f.env.MustDo(http.MethodPut, "/knowledge/settings", api.KnowledgeSettings{Enabled: true, ProviderId: &zero, Sources: []string{"notes"}}, &got)
	if got.ProviderId != nil {
		t.Fatalf("zero saved as %d", *got.ProviderId)
	}
	f.note(t, "甲", "证书", false)
	f.sync()
	// Another provider rebuilds everything, and the old vectors are not searched meanwhile.
	f.ai.known[7] = true
	f.env.MustDo(http.MethodPut, "/knowledge/settings", api.KnowledgeSettings{Enabled: true, ProviderId: &seven, Sources: []string{"notes"}}, &got)
	if got.ProviderId == nil || *got.ProviderId != 7 {
		t.Fatalf("saved: %+v", got)
	}
	f.ai.reset()
	if hits := f.search(t, "证书"); len(hits) != 0 {
		t.Errorf("old provider searched: %s", titles(hits))
	}
	f.ai.reset()
	f.sync()
	if n := f.ai.embedded(); n != 1 {
		t.Fatalf("rebuild embedded %d", n)
	}
	if hits := f.search(t, "证书"); titles(hits) != "甲" {
		t.Errorf("after rebuild: %s", titles(hits))
	}
	for _, p := range f.ai.providers {
		if p != 7 {
			t.Errorf("provider %d", p)
		}
	}
	// A deleted provider shows in the status, and the index stays.
	f.ai.known[7] = false
	f.sync()
	st := f.status(t)
	if st.Available || st.LastError == nil || !strings.Contains(*st.LastError, "删除") || st.Docs != 1 {
		t.Fatalf("deleted provider: %+v", st)
	}
}

func TestOtherSources(t *testing.T) {
	f := setup(t)
	now := time.Now().UTC()
	f.exec(t, `INSERT INTO read_items (url, title, content, summary, note, status, created_at, updated_at) VALUES ('https://example.com/a', 'SQLite 锁', '数据库 WAL 模式下读不阻塞写', '', '', 'ready', ?, ?)`, now, now)
	f.exec(t, `INSERT INTO read_items (url, title, content, status, created_at, updated_at) VALUES ('https://example.com/b', '没抓好', '数据库', 'failed', ?, ?)`, now, now)
	f.exec(t, `INSERT INTO journal_diary (day, body, updated_at) VALUES ('2026-11-11', '今天去跑步', ?)`, now)
	f.exec(t, `INSERT INTO cards (question, answer, due, created_at, updated_at) VALUES ('WAL 是什么', '数据库的预写日志', ?, ?, ?)`, now, now, now)
	f.exec(t, `INSERT INTO cards (question, answer, suspended, due, created_at, updated_at) VALUES ('停用的卡', '数据库', 1, ?, ?, ?)`, now, now, now)
	f.enable(t)
	f.sync()
	st := f.status(t)
	if st.Docs != 3 || len(st.Sources) != 3 {
		t.Fatalf("status: %+v", st)
	}
	hits := f.search(t, "数据库")
	if len(hits) != 2 {
		t.Fatalf("hits: %s", titles(hits))
	}
	links := hits[0].Link + " " + hits[1].Link
	if !strings.Contains(links, "https://example.com/a") || !strings.Contains(links, "/cards") {
		t.Errorf("links: %s", links)
	}
	if hits := f.search(t, "跑步"); len(hits) != 1 || hits[0].Source != "journal" || hits[0].Link != "/journal?date=2026-11-11" {
		t.Errorf("journal: %+v", hits)
	}
}

func TestHiddenModulesAreNotSearched(t *testing.T) {
	f := setup(t)
	f.note(t, "甲", "证书", false)
	f.exec(t, `INSERT INTO journal_diary (day, body, updated_at) VALUES ('2026-11-11', '证书的日记', ?)`, time.Now().UTC())
	f.exec(t, `INSERT INTO cards (question, answer, due, created_at, updated_at) VALUES ('证书卡', '答', ?, ?, ?)`, time.Now().UTC(), time.Now().UTC(), time.Now().UTC())
	f.enable(t)
	f.sync()
	if got := len(f.search(t, "证书")); got != 3 {
		t.Fatalf("visible: %d", got)
	}
	// Hiding a module hides its passages at once, without a sync. Cards follow the notes.
	h := hiddenSet{map[string]bool{"notes": true}}
	module.Provide[contracts.HiddenModules](f.env.App.Deps.Registry, contracts.HiddenModulesKey, h)
	hits := f.search(t, "证书")
	if len(hits) != 1 || hits[0].Source != "journal" {
		t.Fatalf("notes hidden: %s", titles(hits))
	}
	h.set["journal"] = true
	if got := f.search(t, "证书"); len(got) != 0 {
		t.Fatalf("all hidden: %s", titles(got))
	}
	// The assistant action follows the same rule.
	if _, err := f.env.App.Deps.Actions.Run(context.Background(), "knowledge.search", []byte(`{"query":"证书"}`)); err != nil {
		t.Fatal(err)
	}
	h.set["notes"], h.set["journal"] = false, false
	res, err := f.env.App.Deps.Actions.Run(context.Background(), "knowledge.search", []byte(`{"query":"证书"}`))
	if err != nil || len(res.(map[string]any)["items"].([]api.KnowledgeHit)) != 3 {
		t.Fatalf("action: %v %v", res, err)
	}
	if _, err := f.env.App.Deps.Actions.Run(context.Background(), "knowledge.search", []byte(`{"query":""}`)); err == nil {
		t.Error("empty query should fail")
	}
}

func TestBudgetAndErrors(t *testing.T) {
	f := setup(t)
	for i := 0; i < 5; i++ {
		f.note(t, fmt.Sprintf("笔记%d", i), "证书", false)
	}
	f.enable(t)
	knowledge.SetBudget(f.m, 3)
	f.sync()
	st := f.status(t)
	if st.Docs != 3 || st.Pending == nil || *st.Pending != 2 {
		t.Fatalf("first pass: %+v", st)
	}
	f.sync()
	if st := f.status(t); st.Docs != 5 || st.Pending != nil {
		t.Fatalf("second pass: %+v", st)
	}

	// A failing provider is reported in the status, and nothing is lost.
	f.note(t, "新的", "端口", false)
	f.ai.mu.Lock()
	f.ai.err = errors.New("这个供应商没有嵌入接口")
	f.ai.mu.Unlock()
	f.sync()
	st = f.status(t)
	if st.LastError == nil || !strings.Contains(*st.LastError, "没有嵌入接口") || st.Docs != 5 {
		t.Fatalf("error status: %+v", st)
	}
	f.ai.mu.Lock()
	f.ai.err = nil
	f.ai.mu.Unlock()
	f.sync()
	if st := f.status(t); st.LastError != nil || st.Docs != 6 {
		t.Fatalf("recovered: %+v", st)
	}
	// No provider at all.
	f.ai.mu.Lock()
	f.ai.available = false
	f.ai.mu.Unlock()
	f.sync()
	if e := knowledge.LastError(f.m); !strings.Contains(e, "供应商") {
		t.Errorf("no provider: %q", e)
	}
}

func TestSyncEndpointAndClear(t *testing.T) {
	f := setup(t)
	f.note(t, "甲", "证书", false)
	f.enable(t)
	if code, _ := f.env.Do(http.MethodPost, "/knowledge/sync", nil, nil); code != http.StatusAccepted {
		t.Fatalf("sync: %d", code)
	}
	deadline := time.Now().Add(5 * time.Second)
	for f.status(t).Docs != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("not synced: %+v", f.status(t))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if code, _ := f.env.Do(http.MethodDelete, "/knowledge/index", nil, nil); code != http.StatusNoContent {
		t.Fatalf("clear: %d", code)
	}
	if st := f.status(t); st.Docs != 0 || st.Chunks != 0 {
		t.Fatalf("after clear: %+v", st)
	}
	if hits := f.search(t, "证书"); len(hits) != 0 {
		t.Errorf("searched after clear: %s", titles(hits))
	}
}
