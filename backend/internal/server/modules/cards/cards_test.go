package cards_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/modules/cards"
	"github.com/j0x3n/x-console/backend/internal/server/modules/cards/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// clock is the fixed time of the tests: 2026-10-12 09:00 in Asia/Shanghai.
var clock = time.Date(2026, 10, 12, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))

type rig struct {
	env *testutil.Env
	m   *cards.Module
	now time.Time
}

func setup(t *testing.T) *rig {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*cards.Module](env.App.Deps.Registry, cards.ServiceKey)
	if !ok {
		t.Fatal("cards module not registered")
	}
	r := &rig{env: env, m: m, now: clock}
	cards.SetNow(m, func() time.Time { return r.now })
	return r
}

func (r *rig) days(n int) { r.now = r.now.AddDate(0, 0, n) }

func (r *rig) create(t *testing.T, q, a string) api.Card {
	t.Helper()
	var c api.Card
	r.env.MustDo(http.MethodPost, "/cards", map[string]any{"question": q, "answer": a}, &c)
	return c
}

func (r *rig) summary(t *testing.T) api.CardSummary {
	t.Helper()
	var s api.CardSummary
	r.env.MustDo(http.MethodGet, "/cards/summary", nil, &s)
	return s
}

func (r *rig) queue(t *testing.T) api.CardQueue {
	t.Helper()
	var q api.CardQueue
	r.env.MustDo(http.MethodGet, "/cards/queue", nil, &q)
	return q
}

func (r *rig) review(t *testing.T, id int64, rating int) api.CardReviewResult {
	t.Helper()
	var out api.CardReviewResult
	r.env.MustDo(http.MethodPost, fmt.Sprintf("/cards/%d/review", id), map[string]any{"rating": rating}, &out)
	return out
}

func (r *rig) settings(t *testing.T, body map[string]any) {
	t.Helper()
	r.env.MustDo(http.MethodPut, "/cards/settings", body, nil)
}

// status sends a request and returns the status and the error code.
func (r *rig) status(method, path string, body any) (int, string) {
	code, raw := r.env.Do(method, path, body, nil)
	var e struct{ Code string }
	_ = json.Unmarshal(raw, &e)
	return code, e.Code
}

func TestCreateListEditDelete(t *testing.T) {
	r := setup(t)
	var c api.Card
	r.env.MustDo(http.MethodPost, "/cards", map[string]any{
		"question": "  SQLite 的 WAL 模式有什么好处？ ", "answer": "读写互不阻塞。", "deck": "数据库",
		"sourceTitle": "WAL 笔记", "sourceLink": "/notes/3",
	}, &c)
	if c.Id == 0 || c.Question != "SQLite 的 WAL 模式有什么好处？" || c.Deck != "数据库" || c.State != api.New || c.Reps != 0 || c.SourceLink != "/notes/3" {
		t.Fatalf("created: %+v", c)
	}
	other := r.create(t, "Go 的 context 用来做什么？", "传递取消信号和超时。")
	if other.Deck != "默认" {
		t.Fatalf("default deck: %q", other.Deck)
	}

	var page api.CardPage
	r.env.MustDo(http.MethodGet, "/cards", nil, &page)
	if page.Total != 2 || len(page.Items) != 2 || page.Items[0].Id != other.Id {
		t.Fatalf("list: %+v", page)
	}
	r.env.MustDo(http.MethodGet, "/cards?q=wal", nil, &page)
	if page.Total != 1 || page.Items[0].Id != c.Id {
		t.Fatalf("search is case-insensitive: %+v", page)
	}
	r.env.MustDo(http.MethodGet, "/cards?q=%25", nil, &page)
	if page.Total != 0 {
		t.Fatalf("a percent sign is just a character: %+v", page)
	}
	r.env.MustDo(http.MethodGet, "/cards?deck=数据库", nil, &page)
	if page.Total != 1 {
		t.Fatalf("deck filter: %+v", page)
	}
	r.env.MustDo(http.MethodGet, "/cards?limit=1", nil, &page)
	if len(page.Items) != 1 || page.NextCursor == nil || page.Total != 2 {
		t.Fatalf("first page: %+v", page)
	}
	var next api.CardPage
	r.env.MustDo(http.MethodGet, "/cards?limit=1&cursor="+*page.NextCursor, nil, &next)
	if len(next.Items) != 1 || next.Items[0].Id != c.Id || next.NextCursor != nil {
		t.Fatalf("second page: %+v", next)
	}

	var edited api.Card
	r.env.MustDo(http.MethodPatch, fmt.Sprintf("/cards/%d", c.Id), map[string]any{"answer": "读和写可以同时进行。", "deck": "后端"}, &edited)
	if edited.Answer != "读和写可以同时进行。" || edited.Deck != "后端" || edited.Question != c.Question {
		t.Fatalf("edited: %+v", edited)
	}
	var decks []api.CardDeck
	r.env.MustDo(http.MethodGet, "/cards/decks", nil, &decks)
	if len(decks) != 2 || decks[0].Name != "后端" || decks[0].NewCards != 1 {
		t.Fatalf("decks: %+v", decks)
	}

	r.review(t, c.Id, 3)
	if code, _ := r.status(http.MethodDelete, fmt.Sprintf("/cards/%d", c.Id), nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := r.status(http.MethodGet, fmt.Sprintf("/cards/%d", c.Id), nil); code != http.StatusNotFound {
		t.Fatalf("deleted card: %d", code)
	}
	var left int
	if err := r.env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM card_reviews`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("review records after delete: %d %v", left, err)
	}
	if code, _ := r.status(http.MethodDelete, "/cards/9999", nil); code != http.StatusNotFound {
		t.Fatalf("delete missing: %d", code)
	}
}

func TestValidation(t *testing.T) {
	r := setup(t)
	for name, body := range map[string]map[string]any{
		"empty question": {"question": "  ", "answer": "a"},
		"empty answer":   {"question": "q", "answer": ""},
		"long question":  {"question": strings.Repeat("字", 4001), "answer": "a"},
		"bad link":       {"question": "q", "answer": "a", "sourceLink": "javascript:alert(1)"},
		"protocol link":  {"question": "q", "answer": "a", "sourceLink": "//evil.example"},
		"long deck":      {"question": "q", "answer": "a", "deck": strings.Repeat("牌", 61)},
	} {
		if code, _ := r.status(http.MethodPost, "/cards", body); code != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, code)
		}
	}
	c := r.create(t, "q", "a")
	if code, _ := r.status(http.MethodPatch, fmt.Sprintf("/cards/%d", c.Id), map[string]any{"question": ""}); code != http.StatusBadRequest {
		t.Errorf("empty patch: %d", code)
	}
	if code, _ := r.status(http.MethodPatch, "/cards/9999", map[string]any{"question": "x"}); code != http.StatusNotFound {
		t.Errorf("patch missing: %d", code)
	}
	for _, rating := range []int{0, 5} {
		if code, _ := r.status(http.MethodPost, fmt.Sprintf("/cards/%d/review", c.Id), map[string]any{"rating": rating}); code != http.StatusBadRequest {
			t.Errorf("rating %d: %d", rating, code)
		}
	}
	if code, _ := r.status(http.MethodPost, "/cards/9999/review", map[string]any{"rating": 3}); code != http.StatusNotFound {
		t.Errorf("review missing: %d", code)
	}
}

func TestReviewMovesDueForward(t *testing.T) {
	r := setup(t)
	var ids []int64
	for i := 0; i < 4; i++ {
		ids = append(ids, r.create(t, fmt.Sprintf("q%d", i), "a").Id)
	}
	q := r.queue(t)
	if len(q.Items) != 4 {
		t.Fatalf("queue: %d", len(q.Items))
	}
	iv := q.Items[0].Intervals
	if !(iv.Again < iv.Hard && iv.Hard < iv.Good && iv.Good < iv.Easy) {
		t.Fatalf("intervals: %+v", iv)
	}
	// Each rating on its own new card: the due dates grow with the rating.
	var due []time.Time
	for i, id := range ids {
		res := r.review(t, id, i+1)
		if res.Card.State != api.Review || res.Card.Reps != 1 || res.Card.LastReview == nil {
			t.Fatalf("after rating %d: %+v", i+1, res.Card)
		}
		want := []int{iv.Again, iv.Hard, iv.Good, iv.Easy}[i]
		if res.IntervalDays != want {
			t.Fatalf("rating %d: interval %d, preview said %d", i+1, res.IntervalDays, want)
		}
		due = append(due, res.Card.Due)
	}
	for i := 1; i < len(due); i++ {
		if !due[i].After(due[i-1]) {
			t.Fatalf("due dates must grow with the rating: %v", due)
		}
	}
	if s := r.summary(t); s.Due != 0 || s.LearnedToday != 4 || s.Total != 4 || s.NextDue == nil {
		t.Fatalf("summary after reviewing everything: %+v", s)
	}

	// Three days later the Good card is due, the Again card too (it was due
	// after one day), the Hard card is due after two, but not the Easy card.
	r.days(3)
	s := r.summary(t)
	if s.DueReview != 3 || s.DueNew != 0 {
		t.Fatalf("three days later: %+v", s)
	}
	failed := r.review(t, ids[2], 1)
	if failed.Card.Lapses != 1 || failed.Card.Reps != 2 {
		t.Fatalf("Again must count a lapse: %+v", failed.Card)
	}
	if s := r.summary(t); s.DueReview != 2 || s.ReviewedToday != 1 {
		t.Fatalf("after one review: %+v", s)
	}
}

func TestDailyLimits(t *testing.T) {
	r := setup(t)
	var ids []int64
	for i := 0; i < 6; i++ {
		ids = append(ids, r.create(t, fmt.Sprintf("q%d", i), "a").Id)
	}
	r.settings(t, map[string]any{"newPerDay": 3})
	if s := r.summary(t); s.DueNew != 3 || s.Due != 3 || s.NewCards != 6 {
		t.Fatalf("new limit: %+v", s)
	}
	q := r.queue(t)
	if len(q.Items) != 3 || q.Items[0].Card.Id != ids[0] {
		t.Fatalf("queue under the new limit: %d", len(q.Items))
	}
	for _, it := range q.Items {
		r.review(t, it.Card.Id, 3)
	}
	if s := r.summary(t); s.DueNew != 0 || s.LearnedToday != 3 {
		t.Fatalf("limit used up: %+v", s)
	}
	if len(r.queue(t).Items) != 0 {
		t.Fatal("no new card may be left after the limit")
	}
	r.settings(t, map[string]any{"newPerDay": 5})
	if s := r.summary(t); s.DueNew != 2 {
		t.Fatalf("raising the limit: %+v", s)
	}
	r.settings(t, map[string]any{"newPerDay": 0})
	if s := r.summary(t); s.DueNew != 0 {
		t.Fatalf("zero new cards: %+v", s)
	}

	// Review limit: three learned cards come back in three days.
	r.settings(t, map[string]any{"newPerDay": 10, "reviewPerDay": 2})
	r.days(3)
	if s := r.summary(t); s.DueReview != 2 || s.DueNew != 3 {
		t.Fatalf("review limit: %+v", s)
	}
	q = r.queue(t)
	if len(q.Items) != 5 || q.Items[0].Card.State != api.Review || q.Items[2].Card.State != api.New {
		t.Fatalf("old cards come before new ones: %+v", q.Items)
	}
	r.review(t, q.Items[0].Card.Id, 3)
	r.review(t, q.Items[1].Card.Id, 3)
	if s := r.summary(t); s.DueReview != 0 || s.ReviewedToday != 2 {
		t.Fatalf("review limit used up: %+v", s)
	}
	// The next day the third old card is still waiting.
	r.days(1)
	if s := r.summary(t); s.DueReview < 1 {
		t.Fatalf("overdue cards come back the next day: %+v", s)
	}
}

func TestSuspendedAndStaleReviews(t *testing.T) {
	r := setup(t)
	a := r.create(t, "q1", "a")
	b := r.create(t, "q2", "a")
	var patched api.Card
	r.env.MustDo(http.MethodPatch, fmt.Sprintf("/cards/%d", b.Id), map[string]any{"suspended": true}, &patched)
	if !patched.Suspended {
		t.Fatal("not suspended")
	}
	if s := r.summary(t); s.Due != 1 || s.Suspended != 1 || s.Total != 2 {
		t.Fatalf("a suspended card is not due: %+v", s)
	}
	if code, c := r.status(http.MethodPost, fmt.Sprintf("/cards/%d/review", b.Id), map[string]any{"rating": 3}); code != http.StatusConflict || c != "card_suspended" {
		t.Fatalf("review of a suspended card: %d %s", code, c)
	}
	var page api.CardPage
	r.env.MustDo(http.MethodGet, "/cards?suspended=true", nil, &page)
	if page.Total != 1 || page.Items[0].Id != b.Id {
		t.Fatalf("suspended filter: %+v", page)
	}

	path := fmt.Sprintf("/cards/%d/review", a.Id)
	if code, _ := r.status(http.MethodPost, path, map[string]any{"rating": 3, "reps": 0}); code != http.StatusOK {
		t.Fatalf("first review: %d", code)
	}
	if code, c := r.status(http.MethodPost, path, map[string]any{"rating": 3, "reps": 0}); code != http.StatusConflict || c != "card_stale" {
		t.Fatalf("second review with the same reps: %d %s", code, c)
	}
	var n int
	if err := r.env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM card_reviews WHERE card_id = ?`, a.Id).Scan(&n); err != nil || n != 1 {
		t.Fatalf("review records: %d %v", n, err)
	}
	// Without reps the check is skipped.
	if code, _ := r.status(http.MethodPost, path, map[string]any{"rating": 3}); code != http.StatusOK {
		t.Fatalf("review without reps: %d", code)
	}
}

func TestSettings(t *testing.T) {
	r := setup(t)
	var s api.CardSettings
	r.env.MustDo(http.MethodGet, "/cards/settings", nil, &s)
	if s.NewPerDay != 10 || s.ReviewPerDay != 100 || s.Retention != 0.9 || !s.PushEnabled || s.PushTime != "20:00" || !s.AiEnabled || s.AiAvailable {
		t.Fatalf("defaults: %+v", s)
	}
	r.env.MustDo(http.MethodPut, "/cards/settings", map[string]any{"retention": 0.85, "pushTime": "21:30", "pushEnabled": false}, &s)
	if s.Retention != 0.85 || s.PushTime != "21:30" || s.PushEnabled || s.NewPerDay != 10 {
		t.Fatalf("saved: %+v", s)
	}
	for name, body := range map[string]map[string]any{
		"retention low":  {"retention": 0.5},
		"retention high": {"retention": 0.99},
		"new limit":      {"newPerDay": -1},
		"review limit":   {"reviewPerDay": 99999},
		"time":           {"pushTime": "8点"},
		"time range":     {"pushTime": "25:00"},
	} {
		if code, _ := r.status(http.MethodPut, "/cards/settings", body); code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	// A higher target retention gives shorter intervals.
	c := r.create(t, "q", "a")
	r.settings(t, map[string]any{"retention": 0.97})
	high := r.queue(t).Items[0].Intervals
	r.settings(t, map[string]any{"retention": 0.7})
	low := r.queue(t).Items[0].Intervals
	if c.Id == 0 || low.Easy <= high.Easy {
		t.Fatalf("retention 70%% %+v vs 97%% %+v", low, high)
	}
}

// fakeLLM is the AI boundary with queued answers.
type fakeLLM struct{ *llm.Fake }

func (f *fakeLLM) Available(context.Context) bool { return true }
func (f *fakeLLM) CompleteText(ctx context.Context, purpose, system, user string) (string, error) {
	res, err := f.Complete(ctx, llm.Request{Purpose: purpose, System: system, Messages: []llm.Message{{Role: "user", Content: user}}})
	return res.Text, err
}
func (f *fakeLLM) CompleteJSON(ctx context.Context, purpose, system, user string, schema json.RawMessage, out any) error {
	res, err := f.Complete(ctx, llm.Request{Purpose: purpose, System: system, Messages: []llm.Message{{Role: "user", Content: user}}, JSONSchema: schema})
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(res.Text), out)
}

func TestGenerateDraft(t *testing.T) {
	r := setup(t)
	text := "WAL 模式把写入先追加到日志文件，读操作仍然读主数据库文件，所以读写互不阻塞。"

	// No model is set up in the test server.
	if code, c := r.status(http.MethodPost, "/cards/generate", map[string]any{"text": text}); code != http.StatusConflict || c != "ai_not_configured" {
		t.Fatalf("without a model: %d %s", code, c)
	}

	fake := &fakeLLM{Fake: llm.NewFake()}
	module.Provide[contracts.LLM](r.env.App.Deps.Registry, contracts.LLMKey, fake)
	fake.Queue(llm.Result{Text: `{"question":"WAL 模式下读写为什么互不阻塞？","answer":"写入先追加到日志文件，读仍然读主文件。"}`}, nil)
	var draft api.CardDraft
	r.env.MustDo(http.MethodPost, "/cards/generate", map[string]any{"text": text, "sourceTitle": "SQLite 笔记"}, &draft)
	if draft.Question != "WAL 模式下读写为什么互不阻塞？" || draft.Answer == "" {
		t.Fatalf("draft: %+v", draft)
	}
	call := fake.Calls[0]
	if call.Purpose != "fast" || !strings.Contains(call.Messages[0].Content, text) || !strings.Contains(call.Messages[0].Content, "SQLite 笔记") || strings.Contains(call.System, "填空") {
		t.Fatalf("question-answer call: %+v", call)
	}

	fake.Queue(llm.Result{Text: `{"question":"WAL 模式把写入先追加到 ____。","answer":"日志文件"}`}, nil)
	r.env.MustDo(http.MethodPost, "/cards/generate", map[string]any{"text": text, "style": "cloze"}, &draft)
	if !strings.Contains(draft.Question, "____") || !strings.Contains(fake.Calls[1].System, "填空") {
		t.Fatalf("cloze: %+v / %s", draft, fake.Calls[1].System)
	}

	// An empty draft is an error, not a card.
	fake.Queue(llm.Result{Text: `{"question":"","answer":""}`}, nil)
	if code, c := r.status(http.MethodPost, "/cards/generate", map[string]any{"text": text}); code != http.StatusBadGateway || c != "ai_failed" {
		t.Fatalf("empty draft: %d %s", code, c)
	}
	if code, _ := r.status(http.MethodPost, "/cards/generate", map[string]any{"text": "  "}); code != http.StatusBadRequest {
		t.Fatalf("empty text: %d", code)
	}
	if code, _ := r.status(http.MethodPost, "/cards/generate", map[string]any{"text": strings.Repeat("字", 4001)}); code != http.StatusBadRequest {
		t.Fatalf("long text: %d", code)
	}

	// Switched off in the settings: nothing is sent to the provider.
	r.settings(t, map[string]any{"aiEnabled": false})
	before := len(fake.Calls)
	if code, c := r.status(http.MethodPost, "/cards/generate", map[string]any{"text": text}); code != http.StatusConflict || c != "ai_disabled" {
		t.Fatalf("switched off: %d %s", code, c)
	}
	if len(fake.Calls) != before {
		t.Fatal("the text must not reach the provider when AI is off")
	}
	var s api.CardSettings
	r.env.MustDo(http.MethodGet, "/cards/settings", nil, &s)
	if !s.AiAvailable || s.AiEnabled {
		t.Fatalf("settings: %+v", s)
	}
}

func TestActions(t *testing.T) {
	r := setup(t)
	ctx := context.Background()
	out, err := r.env.App.Deps.Actions.Run(ctx, "cards.create", json.RawMessage(`{"question":"HTTP 204 是什么意思？","answer":"成功，没有内容。","deck":"网络","sourceLink":"/notes/9"}`))
	if err != nil {
		t.Fatal(err)
	}
	card, ok := out.(api.Card)
	if !ok || card.Id == 0 || card.Deck != "网络" {
		t.Fatalf("cards.create: %#v", out)
	}
	if _, err := r.env.App.Deps.Actions.Run(ctx, "cards.create", json.RawMessage(`{"question":"","answer":"x"}`)); err == nil {
		t.Fatal("an empty question must be refused")
	}
	out, err = r.env.App.Deps.Actions.Run(ctx, "cards.due_count", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := out.(map[string]int)
	if !ok || got["due"] != 1 || got["dueNew"] != 1 || got["dueReview"] != 0 {
		t.Fatalf("cards.due_count: %#v", out)
	}
	n, err := r.m.DueCount(ctx)
	if err != nil || n != 1 {
		t.Fatalf("DueCount: %d %v", n, err)
	}
	if _, ok := module.Lookup[contracts.CardsDue](r.env.App.Deps.Registry, contracts.CardsDueKey); !ok {
		t.Fatal("contract not provided")
	}
}

// ---- push and Telegram ----

type capture struct {
	mu   sync.Mutex
	sent []notify.Stored
}

func (c *capture) Name() string { return "telegram" }
func (c *capture) Send(_ context.Context, n notify.Stored) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, n)
	return nil
}
func (c *capture) last(t *testing.T) notify.Stored {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sent) == 0 {
		t.Fatal("nothing was sent to Telegram")
	}
	return c.sent[len(c.sent)-1]
}
func (c *capture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sent)
}

type notice struct{ Kind, Title, Link string }

func (r *rig) notices(t *testing.T) []notice {
	t.Helper()
	var out struct{ Items []notice }
	r.env.MustDo(http.MethodGet, "/notifications", nil, &out)
	var got []notice
	for _, n := range out.Items {
		if n.Kind == "card.due" {
			got = append(got, n)
		}
	}
	return got
}

func (r *rig) at(h, m int) {
	r.now = time.Date(r.now.Year(), r.now.Month(), r.now.Day(), h, m, 0, 0, clock.Location())
}

func TestDailyPush(t *testing.T) {
	r := setup(t)
	ctx := context.Background()
	push := func() {
		t.Helper()
		if err := cards.PushJob(r.m, ctx); err != nil {
			t.Fatal(err)
		}
	}
	r.create(t, "q1", "a")
	r.create(t, "q2", "a")

	r.at(19, 59)
	push()
	if len(r.notices(t)) != 0 {
		t.Fatal("pushed before the chosen time")
	}
	r.at(20, 0)
	push()
	got := r.notices(t)
	if len(got) != 1 || got[0].Title != "今天有 2 张卡待复习" || got[0].Link != "/cards" {
		t.Fatalf("push: %+v", got)
	}
	r.at(20, 1)
	push()
	if len(r.notices(t)) != 1 {
		t.Fatal("pushed twice in one day")
	}

	// Next day: nothing is due, so nothing is sent.
	r.days(1)
	for _, c := range r.queue(t).Items {
		r.review(t, c.Card.Id, 4)
	}
	r.at(20, 5)
	push()
	if len(r.notices(t)) != 1 {
		t.Fatal("pushed with nothing to review")
	}

	// A day with cards, but the server came back more than 3 hours late.
	r.days(30)
	r.at(23, 30)
	push()
	if len(r.notices(t)) != 1 {
		t.Fatal("a late push must be skipped")
	}

	// Within 3 hours it is still sent. Switched off, it is not.
	r.days(1)
	r.at(22, 30)
	r.settings(t, map[string]any{"pushEnabled": false})
	push()
	if len(r.notices(t)) != 1 {
		t.Fatal("pushed while switched off")
	}
	r.settings(t, map[string]any{"pushEnabled": true, "pushTime": "21:00"})
	push()
	if got := r.notices(t); len(got) != 2 {
		t.Fatalf("late but inside the window: %+v", got)
	}
}

func TestTelegramReview(t *testing.T) {
	r := setup(t)
	ctx := context.Background()
	tg := &capture{}
	r.env.App.Deps.Notify.RegisterChannel(tg)
	first := r.create(t, "第一题", "答案一")
	second := r.create(t, "第二题", "答案二")

	// "开始" sends the first question with a button for the answer.
	if err := r.env.App.Deps.Notify.HandleAction(ctx, "cards.start"); err != nil {
		t.Fatal(err)
	}
	q := tg.last(t)
	if !strings.Contains(q.Body, "第一题") || strings.Contains(q.Body, "答案一") || !strings.Contains(q.Title, "还剩 2 张") {
		t.Fatalf("question message: %+v", q.Notification)
	}
	if len(q.Actions) != 1 || q.Actions[0].ID != fmt.Sprintf("cards.show:%d", first.Id) || q.Actions[0].Label != "显示答案" {
		t.Fatalf("question buttons: %+v", q.Actions)
	}

	// The answer comes with the four ratings.
	if err := r.env.App.Deps.Notify.HandleAction(ctx, q.Actions[0].ID); err != nil {
		t.Fatal(err)
	}
	a := tg.last(t)
	if !strings.Contains(a.Body, "答案一") || len(a.Actions) != 4 {
		t.Fatalf("answer message: %+v", a)
	}
	labels := []string{}
	for _, act := range a.Actions {
		labels = append(labels, act.Label)
		if len(act.ID) > 64 {
			t.Fatalf("button id too long for Telegram: %s", act.ID)
		}
	}
	if strings.Join(labels, ",") != "重来,困难,良好,简单" {
		t.Fatalf("rating buttons: %v", labels)
	}

	// "良好" rates the card and sends the next question.
	good := a.Actions[2].ID
	if err := r.env.App.Deps.Notify.HandleAction(ctx, good); err != nil {
		t.Fatal(err)
	}
	var c api.Card
	r.env.MustDo(http.MethodGet, fmt.Sprintf("/cards/%d", first.Id), nil, &c)
	if c.Reps != 1 || c.State != api.Review {
		t.Fatalf("card after the rating: %+v", c)
	}
	n := tg.last(t)
	if !strings.Contains(n.Body, "第二题") || !strings.Contains(n.Title, "还剩 1 张") {
		t.Fatalf("next question: %+v", n.Notification)
	}

	// The same button a second time does not count again.
	count := tg.count()
	err := r.env.App.Deps.Notify.HandleAction(ctx, good)
	if err == nil || !strings.Contains(err.Error(), "card_stale") {
		t.Fatalf("pressing a rating twice: %v", err)
	}
	if tg.count() != count {
		t.Fatal("a stale button must not send a message")
	}
	r.env.MustDo(http.MethodGet, fmt.Sprintf("/cards/%d", first.Id), nil, &c)
	if c.Reps != 1 {
		t.Fatalf("reps after the second press: %d", c.Reps)
	}

	// The last card ends the session with a closing message.
	if err := r.env.App.Deps.Notify.HandleAction(ctx, n.Actions[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := r.env.App.Deps.Notify.HandleAction(ctx, tg.last(t).Actions[3].ID); err != nil {
		t.Fatal(err)
	}
	done := tg.last(t)
	if done.Title != "今天的卡片复习完了" || !strings.Contains(done.Body, "2 张") || len(done.Actions) != 0 {
		t.Fatalf("closing message: %+v", done.Notification)
	}
	var d api.Card
	r.env.MustDo(http.MethodGet, fmt.Sprintf("/cards/%d", second.Id), nil, &d)
	if d.Reps != 1 {
		t.Fatalf("second card: %+v", d)
	}

	// Nothing left: "开始" says so. Bad ids are refused.
	if err := r.env.App.Deps.Notify.HandleAction(ctx, "cards.start"); err != nil {
		t.Fatal(err)
	}
	if got := tg.last(t); !strings.Contains(got.Title, "复习完了") {
		t.Fatalf("start with nothing due: %+v", got.Notification)
	}
	for _, id := range []string{"cards.show:x", "cards.show:9999", "cards.rate:1:2", "cards.rate:a:b:c", "cards.other"} {
		if err := r.env.App.Deps.Notify.HandleAction(ctx, id); err == nil {
			t.Errorf("%q should be refused", id)
		}
	}
}
