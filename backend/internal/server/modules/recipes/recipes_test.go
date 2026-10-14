package recipes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// clock is the fixed time of the timer tests.
var clock = time.Date(2026, 10, 14, 18, 0, 0, 0, time.UTC)

type rig struct {
	env *testutil.Env
	m   *recipes.Module
	now time.Time
}

func setup(t *testing.T) *rig {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*recipes.Module](env.App.Deps.Registry, recipes.ServiceKey)
	if !ok {
		t.Fatal("recipes module not registered")
	}
	r := &rig{env: env, m: m, now: clock}
	recipes.SetNow(m, func() time.Time { return r.now })
	return r
}

func (r *rig) advance(t *testing.T, d time.Duration) {
	t.Helper()
	r.now = r.now.Add(d)
	if err := recipes.CheckTimers(r.m, context.Background(), r.now); err != nil {
		t.Fatal(err)
	}
}

func idStr(id int64) string { return strconv.FormatInt(id, 10) }

func (r *rig) create(t *testing.T, body map[string]any) api.Recipe {
	t.Helper()
	var v api.Recipe
	r.env.MustDo(http.MethodPost, "/recipes", body, &v)
	return v
}

// fail does a request that must be refused with the status, and the error text must contain want.
func (r *rig) fail(t *testing.T, method, path string, body any, status int, want string) {
	t.Helper()
	got, raw := r.env.Do(method, path, body, nil)
	if got != status || !strings.Contains(string(raw), want) {
		t.Fatalf("%s %s: want %d with %q, got %d %s", method, path, status, want, got, raw)
	}
}

type recipeList struct {
	Items []api.Recipe         `json:"items"`
	Tags  []api.RecipeTagCount `json:"tags"`
}

func (r *rig) list(t *testing.T, query string) recipeList {
	t.Helper()
	var out recipeList
	r.env.MustDo(http.MethodGet, "/recipes"+query, nil, &out)
	return out
}

func (r *rig) timers(t *testing.T) []api.CookTimer {
	t.Helper()
	var out struct{ Items []api.CookTimer }
	r.env.MustDo(http.MethodGet, "/cook-timers", nil, &out)
	return out.Items
}

func (r *rig) startTimer(t *testing.T, body map[string]any) api.CookTimer {
	t.Helper()
	var v api.CookTimer
	r.env.MustDo(http.MethodPost, "/cook-timers", body, &v)
	return v
}

func (r *rig) patchTimer(t *testing.T, id int64, body map[string]any) api.CookTimer {
	t.Helper()
	var v api.CookTimer
	r.env.MustDo(http.MethodPatch, "/cook-timers/"+idStr(id), body, &v)
	return v
}

type notice struct{ Kind, Title, Body, Link string }

func (r *rig) notices(t *testing.T, kind string) []notice {
	t.Helper()
	var out struct{ Items []notice }
	r.env.MustDo(http.MethodGet, "/notifications", nil, &out)
	var got []notice
	for _, n := range out.Items {
		if n.Kind == kind {
			got = append(got, n)
		}
	}
	return got
}

// chicken is a small recipe used by several tests: 3 ingredients, 2 steps.
func chicken() map[string]any {
	return map[string]any{
		"title":        " 临沂炒鸡 ",
		"summary":      "鲁菜",
		"servings":     "一只散养公鸡",
		"totalMinutes": 90,
		"ingredients": []map[string]any{
			{"name": "公鸡", "amount": "1500 克", "note": "切块"},
			{"name": "花椒", "amount": "1 汤匙"},
			{"name": "生姜", "note": "切片"},
		},
		"steps": []map[string]any{
			{"title": "泡血水", "text": "鸡块泡 30 分钟", "timers": []map[string]any{{"label": "泡水", "seconds": 1800}}, "ingredients": []int{0}},
			{"text": "加开水炖", "timers": []map[string]any{{"label": "炖煮", "seconds": 1200, "maxSeconds": 2400}}, "ingredients": []int{1, 2}},
		},
		"tips":      "**要点**：火大一点",
		"tags":      []string{"辣", "鲁菜", "鸡肉"},
		"sourceUrl": "https://example.com/linyi",
	}
}

func TestRecipeCRUD(t *testing.T) {
	r := setup(t)
	sub, cancel := r.env.App.Deps.Bus.Subscribe("recipe.", 16)
	defer cancel()
	next := func() events.Event {
		select {
		case ev := <-sub:
			return ev
		case <-time.After(time.Second):
			t.Fatal("no event")
			return events.Event{}
		}
	}

	v := r.create(t, chicken())
	if ev := next(); ev.Topic != "recipe.created" {
		t.Fatalf("event: %s", ev.Topic)
	}
	if v.Title != "临沂炒鸡" || v.TotalMinutes == nil || *v.TotalMinutes != 90 || len(v.Ingredients) != 3 || len(v.Steps) != 2 || v.SourceNoteId != nil {
		t.Fatalf("created: %+v", v)
	}
	// amount and note are always there, steps always carry their lists
	if v.Ingredients[2].Amount == nil || *v.Ingredients[2].Amount != "" || *v.Ingredients[1].Note != "" {
		t.Fatalf("ingredient defaults: %+v", v.Ingredients)
	}
	if v.Steps[1].Title == nil || *v.Steps[1].Title != "" || len(*v.Steps[1].Ingredients) != 2 || *(*v.Steps[1].Timers)[0].MaxSeconds != 2400 {
		t.Fatalf("steps: %+v", v.Steps)
	}

	var got api.Recipe
	r.env.MustDo(http.MethodGet, "/recipes/"+idStr(v.Id), nil, &got)
	if got.Title != v.Title || got.Tips != "**要点**：火大一点" || got.SourceUrl != "https://example.com/linyi" {
		t.Fatalf("get: %+v", got)
	}

	// only the given fields change
	var upd api.Recipe
	r.env.MustDo(http.MethodPatch, "/recipes/"+idStr(v.Id), map[string]any{"title": "临沂炒鸡（改）", "tags": []string{"鸡肉", "辣", " 辣 ", ""}}, &upd)
	if ev := next(); ev.Topic != "recipe.updated" {
		t.Fatalf("event: %s", ev.Topic)
	}
	if upd.Title != "临沂炒鸡（改）" || len(upd.Steps) != 2 || len(upd.Ingredients) != 3 || upd.Servings != "一只散养公鸡" || strings.Join(upd.Tags, ",") != "鸡肉,辣" {
		t.Fatalf("patch: %+v", upd)
	}
	// null clears the nullable fields, a missing key keeps them
	r.env.MustDo(http.MethodPatch, "/recipes/"+idStr(v.Id), map[string]any{"summary": "新的简介"}, &upd)
	if upd.TotalMinutes == nil || *upd.TotalMinutes != 90 {
		t.Fatalf("minutes kept: %+v", upd.TotalMinutes)
	}
	r.env.MustDo(http.MethodPatch, "/recipes/"+idStr(v.Id), map[string]any{"totalMinutes": nil, "sourceNoteId": 7}, &upd)
	if upd.TotalMinutes != nil || upd.SourceNoteId == nil || *upd.SourceNoteId != 7 {
		t.Fatalf("clear minutes: %+v", upd)
	}
	r.env.MustDo(http.MethodPatch, "/recipes/"+idStr(v.Id), map[string]any{"sourceNoteId": nil}, &upd)
	if upd.SourceNoteId != nil {
		t.Fatalf("clear note: %+v", upd.SourceNoteId)
	}
	next()
	next()
	next()

	if status, _ := r.env.Do(http.MethodDelete, "/recipes/"+idStr(v.Id), nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if ev := next(); ev.Topic != "recipe.deleted" {
		t.Fatalf("event: %s", ev.Topic)
	}
	r.fail(t, http.MethodGet, "/recipes/"+idStr(v.Id), nil, 404, "")
	r.fail(t, http.MethodPatch, "/recipes/"+idStr(v.Id), map[string]any{"title": "x"}, 404, "")
	r.fail(t, http.MethodDelete, "/recipes/"+idStr(v.Id), nil, 404, "")
}

func TestRecipeListFilterAndTags(t *testing.T) {
	r := setup(t)
	a := r.create(t, map[string]any{"title": "番茄炒蛋", "summary": "家常", "tags": []string{"家常", "蛋"}, "ingredients": []map[string]any{{"name": "番茄"}, {"name": "鸡蛋"}}})
	r.create(t, chicken())
	c := r.create(t, map[string]any{"title": "红烧肉", "tags": []string{"家常", "肉"}})

	out := r.list(t, "")
	if len(out.Items) != 3 || out.Items[0].Id != c.Id || out.Items[2].Id != a.Id {
		t.Fatalf("order, newest first: %+v", out.Items)
	}
	if len(out.Tags) != 6 || out.Tags[0].Tag != "家常" || out.Tags[0].Count != 2 {
		t.Fatalf("tags: %+v", out.Tags)
	}
	ids := func(l recipeList) string {
		var s []string
		for _, v := range l.Items {
			s = append(s, v.Title)
		}
		return strings.Join(s, ",")
	}
	if got := ids(r.list(t, "?tag=家常")); got != "红烧肉,番茄炒蛋" {
		t.Fatalf("tag: %s", got)
	}
	for q, want := range map[string]string{"鸡蛋": "番茄炒蛋", "鲁菜": "临沂炒鸡", "花椒": "临沂炒鸡", "红烧": "红烧肉", "家常": "红烧肉,番茄炒蛋", "没有": ""} {
		if got := ids(r.list(t, "?q="+q)); got != want {
			t.Fatalf("q=%s: %q want %q", q, got, want)
		}
	}
	// tags count all recipes even when the list is filtered
	if f := r.list(t, "?q=红烧"); len(f.Items) != 1 || len(f.Tags) != 6 {
		t.Fatalf("filtered tags: %+v", f)
	}
}

func TestRecipeValidation(t *testing.T) {
	r := setup(t)
	with := func(k string, v any) map[string]any {
		body := map[string]any{"title": "菜"}
		body[k] = v
		return body
	}
	long := func(n int) string { return strings.Repeat("字", n) }
	step := func(s map[string]any) map[string]any { return with("steps", []map[string]any{s}) }
	cases := []struct {
		name string
		body map[string]any
		want string
	}{
		{"no title", map[string]any{"title": "  "}, "菜名不能为空"},
		{"long title", map[string]any{"title": long(101)}, "菜名太长"},
		{"long summary", with("summary", long(501)), "简介太长"},
		{"long servings", with("servings", long(51)), "份量太长"},
		{"minutes zero", with("totalMinutes", 0), "大概用时"},
		{"minutes high", with("totalMinutes", 10081), "大概用时"},
		{"long tips", with("tips", long(20001)), "备注太长"},
		{"too many tags", with("tags", func() []string {
			var s []string
			for i := 0; i < 21; i++ {
				s = append(s, "t"+strconv.Itoa(i))
			}
			return s
		}()), "标签最多 20 个"},
		{"long tag", with("tags", []string{long(21)}), "太长"},
		{"bad url", with("sourceUrl", "ftp://example.com/a"), "来源地址"},
		{"url without scheme", with("sourceUrl", "example.com"), "来源地址"},
		{"js url", with("sourceUrl", "javascript:alert(1)"), "来源地址"},
		{"ingredient no name", with("ingredients", []map[string]any{{"name": "盐"}, {"name": " "}}), "第 2 个食材的名称不能为空"},
		{"ingredient long amount", with("ingredients", []map[string]any{{"name": "盐", "amount": long(31)}}), "第 1 个食材的用量太长"},
		{"too many ingredients", with("ingredients", func() []map[string]any {
			var s []map[string]any
			for i := 0; i < 101; i++ {
				s = append(s, map[string]any{"name": "a"})
			}
			return s
		}()), "食材最多 100 个"},
		{"step no text", step(map[string]any{"text": " "}), "第 1 步的正文不能为空"},
		{"step long text", step(map[string]any{"text": long(2001)}), "第 1 步的正文太长"},
		{"too many steps", with("steps", func() []map[string]any {
			var s []map[string]any
			for i := 0; i < 101; i++ {
				s = append(s, map[string]any{"text": "a"})
			}
			return s
		}()), "步骤最多 100 步"},
		{"timer no label", step(map[string]any{"text": "a", "timers": []map[string]any{{"label": "", "seconds": 5}}}), "第 1 步第 1 个计时的名字不能为空"},
		{"timer zero", step(map[string]any{"text": "a", "timers": []map[string]any{{"label": "a", "seconds": 0}}}), "时长要在 1 到 86400"},
		{"timer high", step(map[string]any{"text": "a", "timers": []map[string]any{{"label": "a", "seconds": 86401}}}), "时长要在 1 到 86400"},
		{"timer max not above", step(map[string]any{"text": "a", "timers": []map[string]any{{"label": "a", "seconds": 60, "maxSeconds": 60}}}), "上限"},
		{"timer max high", step(map[string]any{"text": "a", "timers": []map[string]any{{"label": "a", "seconds": 60, "maxSeconds": 86401}}}), "上限"},
		{"too many timers", step(map[string]any{"text": "a", "timers": []map[string]any{
			{"label": "a", "seconds": 1}, {"label": "a", "seconds": 1}, {"label": "a", "seconds": 1},
			{"label": "a", "seconds": 1}, {"label": "a", "seconds": 1}, {"label": "a", "seconds": 1}}}), "计时最多 5 个"},
		{"index out of range", step(map[string]any{"text": "a", "ingredients": []int{0}}), "超出了食材清单"},
		{"index negative", step(map[string]any{"text": "a", "ingredients": []int{-1}}), "超出了食材清单"},
		{"unknown field", with("nope", 1), "请求体格式不正确"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { r.fail(t, http.MethodPost, "/recipes", c.body, 400, c.want) })
	}
	dup := map[string]any{"title": "菜", "ingredients": []map[string]any{{"name": "盐"}}, "steps": []map[string]any{{"text": "a", "ingredients": []int{0, 0}}}}
	r.fail(t, http.MethodPost, "/recipes", dup, 400, "重复")
	if n := len(r.list(t, "").Items); n != 0 {
		t.Fatalf("a refused recipe was saved: %d", n)
	}
	// the exact limits are fine
	ok := map[string]any{"title": long(100), "summary": long(500), "totalMinutes": 10080, "tags": []string{long(20)}, "sourceUrl": "http://a.example/x"}
	r.create(t, ok)
}

func TestPatchChecksStepIndexes(t *testing.T) {
	r := setup(t)
	v := r.create(t, chicken())
	path := "/recipes/" + idStr(v.Id)
	// fewer ingredients than the steps use
	r.fail(t, http.MethodPatch, path, map[string]any{"ingredients": []map[string]any{{"name": "公鸡"}}}, 400, "第 2 步用到的食材超出了食材清单")
	// steps that point past the current ingredients
	r.fail(t, http.MethodPatch, path, map[string]any{"steps": []map[string]any{{"text": "a", "ingredients": []int{3}}}}, 400, "超出了食材清单")
	// both at once are checked against each other
	var upd api.Recipe
	r.env.MustDo(http.MethodPatch, path, map[string]any{
		"ingredients": []map[string]any{{"name": "公鸡"}},
		"steps":       []map[string]any{{"text": "炖", "ingredients": []int{0}}},
	}, &upd)
	if len(upd.Ingredients) != 1 || len(upd.Steps) != 1 {
		t.Fatalf("patch both: %+v", upd)
	}
	// more ingredients are fine for the old steps
	r.env.MustDo(http.MethodPatch, path, map[string]any{"ingredients": []map[string]any{{"name": "公鸡"}, {"name": "盐"}}}, &upd)
	if len(upd.Ingredients) != 2 || len(*upd.Steps[0].Ingredients) != 1 {
		t.Fatalf("grow: %+v", upd)
	}
	// a refused patch changes nothing
	var again api.Recipe
	r.env.MustDo(http.MethodGet, path, nil, &again)
	if len(again.Ingredients) != 2 {
		t.Fatalf("after refused patch: %+v", again)
	}
}

// ---------- AI parse ----------

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

func withLLM(r *rig) *fakeLLM {
	fake := &fakeLLM{Fake: llm.NewFake()}
	module.Provide[contracts.LLM](r.env.App.Deps.Registry, contracts.LLMKey, fake)
	return fake
}

func answer(t *testing.T, v any) llm.Result {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return llm.Result{Text: string(raw)}
}

func TestParseWithoutAI(t *testing.T) {
	r := setup(t)
	r.fail(t, http.MethodPost, "/recipes/parse", map[string]any{"text": "做法"}, 501, "feature_unavailable")
	r.fail(t, http.MethodPost, "/recipes/parse", map[string]any{"text": "   "}, 400, "原文是空的")
	r.fail(t, http.MethodPost, "/recipes/parse", map[string]any{"text": strings.Repeat("字", 20001)}, 400, "太长")
}

func TestParseRepairsTheAnswer(t *testing.T) {
	r := setup(t)
	fake := withLLM(r)
	names := []string{"公鸡", "花椒", "八角", "干辣椒", "生姜", "大葱", "大蒜", "冰糖", "料酒", "生抽", "老抽", "盐", "食用油", "香菜", "白糖", "豆瓣酱"}
	ings := make([]map[string]any, 0, len(names)+1)
	for i, n := range names {
		ings = append(ings, map[string]any{"name": n, "amount": strconv.Itoa(i+1) + " 克", "note": ""})
	}
	// an ingredient without a name is dropped; the later indexes move up
	ings = append(ings[:3], append([]map[string]any{{"name": " ", "amount": "", "note": ""}}, ings[3:]...)...)
	tags := []string{"辣", " 辣 ", "鲁菜", "鸡肉", "", strings.Repeat("长", 30)}
	steps := []map[string]any{
		{"title": "泡血水", "text": "鸡块泡水", "timers": []map[string]any{{"label": "泡水", "seconds": 1800, "maxSeconds": nil}}, "ingredients": []int{0}},
		{"title": "", "text": "焯水", "timers": []map[string]any{}, "ingredients": []int{}},
		// ingredient 1,2 stay, the empty one (3) goes, 4 and 5 move to 3 and 4, 99 and the repeat go
		{"title": "爆香", "text": "放入花椒八角", "timers": []map[string]any{}, "ingredients": []int{1, 2, 3, 4, 5, 5, 99, -1}},
		{"title": "炒", "text": "翻炒", "timers": []map[string]any{}, "ingredients": []int{}},
		{"title": "加开水炖", "text": "炖", "timers": []map[string]any{
			{"label": "炖煮", "seconds": 1200, "maxSeconds": 2400},
			{"label": "坏的", "seconds": 0, "maxSeconds": nil},
			{"label": "太长", "seconds": 90000, "maxSeconds": nil},
			{"label": "", "seconds": 60, "maxSeconds": 30},
		}, "ingredients": []int{}},
		{"title": "收汁", "text": "  ", "timers": []map[string]any{}, "ingredients": []int{}},
		{"title": "出锅", "text": "撒香菜", "timers": []map[string]any{}, "ingredients": []int{14}},
	}
	fake.Queue(answer(t, map[string]any{
		"title": "  临沂炒鸡 ", "summary": "鲁菜", "servings": "一只散养公鸡", "totalMinutes": 20000,
		"ingredients": ings, "steps": steps, "tips": "## 关键要点\n火要大\n\n来源：https://example.com/x", "tags": tags,
	}), nil)

	var out api.RecipeInput
	r.env.MustDo(http.MethodPost, "/recipes/parse", map[string]any{"text": "临沂炒鸡的做法……", "sourceNoteId": 12}, &out)
	if len(fake.Calls) != 1 || fake.Calls[0].Messages[0].Content != "临沂炒鸡的做法……" || !strings.Contains(fake.Calls[0].System, "备注") || len(fake.Calls[0].JSONSchema) == 0 {
		t.Fatalf("request: %+v", fake.Calls)
	}
	if out.Title != "临沂炒鸡" || out.TotalMinutes != nil || out.SourceNoteId == nil || *out.SourceNoteId != 12 {
		t.Fatalf("head: %+v", out)
	}
	if len(*out.Ingredients) != 16 || (*out.Ingredients)[3].Name != "干辣椒" || *(*out.Ingredients)[0].Amount != "1 克" {
		t.Fatalf("ingredients: %+v", *out.Ingredients)
	}
	if strings.Join(*out.Tags, ",") != "辣,鲁菜,鸡肉,"+strings.Repeat("长", 20) {
		t.Fatalf("tags: %v", *out.Tags)
	}
	st := *out.Steps
	if len(st) != 6 {
		t.Fatalf("a step without text must go: %d", len(st))
	}
	if t0 := *st[0].Timers; len(t0) != 1 || t0[0].Seconds != 1800 || t0[0].MaxSeconds != nil {
		t.Fatalf("step 1 timers: %+v", t0)
	}
	if got := *st[2].Ingredients; len(got) != 4 || got[0] != 1 || got[1] != 2 || got[2] != 3 || got[3] != 4 {
		t.Fatalf("step 3 ingredients: %v", got)
	}
	t4 := *st[4].Timers
	if len(t4) != 2 || t4[0].Seconds != 1200 || *t4[0].MaxSeconds != 2400 || t4[1].Label != "计时" || t4[1].Seconds != 60 || t4[1].MaxSeconds != nil {
		t.Fatalf("step 5 timers: %+v", t4)
	}
	if got := *st[5].Ingredients; len(got) != 1 || got[0] != 13 {
		t.Fatalf("step 7 ingredients: %v", got)
	}
	if !strings.Contains(*out.Tips, "关键要点") || !strings.Contains(*out.Tips, "https://example.com/x") {
		t.Fatalf("tips: %q", *out.Tips)
	}
	// the draft is not saved, and it can be saved as it is
	if n := len(r.list(t, "").Items); n != 0 {
		t.Fatalf("parse saved a recipe: %d", n)
	}
	saved := r.create(t, map[string]any{"title": out.Title, "ingredients": out.Ingredients, "steps": out.Steps, "tags": out.Tags, "tips": out.Tips, "sourceNoteId": out.SourceNoteId})
	if saved.SourceNoteId == nil || *saved.SourceNoteId != 12 || len(saved.Steps) != 6 {
		t.Fatalf("saved draft: %+v", saved)
	}
}

func TestParseFailures(t *testing.T) {
	r := setup(t)
	fake := withLLM(r)
	// no title
	fake.Queue(answer(t, map[string]any{"title": " ", "summary": "", "servings": "", "totalMinutes": nil, "ingredients": []any{}, "steps": []map[string]any{{"title": "", "text": "a", "timers": []any{}, "ingredients": []any{}}}, "tips": "", "tags": []string{}}), nil)
	r.fail(t, http.MethodPost, "/recipes/parse", map[string]any{"text": "x"}, 502, "AI 返回的菜谱不完整，再试一次")
	// nothing in it
	fake.Queue(answer(t, map[string]any{"title": "不是菜谱", "summary": "", "servings": "", "totalMinutes": nil, "ingredients": []any{}, "steps": []any{}, "tips": "", "tags": []string{}}), nil)
	r.fail(t, http.MethodPost, "/recipes/parse", map[string]any{"text": "x"}, 502, "不完整")
	// the model failed
	fake.Queue(llm.Result{}, context.DeadlineExceeded)
	r.fail(t, http.MethodPost, "/recipes/parse", map[string]any{"text": "x"}, 502, "AI 整理失败")
	// not JSON
	fake.Queue(llm.Result{Text: "对不起"}, nil)
	r.fail(t, http.MethodPost, "/recipes/parse", map[string]any{"text": "x"}, 502, "AI 整理失败")
}

// ---------- timers ----------

func TestTimerLifecycle(t *testing.T) {
	r := setup(t)
	sub, cancel := r.env.App.Deps.Bus.Subscribe("cook_timer.", 32)
	defer cancel()
	topics := func() []string {
		var got []string
		for {
			select {
			case ev := <-sub:
				got = append(got, ev.Topic)
			default:
				return got
			}
		}
	}
	rec := r.create(t, chicken())
	v := r.startTimer(t, map[string]any{"label": "炖煮", "seconds": 90, "recipeId": rec.Id, "stepIndex": 1})
	if v.State != api.Running || v.RemainingSeconds != 90 || v.RecipeTitle != "临沂炒鸡" || v.StepIndex == nil || *v.StepIndex != 1 || v.EndsAt == nil || !v.EndsAt.Equal(clock.Add(90*time.Second)) {
		t.Fatalf("started: %+v", v)
	}
	if got := strings.Join(topics(), ","); got != "cook_timer.created" {
		t.Fatalf("events: %s", got)
	}

	// time passes: the remaining seconds are worked out when it is returned
	r.now = r.now.Add(30 * time.Second)
	if l := r.timers(t); len(l) != 1 || l[0].RemainingSeconds != 60 {
		t.Fatalf("remaining: %+v", l)
	}

	// pause keeps the remaining seconds and never runs out
	p := r.patchTimer(t, v.Id, map[string]any{"action": "pause"})
	if p.State != api.Paused || p.RemainingSeconds != 60 || p.EndsAt != nil {
		t.Fatalf("paused: %+v", p)
	}
	r.advance(t, time.Hour)
	if l := r.timers(t); l[0].State != api.Paused || l[0].RemainingSeconds != 60 {
		t.Fatalf("paused timer moved: %+v", l[0])
	}
	if n := len(r.notices(t, "cook_timer.done")); n != 0 {
		t.Fatalf("paused timer notified: %d", n)
	}
	r.fail(t, http.MethodPatch, "/cook-timers/"+idStr(v.Id), map[string]any{"action": "pause"}, 409, "")
	// adding to a paused timer changes what is left
	if p = r.patchTimer(t, v.Id, map[string]any{"addSeconds": 30}); p.RemainingSeconds != 90 || p.State != api.Paused {
		t.Fatalf("add while paused: %+v", p)
	}
	// resume counts from now
	rs := r.patchTimer(t, v.Id, map[string]any{"action": "resume"})
	if rs.State != api.Running || rs.RemainingSeconds != 90 || !rs.EndsAt.Equal(r.now.Add(90*time.Second)) {
		t.Fatalf("resumed: %+v", rs)
	}
	r.fail(t, http.MethodPatch, "/cook-timers/"+idStr(v.Id), map[string]any{"action": "resume"}, 409, "")

	// adding while running moves the end; taking away leaves at least one second
	if a := r.patchTimer(t, v.Id, map[string]any{"addSeconds": 60}); a.RemainingSeconds != 150 {
		t.Fatalf("add: %+v", a)
	}
	if a := r.patchTimer(t, v.Id, map[string]any{"addSeconds": -3600}); a.RemainingSeconds != 1 {
		t.Fatalf("subtract: %+v", a)
	}
	if a := r.patchTimer(t, v.Id, map[string]any{"addSeconds": 59}); a.RemainingSeconds != 60 {
		t.Fatalf("add again: %+v", a)
	}
	topics()

	// not yet
	r.advance(t, 59*time.Second)
	if l := r.timers(t); l[0].State != api.Running {
		t.Fatalf("early: %+v", l[0])
	}
	// the time comes: done, notified once, event once
	r.advance(t, 2*time.Second)
	l := r.timers(t)
	if l[0].State != api.Done || l[0].RemainingSeconds != 0 || l[0].EndsAt == nil {
		t.Fatalf("done: %+v", l[0])
	}
	r.advance(t, 5*time.Second)
	notes := r.notices(t, "cook_timer.done")
	if len(notes) != 1 || notes[0].Title != "时间到：炖煮" || notes[0].Body != "临沂炒鸡 · 第 2 步" || notes[0].Link != "/recipes/"+idStr(rec.Id)+"/cook" {
		t.Fatalf("notification: %+v", notes)
	}
	if got := strings.Join(topics(), ","); got != "cook_timer.done" {
		t.Fatalf("events: %s", got)
	}

	// adding to a finished timer runs it again from now
	again := r.patchTimer(t, v.Id, map[string]any{"addSeconds": 120})
	if again.State != api.Running || again.RemainingSeconds != 120 || !again.EndsAt.Equal(r.now.Add(120*time.Second)) {
		t.Fatalf("restart: %+v", again)
	}
	r.advance(t, 121*time.Second)
	if n := len(r.notices(t, "cook_timer.done")); n != 2 {
		t.Fatalf("second round notified %d times", n)
	}

	// the recipe goes, the timer stays and keeps the name
	if status, _ := r.env.Do(http.MethodDelete, "/recipes/"+idStr(rec.Id), nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete recipe: %d", status)
	}
	l = r.timers(t)
	if len(l) != 1 || l[0].RecipeId != nil || l[0].RecipeTitle != "临沂炒鸡" {
		t.Fatalf("after recipe delete: %+v", l)
	}

	// closing removes it
	topics()
	if status, _ := r.env.Do(http.MethodDelete, "/cook-timers/"+idStr(v.Id), nil, nil); status != http.StatusNoContent {
		t.Fatalf("close: %d", status)
	}
	if got := strings.Join(topics(), ","); got != "cook_timer.deleted" {
		t.Fatalf("events: %s", got)
	}
	r.fail(t, http.MethodDelete, "/cook-timers/"+idStr(v.Id), nil, 404, "")
	r.fail(t, http.MethodPatch, "/cook-timers/"+idStr(v.Id), map[string]any{"addSeconds": 5}, 404, "")
}

func TestTimerValidation(t *testing.T) {
	r := setup(t)
	rec := r.create(t, chicken())
	cases := []struct {
		name string
		body map[string]any
		want string
	}{
		{"no label", map[string]any{"label": " ", "seconds": 5}, "名字不能为空"},
		{"long label", map[string]any{"label": strings.Repeat("字", 31), "seconds": 5}, "名字太长"},
		{"zero", map[string]any{"label": "a", "seconds": 0}, "时长"},
		{"too long", map[string]any{"label": "a", "seconds": 86401}, "时长"},
		{"unknown recipe", map[string]any{"label": "a", "seconds": 5, "recipeId": 9999}, "菜谱不存在"},
		{"step without recipe", map[string]any{"label": "a", "seconds": 5, "stepIndex": 0}, "同时给菜谱"},
		{"step past the end", map[string]any{"label": "a", "seconds": 5, "recipeId": rec.Id, "stepIndex": 2}, "没有第 3 步"},
		{"negative step", map[string]any{"label": "a", "seconds": 5, "recipeId": rec.Id, "stepIndex": -1}, "步骤不正确"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { r.fail(t, http.MethodPost, "/cook-timers", c.body, 400, c.want) })
	}
	v := r.startTimer(t, map[string]any{"label": "a", "seconds": 60})
	if v.RecipeId != nil || v.RecipeTitle != "" || v.StepIndex != nil {
		t.Fatalf("plain timer: %+v", v)
	}
	path := "/cook-timers/" + idStr(v.Id)
	r.fail(t, http.MethodPatch, path, map[string]any{}, 400, "一次只能做一件事")
	r.fail(t, http.MethodPatch, path, map[string]any{"action": "pause", "addSeconds": 5}, 400, "一次只能做一件事")
	r.fail(t, http.MethodPatch, path, map[string]any{"action": "stop"}, 400, "")
	r.fail(t, http.MethodPatch, path, map[string]any{"addSeconds": 0}, 400, "不能是 0")
	r.fail(t, http.MethodPatch, path, map[string]any{"addSeconds": 86400}, 400, "24 小时")
}

func TestTimerLimit(t *testing.T) {
	r := setup(t)
	var first api.CookTimer
	for i := 0; i < 20; i++ {
		v := r.startTimer(t, map[string]any{"label": "t" + strconv.Itoa(i), "seconds": 600 + i})
		if i == 0 {
			first = v
		}
	}
	r.fail(t, http.MethodPost, "/cook-timers", map[string]any{"label": "多了", "seconds": 5}, 400, "最多 20 个")
	// closing one makes room; finished timers still count
	r.env.MustDo(http.MethodDelete, "/cook-timers/"+idStr(first.Id), nil, nil)
	r.startTimer(t, map[string]any{"label": "刚好", "seconds": 5})
	r.advance(t, time.Hour)
	r.fail(t, http.MethodPost, "/cook-timers", map[string]any{"label": "多了", "seconds": 5}, 400, "最多 20 个")
	l := r.timers(t)
	if len(l) != 20 || l[0].Label != "刚好" {
		t.Fatalf("list: %d first %+v", len(l), l[0])
	}
}

func TestTimerOrderAndCleanup(t *testing.T) {
	r := setup(t)
	slow := r.startTimer(t, map[string]any{"label": "慢", "seconds": 600})
	fast := r.startTimer(t, map[string]any{"label": "快", "seconds": 60})
	paused := r.startTimer(t, map[string]any{"label": "停", "seconds": 300})
	r.patchTimer(t, paused.Id, map[string]any{"action": "pause"})
	var labels []string
	for _, v := range r.timers(t) {
		labels = append(labels, v.Label)
	}
	if strings.Join(labels, ",") != "快,停,慢" {
		t.Fatalf("order: %v", labels)
	}
	// finished timers are removed after 6 hours, the others stay
	r.advance(t, 2*time.Minute)
	if l := r.timers(t); l[0].Id != fast.Id || l[0].State != api.Done {
		t.Fatalf("fast: %+v", l[0])
	}
	r.advance(t, 5*time.Hour+30*time.Minute)
	if len(r.timers(t)) != 3 {
		t.Fatalf("early cleanup: %+v", r.timers(t))
	}
	sub, cancel := r.env.App.Deps.Bus.Subscribe("cook_timer.deleted", 8)
	defer cancel()
	r.advance(t, time.Hour)
	ids := func() map[int64]bool {
		got := map[int64]bool{}
		for _, v := range r.timers(t) {
			got[v.Id] = true
		}
		return got
	}
	// fast was done for more than 6 hours; slow only for one hour
	if got := ids(); len(got) != 2 || !got[slow.Id] || !got[paused.Id] || len(sub) != 1 {
		t.Fatalf("after first cleanup: %v events %d (fast %d)", got, len(sub), fast.Id)
	}
	r.advance(t, 6*time.Hour)
	if got := ids(); len(got) != 1 || !got[paused.Id] || len(sub) != 2 {
		t.Fatalf("after second cleanup: %v events %d", got, len(sub))
	}
}

func TestDismissFromNotification(t *testing.T) {
	r := setup(t)
	v := r.startTimer(t, map[string]any{"label": "煮蛋", "seconds": 5})
	r.advance(t, 10*time.Second)
	notes := r.notices(t, "cook_timer.done")
	if len(notes) != 1 || notes[0].Body != "" || notes[0].Link != "/recipes" {
		t.Fatalf("notification without recipe: %+v", notes)
	}
	// the buttons are kept in the stored data
	var data string
	if err := r.env.App.Deps.DB.QueryRow(`SELECT data FROM notifications WHERE kind = 'cook_timer.done'`).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var stored struct{ Actions []struct{ ID, Label string } }
	if err := json.Unmarshal([]byte(data), &stored); err != nil || len(stored.Actions) != 1 || stored.Actions[0].Label != "关掉" {
		t.Fatalf("buttons: %s %v", data, err)
	}
	actionID := stored.Actions[0].ID
	if actionID != "cook_timer.dismiss:"+idStr(v.Id) {
		t.Fatalf("action id %q", actionID)
	}
	r.env.MustDo(http.MethodPost, "/notify/actions", map[string]string{"actionId": actionID}, nil)
	if len(r.timers(t)) != 0 {
		t.Fatal("timer still there")
	}
	// pressing it again is fine
	r.env.MustDo(http.MethodPost, "/notify/actions", map[string]string{"actionId": actionID}, nil)
	if status, _ := r.env.Do(http.MethodPost, "/notify/actions", map[string]string{"actionId": "cook_timer.dismiss:abc"}, nil); status != http.StatusNotFound {
		t.Fatalf("a bad id: %d", status)
	}
}

// ---------- AI actions ----------

func TestAIActions(t *testing.T) {
	r := setup(t)
	run := func(name, args string) (string, error) {
		out, err := r.env.App.Deps.Actions.Run(context.Background(), name, json.RawMessage(args))
		raw, _ := json.Marshal(out)
		return string(raw), err
	}
	out, err := run("recipes.create", `{"title":"番茄炒蛋","ingredients":[{"name":"番茄","amount":"2 个"},{"name":"鸡蛋","amount":"3 个"}],`+
		`"steps":[{"text":"炒蛋","timers":[{"label":"炒","seconds":60}],"ingredients":[1]},{"text":"加番茄","ingredients":[0]}],"tags":["家常","蛋"]}`)
	if err != nil || !strings.Contains(out, `"title":"番茄炒蛋"`) {
		t.Fatalf("create: %s %v", out, err)
	}
	if _, err = run("recipes.create", `{"title":" "}`); err == nil {
		t.Fatal("an invalid recipe must be refused")
	}
	if _, err = run("recipes.create", `{"title":"a","steps":[{"text":"b","ingredients":[3]}]}`); err == nil {
		t.Fatal("a bad index must be refused")
	}
	list := r.list(t, "")
	if len(list.Items) != 1 {
		t.Fatalf("saved: %d", len(list.Items))
	}
	rid := list.Items[0].Id

	out, err = run("recipes.search", `{"q":"鸡蛋"}`)
	if err != nil || !strings.Contains(out, `"title":"番茄炒蛋"`) || !strings.Contains(out, `"ingredients":["番茄","鸡蛋"]`) || strings.Contains(out, `"steps"`) {
		t.Fatalf("search q: %s %v", out, err)
	}
	if out, err = run("recipes.search", `{"tag":"家常"}`); err != nil || !strings.Contains(out, "番茄炒蛋") {
		t.Fatalf("search tag: %s %v", out, err)
	}
	if out, err = run("recipes.search", `{"q":"牛肉"}`); err != nil || strings.Contains(out, "番茄炒蛋") {
		t.Fatalf("search none: %s %v", out, err)
	}
	if out, err = run("recipes.search", ``); err != nil || !strings.Contains(out, "番茄炒蛋") {
		t.Fatalf("search all: %s %v", out, err)
	}
	out, err = run("recipes.get", `{"id":`+idStr(rid)+`}`)
	if err != nil || !strings.Contains(out, `"text":"加番茄"`) {
		t.Fatalf("get: %s %v", out, err)
	}
	if _, err = run("recipes.get", `{"id":999}`); err == nil {
		t.Fatal("an unknown id must fail")
	}

	out, err = run("cook_timers.start", `{"label":"炖煮","minutes":1.5,"recipeId":`+idStr(rid)+`,"stepIndex":1}`)
	if err != nil || !strings.Contains(out, `"seconds":90`) || !strings.Contains(out, `"recipeTitle":"番茄炒蛋"`) {
		t.Fatalf("start: %s %v", out, err)
	}
	if l := r.timers(t); len(l) != 1 || l[0].Seconds != 90 {
		t.Fatalf("timers: %+v", l)
	}
	for _, bad := range []string{`{"label":"a","minutes":0}`, `{"label":"a","minutes":-1}`, `{"label":"a","minutes":2000}`, `{"label":" ","minutes":1}`, `{"label":"a","minutes":1,"stepIndex":0}`} {
		if _, err = run("cook_timers.start", bad); err == nil {
			t.Fatalf("%s must be refused", bad)
		}
	}
}

// TestSchedulerFinishesTimer runs on the real clock and the real scheduler.
func TestSchedulerFinishesTimer(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the scheduler")
	}
	env := testutil.New(t)
	var v api.CookTimer
	env.MustDo(http.MethodPost, "/cook-timers", map[string]any{"label": "煮蛋", "seconds": 1}, &v)
	deadline := time.Now().Add(9 * time.Second)
	for time.Now().Before(deadline) {
		var out struct{ Items []api.CookTimer }
		env.MustDo(http.MethodGet, "/cook-timers", nil, &out)
		if len(out.Items) == 1 && out.Items[0].State == api.Done {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("the scheduler did not finish the timer")
}
