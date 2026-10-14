package recipes_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/api"
)

// ---------- helpers ----------

// serverZone is the time zone testutil gives the server.
var serverZone = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return loc
}()

// day is the date offset days from the fixed clock, in the server time zone.
func day(offset int) string {
	return clock.In(serverZone).AddDate(0, 0, offset).Format(time.DateOnly)
}

type checkin struct {
	ID     int64
	Amount float64
	Source string
}

type fakeHabits struct {
	mu    sync.Mutex
	calls []checkin
	err   error
}

func (f *fakeHabits) Checkin(_ context.Context, id int64, amount float64, source string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, checkin{id, amount, source})
	return f.err
}
func (f *fakeHabits) Today(context.Context) ([]contracts.HabitProgress, error) { return nil, nil }
func (f *fakeHabits) got() []checkin {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]checkin{}, f.calls...)
}

type fakeBody struct {
	points []contracts.BodyPoint
	err    error
}

func (f *fakeBody) Daily(_ context.Context, metric string, _, _ time.Time) ([]contracts.BodyPoint, error) {
	if metric != "weight" {
		return nil, nil
	}
	return f.points, f.err
}

type fakeHidden struct{ hidden map[string]bool }

func (f fakeHidden) Hidden(_ context.Context, name string) bool { return f.hidden[name] }

func (r *rig) withHabits(err error) *fakeHabits {
	h := &fakeHabits{err: err}
	module.Provide[contracts.Habits](r.env.App.Deps.Registry, contracts.HabitsKey, h)
	return h
}

func (r *rig) withBody(points ...contracts.BodyPoint) *fakeBody {
	b := &fakeBody{points: points}
	module.Provide[contracts.BodySeries](r.env.App.Deps.Registry, contracts.BodySeriesKey, b)
	return b
}

type mealList struct {
	Items   []api.Meal      `json:"items"`
	Weights []api.DayWeight `json:"weights"`
}

func (r *rig) meals(t *testing.T, query string) mealList {
	t.Helper()
	var out mealList
	r.env.MustDo(http.MethodGet, "/meals"+query, nil, &out)
	return out
}

func (r *rig) meal(t *testing.T, body map[string]any) api.Meal {
	t.Helper()
	var v api.Meal
	r.env.MustDo(http.MethodPost, "/meals", body, &v)
	return v
}

func (r *rig) getRecipe(t *testing.T, id int64) api.Recipe {
	t.Helper()
	var v api.Recipe
	r.env.MustDo(http.MethodGet, "/recipes/"+idStr(id), nil, &v)
	return v
}

func (r *rig) simple(t *testing.T, title string, nutrition ...string) api.Recipe {
	t.Helper()
	body := map[string]any{"title": title}
	if nutrition != nil {
		body["nutrition"] = nutrition
	}
	return r.create(t, body)
}

func (r *rig) putSettings(t *testing.T, body map[string]any) api.RecipeSettings {
	t.Helper()
	var v api.RecipeSettings
	r.env.MustDo(http.MethodPut, "/recipes/settings", body, &v)
	return v
}

// usageLLM records the purpose and the usage mark of every JSON call.
type usageLLM struct {
	*fakeLLM
	mu       sync.Mutex
	purposes []string
	usage    []contracts.AIUsage
}

func (u *usageLLM) CompleteJSON(ctx context.Context, purpose, system, user string, schema json.RawMessage, out any) error {
	u.mu.Lock()
	u.purposes = append(u.purposes, purpose)
	u.usage = append(u.usage, contracts.AIUsageFrom(ctx))
	u.mu.Unlock()
	return u.fakeLLM.CompleteJSON(ctx, purpose, system, user, schema, out)
}

func withUsageLLM(r *rig) *usageLLM {
	u := &usageLLM{fakeLLM: &fakeLLM{Fake: llm.NewFake()}}
	module.Provide[contracts.LLM](r.env.App.Deps.Registry, contracts.LLMKey, u)
	return u
}

func nutritionAnswer(t *testing.T, kinds ...string) llm.Result {
	t.Helper()
	return answer(t, map[string]any{"nutrition": kinds})
}

// ---------- nutrition ----------

func TestNutritionIsValidated(t *testing.T) {
	r := setup(t)
	v := r.simple(t, "清炒时蔬", "veg_rich", "low_fat")
	if len(v.Nutrition) != 2 || v.Nutrition[0] != "veg_rich" || v.Nutrition[1] != "low_fat" {
		t.Fatalf("created: %+v", v.Nutrition)
	}
	if plain := r.simple(t, "白粥"); plain.Nutrition == nil || len(plain.Nutrition) != 0 {
		t.Fatalf("no nutrition must be an empty list: %#v", plain.Nutrition)
	}
	r.fail(t, http.MethodPost, "/recipes", map[string]any{"title": "a", "nutrition": []string{"spicy"}}, 400, "不认识")
	r.fail(t, http.MethodPost, "/recipes", map[string]any{"title": "a", "nutrition": []string{"oily", "oily"}}, 400, "重复")
	r.fail(t, http.MethodPatch, "/recipes/"+idStr(v.Id), map[string]any{"nutrition": []string{"high_protein", "x"}}, 400, "不认识")
	r.fail(t, http.MethodPatch, "/recipes/"+idStr(v.Id), map[string]any{"nutrition": []string{"oily", "oily"}}, 400, "重复")
	if got := r.getRecipe(t, v.Id); len(got.Nutrition) != 2 {
		t.Fatalf("a refused patch changed it: %+v", got.Nutrition)
	}

	var upd api.Recipe
	r.env.MustDo(http.MethodPatch, "/recipes/"+idStr(v.Id), map[string]any{"nutrition": []string{"oily"}}, &upd)
	if len(upd.Nutrition) != 1 || upd.Nutrition[0] != "oily" || upd.Title != "清炒时蔬" {
		t.Fatalf("patch: %+v", upd)
	}
	// other patches keep it, an empty list clears it
	r.env.MustDo(http.MethodPatch, "/recipes/"+idStr(v.Id), map[string]any{"summary": "x"}, &upd)
	if len(upd.Nutrition) != 1 {
		t.Fatalf("kept: %+v", upd.Nutrition)
	}
	r.env.MustDo(http.MethodPatch, "/recipes/"+idStr(v.Id), map[string]any{"nutrition": []string{}}, &upd)
	if len(upd.Nutrition) != 0 {
		t.Fatalf("cleared: %+v", upd.Nutrition)
	}
}

func TestParseGivesNutrition(t *testing.T) {
	r := setup(t)
	fake := withLLM(r)
	fake.Queue(answer(t, map[string]any{
		"title": "清炒时蔬", "summary": "", "servings": "", "totalMinutes": nil,
		"ingredients": []map[string]any{{"name": "青菜", "amount": "300 克", "note": ""}},
		"steps":       []map[string]any{{"title": "", "text": "炒", "timers": []map[string]any{}, "ingredients": []int{0}}},
		"tips":        "", "tags": []string{"素"},
		"nutrition": []string{"veg_rich", "bogus", "low_fat", "veg_rich", " oily ", ""},
	}), nil)
	var out api.RecipeInput
	r.env.MustDo(http.MethodPost, "/recipes/parse", map[string]any{"text": "清炒时蔬的做法"}, &out)
	if out.Nutrition == nil || len(*out.Nutrition) != 3 || (*out.Nutrition)[0] != "veg_rich" || (*out.Nutrition)[1] != "low_fat" || (*out.Nutrition)[2] != "oily" {
		t.Fatalf("unknown values go, repeats go: %+v", out.Nutrition)
	}
	call := fake.Calls[0]
	if !strings.Contains(string(call.JSONSchema), `"nutrition"`) || !strings.Contains(string(call.JSONSchema), "high_protein") {
		t.Fatalf("schema: %s", call.JSONSchema)
	}
	for _, kind := range []string{"high_protein", "veg_rich", "low_fat", "oily", "high_carb", "sugary"} {
		if !strings.Contains(call.System, kind) {
			t.Fatalf("the prompt must explain %s", kind)
		}
	}
	// the draft can be saved with its classes
	saved := r.create(t, map[string]any{"title": out.Title, "nutrition": out.Nutrition})
	if len(saved.Nutrition) != 3 {
		t.Fatalf("saved: %+v", saved.Nutrition)
	}
}

func TestEstimateNutrition(t *testing.T) {
	r := setup(t)
	v := r.create(t, chicken())
	r.fail(t, http.MethodPost, "/recipes/"+idStr(v.Id)+"/nutrition", nil, 501, "feature_unavailable")

	u := withUsageLLM(r)
	u.Queue(nutritionAnswer(t, "high_protein", "nope", "oily", "oily"), nil)
	var out struct {
		Nutrition []api.RecipeNutrition `json:"nutrition"`
	}
	r.env.MustDo(http.MethodPost, "/recipes/"+idStr(v.Id)+"/nutrition", nil, &out)
	if len(out.Nutrition) != 2 || out.Nutrition[0] != "high_protein" || out.Nutrition[1] != "oily" {
		t.Fatalf("estimate: %+v", out.Nutrition)
	}
	if len(u.purposes) != 1 || u.purposes[0] != "fast" || u.usage[0].Source != "recipes" {
		t.Fatalf("purpose and usage: %v %+v", u.purposes, u.usage)
	}
	if msg := u.Calls[0].Messages[0].Content; !strings.Contains(msg, "临沂炒鸡") || !strings.Contains(msg, "公鸡 1500 克") || !strings.Contains(msg, "加开水炖") {
		t.Fatalf("the model must read the recipe: %s", msg)
	}
	if got := r.getRecipe(t, v.Id); len(got.Nutrition) != 0 {
		t.Fatalf("estimate must not save: %+v", got.Nutrition)
	}

	r.fail(t, http.MethodPost, "/recipes/999/nutrition", nil, 404, "")
	u.Queue(llm.Result{}, errors.New("boom"))
	r.fail(t, http.MethodPost, "/recipes/"+idStr(v.Id)+"/nutrition", nil, 502, "ai_failed")
	u.Queue(nutritionAnswer(t, "nope"), nil)
	r.fail(t, http.MethodPost, "/recipes/"+idStr(v.Id)+"/nutrition", nil, 502, "ai_incomplete")
}

func TestFillNutrition(t *testing.T) {
	r := setup(t)
	r.fail(t, http.MethodPost, "/recipes/nutrition/fill", nil, 501, "feature_unavailable")
	u := withUsageLLM(r)

	// 25 recipes, 3 of them already have classes, so 22 are missing
	var done []api.Recipe
	for i := 0; i < 22; i++ {
		done = append(done, r.simple(t, "菜"+string(rune('A'+i))))
	}
	with := []api.Recipe{r.simple(t, "有分类1", "oily"), r.simple(t, "有分类2", "veg_rich"), r.simple(t, "有分类3", "sugary")}
	before := r.getRecipe(t, done[0].Id)

	sub, cancel := r.env.App.Deps.Bus.Subscribe("recipe.", 64)
	defer cancel()
	for i := 0; i < 20; i++ {
		if i == 4 {
			u.Queue(llm.Result{}, errors.New("boom"))
			continue
		}
		u.Queue(nutritionAnswer(t, "low_fat"), nil)
	}
	var out struct{ Filled, Failed, Remaining int }
	r.env.MustDo(http.MethodPost, "/recipes/nutrition/fill", nil, &out)
	if out.Filled != 19 || out.Failed != 1 || out.Remaining != 3 {
		t.Fatalf("first fill: %+v", out)
	}
	if len(u.purposes) != 20 {
		t.Fatalf("at most 20 per call: %d", len(u.purposes))
	}
	for _, p := range u.purposes {
		if p != "fast" {
			t.Fatalf("purpose: %v", u.purposes)
		}
	}
	updated := 0
	for len(sub) > 0 {
		if ev := <-sub; ev.Topic == "recipe.updated" {
			updated++
		}
	}
	if updated != 19 {
		t.Fatalf("recipe.updated events: %d", updated)
	}
	// the recipes that had classes were not touched, a filled one keeps its place in the list
	for _, w := range with {
		if got := r.getRecipe(t, w.Id); len(got.Nutrition) != 1 || got.Nutrition[0] != w.Nutrition[0] {
			t.Fatalf("touched: %+v", got.Nutrition)
		}
	}
	missing := 0
	for _, it := range r.list(t, "").Items {
		if len(it.Nutrition) == 0 {
			missing++
		}
		if it.Id == done[0].Id && !it.UpdatedAt.Equal(before.UpdatedAt) {
			t.Fatalf("fill must not change the update time")
		}
	}
	if missing != 3 {
		t.Fatalf("missing: %d", missing)
	}

	// the rest: nothing is asked for recipes that have classes
	for i := 0; i < 3; i++ {
		u.Queue(nutritionAnswer(t, "high_carb"), nil)
	}
	r.env.MustDo(http.MethodPost, "/recipes/nutrition/fill", nil, &out)
	if out.Filled != 3 || out.Failed != 0 || out.Remaining != 0 || len(u.purposes) != 23 {
		t.Fatalf("second fill: %+v calls %d", out, len(u.purposes))
	}
	r.env.MustDo(http.MethodPost, "/recipes/nutrition/fill", nil, &out)
	if out.Filled != 0 || out.Failed != 0 || len(u.purposes) != 23 {
		t.Fatalf("nothing to do: %+v", out)
	}
}

// ---------- meals ----------

func TestMealCRUDAndEvents(t *testing.T) {
	r := setup(t)
	rec := r.simple(t, "番茄炒蛋")
	sub, cancel := r.env.App.Deps.Bus.Subscribe("", 64)
	defer cancel()
	next := func() events.Event {
		for {
			select {
			case ev := <-sub:
				if strings.HasPrefix(ev.Topic, "meal.") || (ev.Topic == "recipe.updated") {
					return ev
				}
			case <-time.After(time.Second):
				t.Fatal("no event")
			}
		}
	}
	raw := func(ev events.Event) string { b, _ := json.Marshal(ev.Data); return string(b) }

	// the title comes from the recipe, the date is today
	m := r.meal(t, map[string]any{"slot": "dinner", "recipeId": rec.Id, "cooked": true, "rating": 4, "note": " 咸了 "})
	if m.Title != "番茄炒蛋" || m.Date != day(0) || m.RecipeId == nil || *m.RecipeId != rec.Id || !m.Cooked || m.Rating == nil || *m.Rating != 4 || m.Note != "咸了" || m.Slot != api.Dinner {
		t.Fatalf("created: %+v", m)
	}
	if ev := next(); ev.Topic != "meal.created" || !strings.Contains(raw(ev), `"title":"番茄炒蛋"`) {
		t.Fatalf("event: %s %s", ev.Topic, raw(ev))
	}
	if ev := next(); ev.Topic != "recipe.updated" || raw(ev) != `{"id":`+idStr(rec.Id)+`}` {
		t.Fatalf("recipe event: %s %s", ev.Topic, raw(ev))
	}
	// a meal without a recipe needs a name; the title can be changed
	takeout := r.meal(t, map[string]any{"slot": "lunch", "title": " 外卖盖饭 ", "date": day(-1)})
	if takeout.Title != "外卖盖饭" || takeout.RecipeId != nil || takeout.Cooked || takeout.Rating != nil || takeout.Note != "" {
		t.Fatalf("takeout: %+v", takeout)
	}
	if ev := next(); ev.Topic != "meal.created" {
		t.Fatalf("event: %s", ev.Topic)
	}
	custom := r.meal(t, map[string]any{"slot": "snack", "recipeId": rec.Id, "title": "蛋少一点的"})
	if custom.Title != "蛋少一点的" {
		t.Fatalf("custom title: %+v", custom)
	}
	next()
	next()

	// absent keeps, null clears
	var upd api.Meal
	r.env.MustDo(http.MethodPatch, "/meals/"+idStr(m.Id), map[string]any{"note": "下次少放盐"}, &upd)
	if upd.Note != "下次少放盐" || upd.Rating == nil || *upd.Rating != 4 || upd.RecipeId == nil || upd.Title != "番茄炒蛋" || !upd.Cooked {
		t.Fatalf("patch kept: %+v", upd)
	}
	if ev := next(); ev.Topic != "meal.updated" {
		t.Fatalf("event: %s", ev.Topic)
	}
	if ev := next(); ev.Topic != "recipe.updated" {
		t.Fatalf("event: %s", ev.Topic)
	}
	r.env.MustDo(http.MethodPatch, "/meals/"+idStr(m.Id), map[string]any{"rating": nil}, &upd)
	if upd.Rating != nil || upd.RecipeId == nil {
		t.Fatalf("clear rating: %+v", upd)
	}
	r.env.MustDo(http.MethodPatch, "/meals/"+idStr(m.Id), map[string]any{"recipeId": nil, "slot": "lunch", "date": day(-2), "cooked": false}, &upd)
	if upd.RecipeId != nil || upd.Title != "番茄炒蛋" || upd.Slot != api.Lunch || upd.Date != day(-2) || upd.Cooked {
		t.Fatalf("clear recipe: %+v", upd)
	}
	// an empty title with a recipe takes the recipe name again
	r.env.MustDo(http.MethodPatch, "/meals/"+idStr(custom.Id), map[string]any{"title": ""}, &upd)
	if upd.Title != "番茄炒蛋" {
		t.Fatalf("default title: %+v", upd)
	}
	r.fail(t, http.MethodPatch, "/meals/"+idStr(takeout.Id), map[string]any{"title": " "}, 400, "名称不能为空")

	if status, _ := r.env.Do(http.MethodDelete, "/meals/"+idStr(takeout.Id), nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	var deleted events.Event
	for deleted.Topic != "meal.deleted" {
		deleted = next()
	}
	if raw(deleted) != `{"id":`+idStr(takeout.Id)+`}` {
		t.Fatalf("deleted payload: %s", raw(deleted))
	}
	r.fail(t, http.MethodDelete, "/meals/"+idStr(takeout.Id), nil, 404, "")
	r.fail(t, http.MethodPatch, "/meals/"+idStr(takeout.Id), map[string]any{"note": "x"}, 404, "")
	if got := r.meals(t, ""); len(got.Items) != 2 {
		t.Fatalf("after delete: %+v", got.Items)
	}
	// the audit log has the writes, refused ones included
	for action, want := range map[string]int{"meal.create": 3, "meal.update": 6, "meal.delete": 2} {
		var n int
		if err := r.env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = ?`, action).Scan(&n); err != nil || n != want {
			t.Fatalf("audit %s: %d (%v), want %d", action, n, err, want)
		}
	}
}

func TestMealValidation(t *testing.T) {
	r := setup(t)
	rec := r.simple(t, "粥")
	cases := []struct {
		name string
		body map[string]any
		want string
	}{
		{"bad date", map[string]any{"slot": "lunch", "title": "a", "date": "2026/10/01"}, "YYYY-MM-DD"},
		{"not a day", map[string]any{"slot": "lunch", "title": "a", "date": "2026-02-30"}, "YYYY-MM-DD"},
		{"future", map[string]any{"slot": "lunch", "title": "a", "date": day(1)}, "以后"},
		{"no slot", map[string]any{"title": "a"}, "餐次"},
		{"bad slot", map[string]any{"slot": "brunch", "title": "a"}, "餐次"},
		{"rating 0", map[string]any{"slot": "lunch", "title": "a", "rating": 0}, "1 到 5"},
		{"rating 6", map[string]any{"slot": "lunch", "title": "a", "rating": 6}, "1 到 5"},
		{"no title no recipe", map[string]any{"slot": "lunch"}, "名称不能为空"},
		{"blank title no recipe", map[string]any{"slot": "lunch", "title": "  "}, "名称不能为空"},
		{"long title", map[string]any{"slot": "lunch", "title": strings.Repeat("长", 101)}, "太长"},
		{"long note", map[string]any{"slot": "lunch", "title": "a", "note": strings.Repeat("长", 1001)}, "太长"},
		{"unknown recipe", map[string]any{"slot": "lunch", "recipeId": 999}, "菜谱不存在"},
		{"unknown field", map[string]any{"slot": "lunch", "title": "a", "color": "red"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { r.fail(t, http.MethodPost, "/meals", c.body, 400, c.want) })
	}
	if n := len(r.meals(t, "").Items); n != 0 {
		t.Fatalf("refused meals were saved: %d", n)
	}
	// the borders are fine
	r.meal(t, map[string]any{"slot": "lunch", "title": strings.Repeat("长", 100), "note": strings.Repeat("长", 1000), "rating": 1, "date": day(0)})
	m := r.meal(t, map[string]any{"slot": "snack", "recipeId": rec.Id, "rating": 5})

	for name, body := range map[string]map[string]any{
		"date":   {"date": day(1)},
		"format": {"date": "yesterday"},
		"slot":   {"slot": "brunch"},
		"rating": {"rating": 6},
		"recipe": {"recipeId": 999},
		"title":  {"title": strings.Repeat("长", 101)},
		"note":   {"note": strings.Repeat("长", 1001)},
		"field":  {"mood": "ok"},
	} {
		t.Run("patch "+name, func(t *testing.T) { r.fail(t, http.MethodPatch, "/meals/"+idStr(m.Id), body, 400, "") })
	}
	if got := r.meals(t, "").Items; len(got) != 2 {
		t.Fatalf("meals: %+v", got)
	}
	for _, g := range r.meals(t, "").Items {
		if g.Id == m.Id && (g.Rating == nil || *g.Rating != 5 || g.Slot != api.Snack) {
			t.Fatalf("a refused patch changed the meal: %+v", g)
		}
	}
}

func TestMealListRangeAndOrder(t *testing.T) {
	r := setup(t)
	r.meal(t, map[string]any{"slot": "lunch", "title": "太早", "date": day(-30)})
	edge := r.meal(t, map[string]any{"slot": "lunch", "title": "边上", "date": day(-29)})
	// order inside a day: breakfast, lunch, dinner, snack; days newest first
	snack := r.meal(t, map[string]any{"slot": "snack", "title": "饼干"})
	dinner := r.meal(t, map[string]any{"slot": "dinner", "title": "晚饭"})
	breakfast := r.meal(t, map[string]any{"slot": "breakfast", "title": "早饭"})
	lunch2 := r.meal(t, map[string]any{"slot": "lunch", "title": "午饭2"})
	lunch1 := r.meal(t, map[string]any{"slot": "lunch", "title": "午饭1", "date": day(0)})
	yesterday := r.meal(t, map[string]any{"slot": "breakfast", "title": "昨天早饭", "date": day(-1)})

	got := r.meals(t, "")
	var ids []int64
	for _, m := range got.Items {
		ids = append(ids, m.Id)
	}
	want := []int64{breakfast.Id, lunch2.Id, lunch1.Id, dinner.Id, snack.Id, yesterday.Id, edge.Id}
	if len(ids) != len(want) {
		t.Fatalf("default range: %+v", got.Items)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("order: %v want %v", ids, want)
		}
	}
	if got.Weights == nil {
		t.Fatal("weights must be a list")
	}

	if n := len(r.meals(t, "?from="+day(-30)).Items); n != 8 {
		t.Fatalf("from: %d", n)
	}
	if n := len(r.meals(t, "?from="+day(-1)+"&to="+day(-1)).Items); n != 1 {
		t.Fatalf("one day: %d", n)
	}
	if n := len(r.meals(t, "?to="+day(-2)).Items); n != 2 {
		t.Fatalf("to only is the 30 days up to it: %d", n)
	}
	r.fail(t, http.MethodGet, "/meals?from=2026-1-1", nil, 400, "YYYY-MM-DD")
	r.fail(t, http.MethodGet, "/meals?to=abc", nil, 400, "YYYY-MM-DD")
	r.fail(t, http.MethodGet, "/meals?from="+day(0)+"&to="+day(-1), nil, 400, "不能晚于")
	r.fail(t, http.MethodGet, "/meals?from="+day(-366)+"&to="+day(0), nil, 400, "366")
	r.meals(t, "?from="+day(-365)+"&to="+day(0))
}

func TestMealWeights(t *testing.T) {
	r := setup(t)
	status, raw := r.env.Do(http.MethodGet, "/meals", nil, nil)
	if status != 200 || !strings.Contains(string(raw), `"weights":[]`) {
		t.Fatalf("no body data: %d %s", status, raw)
	}
	body := r.withBody(contracts.BodyPoint{Date: day(-2), Value: 70.5}, contracts.BodyPoint{Date: day(-1), Value: 70.25})
	got := r.meals(t, "")
	if len(got.Weights) != 2 || got.Weights[0].Date != day(-2) || got.Weights[0].Weight != 70.5 || got.Weights[1].Weight != 70.25 {
		t.Fatalf("weights: %+v", got.Weights)
	}
	// a failing source only leaves the weights out
	body.err = errors.New("down")
	status, raw = r.env.Do(http.MethodGet, "/meals", nil, nil)
	if status != 200 || !strings.Contains(string(raw), `"weights":[]`) {
		t.Fatalf("failing source: %d %s", status, raw)
	}
	// a hidden habits module gives no body data
	body.err = nil
	module.Provide[contracts.HiddenModules](r.env.App.Deps.Registry, contracts.HiddenModulesKey, fakeHidden{map[string]bool{"habits": true}})
	if got := r.meals(t, ""); len(got.Weights) != 0 {
		t.Fatalf("hidden: %+v", got.Weights)
	}
}

func TestDeletedRecipeKeepsItsMeals(t *testing.T) {
	r := setup(t)
	rec := r.simple(t, "红烧肉")
	m := r.meal(t, map[string]any{"slot": "dinner", "recipeId": rec.Id, "cooked": true})
	if status, _ := r.env.Do(http.MethodDelete, "/recipes/"+idStr(rec.Id), nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete recipe: %d", status)
	}
	got := r.meals(t, "").Items
	if len(got) != 1 || got[0].Id != m.Id || got[0].RecipeId != nil || got[0].Title != "红烧肉" || !got[0].Cooked {
		t.Fatalf("meal after the recipe is gone: %+v", got)
	}
}

func TestRecipeMealStatistics(t *testing.T) {
	r := setup(t)
	a := r.simple(t, "A")
	b := r.simple(t, "B")
	r.meal(t, map[string]any{"slot": "dinner", "recipeId": a.Id, "cooked": true, "rating": 5, "date": day(-3)})
	r.meal(t, map[string]any{"slot": "dinner", "recipeId": a.Id, "cooked": true, "rating": 4, "date": day(-1)})
	// eaten out: rated, but not cooked, so it is not counted as a cooking and is not the last cooked day
	r.meal(t, map[string]any{"slot": "lunch", "recipeId": a.Id, "cooked": false, "rating": 2, "date": day(0)})
	// cooked without a rating: counts as a cooking, not in the average
	r.meal(t, map[string]any{"slot": "snack", "recipeId": a.Id, "cooked": true, "date": day(-2)})
	r.meal(t, map[string]any{"slot": "snack", "title": "别的", "cooked": true})

	check := func(v api.Recipe) {
		t.Helper()
		if v.CookedCount != 3 || v.LastCookedOn != day(-1) || v.AvgRating == nil || *v.AvgRating != 3.7 {
			t.Fatalf("A: count %d last %q avg %v", v.CookedCount, v.LastCookedOn, v.AvgRating)
		}
	}
	check(r.getRecipe(t, a.Id))
	for _, it := range r.list(t, "").Items {
		switch it.Id {
		case a.Id:
			check(it)
		case b.Id:
			if it.CookedCount != 0 || it.LastCookedOn != "" || it.AvgRating != nil {
				t.Fatalf("B has no meals: %+v", it)
			}
		}
	}
	status, raw := r.env.Do(http.MethodGet, "/recipes/"+idStr(b.Id), nil, nil)
	if status != 200 || !strings.Contains(string(raw), `"avgRating":null`) || !strings.Contains(string(raw), `"lastCookedOn":""`) {
		t.Fatalf("empty statistics: %s", raw)
	}
	// deleting a meal changes them
	for _, m := range r.meals(t, "").Items {
		if m.RecipeId != nil && m.Rating != nil && *m.Rating == 5 {
			r.env.MustDo(http.MethodDelete, "/meals/"+idStr(m.Id), nil, nil)
		}
	}
	if v := r.getRecipe(t, a.Id); v.CookedCount != 2 || v.AvgRating == nil || *v.AvgRating != 3 {
		t.Fatalf("after delete: %+v %v", v.CookedCount, v.AvgRating)
	}
}

// ---------- settings and the habit ----------

func TestRecipeSettings(t *testing.T) {
	r := setup(t)
	var s api.RecipeSettings
	r.env.MustDo(http.MethodGet, "/recipes/settings", nil, &s)
	if s.HabitId != nil || s.DietGoal != api.DietGoalNone || len(s.Staples) != 14 || s.Staples[0] != "盐" || s.Staples[13] != "开水" {
		t.Fatalf("defaults: %+v", s)
	}
	s = r.putSettings(t, map[string]any{"habitId": 7, "dietGoal": "lose"})
	if s.HabitId == nil || *s.HabitId != 7 || s.DietGoal != api.DietGoalLose || len(s.Staples) != 14 {
		t.Fatalf("put: %+v", s)
	}
	// only the given fields change
	s = r.putSettings(t, map[string]any{"staples": []string{" 盐 ", "糖", "盐", "", "八角"}})
	if strings.Join(s.Staples, ",") != "盐,糖,八角" || s.HabitId == nil || s.DietGoal != api.DietGoalLose {
		t.Fatalf("staples: %+v", s)
	}
	s = r.putSettings(t, map[string]any{"staples": []string{}})
	if s.Staples == nil || len(s.Staples) != 0 {
		t.Fatalf("empty staples stay empty: %#v", s.Staples)
	}
	s = r.putSettings(t, map[string]any{"habitId": nil})
	if s.HabitId != nil || s.DietGoal != api.DietGoalLose {
		t.Fatalf("null clears the habit: %+v", s)
	}
	s = r.putSettings(t, map[string]any{})
	if s.DietGoal != api.DietGoalLose {
		t.Fatalf("empty body: %+v", s)
	}

	many := make([]string, 51)
	for i := range many {
		many[i] = "料" + string(rune('A'+i%26)) + string(rune('a'+i/26))
	}
	r.fail(t, http.MethodPut, "/recipes/settings", map[string]any{"dietGoal": "gain", "staples": many}, 400, "50")
	r.fail(t, http.MethodPut, "/recipes/settings", map[string]any{"staples": []string{strings.Repeat("长", 21)}}, 400, "20")
	r.fail(t, http.MethodPut, "/recipes/settings", map[string]any{"dietGoal": "bulk"}, 400, "饮食目标")
	r.fail(t, http.MethodPut, "/recipes/settings", map[string]any{"habitId": 0}, 400, "习惯")
	r.fail(t, http.MethodPut, "/recipes/settings", map[string]any{"mood": 1}, 400, "")
	r.env.MustDo(http.MethodGet, "/recipes/settings", nil, &s)
	if s.DietGoal != api.DietGoalLose {
		t.Fatalf("a refused body must change nothing: %+v", s)
	}
	if got := r.putSettings(t, map[string]any{"staples": many[:50]}); len(got.Staples) != 50 {
		t.Fatalf("50 staples are fine: %d", len(got.Staples))
	}
}

func TestCookedMealChecksInTheHabit(t *testing.T) {
	r := setup(t)
	h := r.withHabits(nil)
	rec := r.simple(t, "番茄炒蛋")

	// no habit chosen: nothing is checked in
	r.meal(t, map[string]any{"slot": "dinner", "recipeId": rec.Id, "cooked": true})
	if len(h.got()) != 0 {
		t.Fatalf("no habit set: %+v", h.got())
	}
	r.putSettings(t, map[string]any{"habitId": 7})
	// not cooked by the user: no check-in
	r.meal(t, map[string]any{"slot": "lunch", "title": "外卖"})
	r.meal(t, map[string]any{"slot": "lunch", "title": "外卖", "cooked": false})
	if len(h.got()) != 0 {
		t.Fatalf("not cooked: %+v", h.got())
	}
	// one check-in for every cooked meal
	r.meal(t, map[string]any{"slot": "dinner", "recipeId": rec.Id, "cooked": true})
	m := r.meal(t, map[string]any{"slot": "snack", "title": "饼干", "cooked": true})
	if got := h.got(); len(got) != 2 || got[0] != (checkin{7, 1, "recipes"}) || got[1] != (checkin{7, 1, "recipes"}) {
		t.Fatalf("check-ins: %+v", got)
	}
	// updating and deleting neither adds nor takes back
	r.env.MustDo(http.MethodPatch, "/meals/"+idStr(m.Id), map[string]any{"cooked": false, "note": "x"}, nil)
	r.env.MustDo(http.MethodPatch, "/meals/"+idStr(m.Id), map[string]any{"cooked": true}, nil)
	r.env.MustDo(http.MethodDelete, "/meals/"+idStr(m.Id), nil, nil)
	if n := len(h.got()); n != 2 {
		t.Fatalf("update and delete: %d", n)
	}

	// clearing the habit stops it
	r.putSettings(t, map[string]any{"habitId": nil})
	r.meal(t, map[string]any{"slot": "dinner", "title": "x", "cooked": true})
	if n := len(h.got()); n != 2 {
		t.Fatalf("cleared: %d", n)
	}
}

func TestCheckinFailuresDoNotFailTheMeal(t *testing.T) {
	r := setup(t)
	r.putSettings(t, map[string]any{"habitId": 7})

	// the check-in is refused, for example the habit was deleted
	h := r.withHabits(errors.New("没有这个习惯"))
	m := r.meal(t, map[string]any{"slot": "dinner", "title": "炒饭", "cooked": true})
	if len(h.got()) != 1 || m.Id == 0 {
		t.Fatalf("refused check-in: %+v %+v", h.got(), m)
	}
	if got := r.meals(t, "").Items; len(got) != 1 {
		t.Fatalf("the meal must be saved: %+v", got)
	}

	// the habits module is not there
	module.Provide[contracts.Habits](r.env.App.Deps.Registry, contracts.HabitsKey, nil)
	r.meal(t, map[string]any{"slot": "dinner", "title": "炒面", "cooked": true})

	// the habits module is hidden
	h2 := r.withHabits(nil)
	module.Provide[contracts.HiddenModules](r.env.App.Deps.Registry, contracts.HiddenModulesKey, fakeHidden{map[string]bool{"habits": true}})
	r.meal(t, map[string]any{"slot": "dinner", "title": "炒粉", "cooked": true})
	if len(h2.got()) != 0 {
		t.Fatalf("hidden: %+v", h2.got())
	}
	if got := r.meals(t, "").Items; len(got) != 3 {
		t.Fatalf("meals: %d", len(got))
	}
}

// ---------- recommendation ----------

func rc(id int64, updated int, nutrition ...string) api.Recipe {
	n := make([]api.RecipeNutrition, len(nutrition))
	for i, k := range nutrition {
		n[i] = api.RecipeNutrition(k)
	}
	return api.Recipe{Id: id, Title: "R" + idStr(id), Nutrition: n, UpdatedAt: clock.Add(time.Duration(updated) * time.Hour), Tags: []string{}}
}

func cooked(r api.Recipe, n int, avg float32) api.Recipe {
	r.CookedCount = n
	if avg > 0 {
		r.AvgRating = &avg
	}
	return r
}

var today = time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)

func ids(items []api.RecipeRecommendation) []int64 {
	out := make([]int64, len(items))
	for i, it := range items {
		out[i] = it.Recipe.Id
	}
	return out
}

func sameIDs(t *testing.T, items []api.RecipeRecommendation, want ...int64) {
	t.Helper()
	got := ids(items)
	if len(got) != len(want) {
		t.Fatalf("order: %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order: %v want %v", got, want)
		}
	}
}

func reasons(t *testing.T, items []api.RecipeRecommendation, id int64) string {
	t.Helper()
	for _, it := range items {
		if it.Recipe.Id == id {
			if it.Reasons == nil {
				t.Fatal("reasons must be a list")
			}
			return strings.Join(it.Reasons, "|")
		}
	}
	t.Fatalf("recipe %d is not recommended", id)
	return ""
}

func TestRecommendLoseWeight(t *testing.T) {
	// all were cooked once, long ago enough to get no bonus, so only the goal counts
	light := cooked(rc(1, 1, "low_fat"), 1, 0)
	veg := cooked(rc(2, 2, "veg_rich"), 1, 0)
	both := cooked(rc(3, 3, "low_fat", "veg_rich"), 1, 0)
	plain := cooked(rc(4, 4), 1, 0)
	oily := cooked(rc(5, 5, "oily"), 1, 0)
	sweet := cooked(rc(6, 6, "sugary", "veg_rich"), 1, 0) // +3 and -3
	last := map[int64]string{1: "2026-10-01", 2: "2026-10-01", 3: "2026-10-01", 4: "2026-10-01", 5: "2026-10-01", 6: "2026-10-01"}
	got := recipes.Recommend([]api.Recipe{oily, plain, light, veg, both, sweet}, last, api.DietGoalLose, nil, today)
	// light, veg and both score 3 (newer first); plain and sweet 0; oily -3
	sameIDs(t, got, 3, 2, 1, 6, 4, 5)
	if r := reasons(t, got, 1); r != "少油，适合减重" {
		t.Fatalf("reason: %q", r)
	}
	if r := reasons(t, got, 2); r != "蔬菜多，适合减重" {
		t.Fatalf("reason: %q", r)
	}
	if r := reasons(t, got, 3); r != "少油、蔬菜多，适合减重" {
		t.Fatalf("reason: %q", r)
	}
	if r := reasons(t, got, 5); r != "" {
		t.Fatalf("a penalty has no reason: %q", r)
	}
	// without a goal the classes do not matter
	none := recipes.Recommend([]api.Recipe{oily, light}, last, api.DietGoalNone, nil, today)
	sameIDs(t, none, 5, 1)
}

func TestRecommendOtherGoals(t *testing.T) {
	prot := cooked(rc(1, 1, "high_protein"), 1, 0)
	veg := cooked(rc(2, 2, "veg_rich"), 1, 0)
	oily := cooked(rc(3, 3, "oily"), 1, 0)
	last := map[int64]string{1: "2026-10-01", 2: "2026-10-01", 3: "2026-10-01"}
	gain := recipes.Recommend([]api.Recipe{veg, oily, prot}, last, api.DietGoalGain, nil, today)
	sameIDs(t, gain, 1, 3, 2)
	if r := reasons(t, gain, 1); r != "高蛋白，适合增肌" {
		t.Fatalf("gain: %q", r)
	}
	steady := recipes.Recommend([]api.Recipe{prot, oily, veg}, last, api.DietGoalSteady, nil, today)
	sameIDs(t, steady, 2, 3, 1)
	if r := reasons(t, steady, 2); r != "蔬菜多" {
		t.Fatalf("steady: %q", r)
	}
	if r := reasons(t, steady, 1); r != "" {
		t.Fatalf("steady protein: %q", r)
	}
}

func TestRecommendRecentMealsGoBack(t *testing.T) {
	a := cooked(rc(1, 1), 1, 0)
	b := cooked(rc(2, 2), 1, 0)
	c := cooked(rc(3, 3), 1, 0)
	// 6 days ago is inside the last 7 days, 7 days ago is not
	last := map[int64]string{1: "2026-10-08", 2: "2026-10-07", 3: "2026-10-14"}
	got := recipes.Recommend([]api.Recipe{c, b, a}, last, api.DietGoalNone, nil, today)
	sameIDs(t, got, 2, 3, 1)
	if reasons(t, got, 1) != "" || reasons(t, got, 3) != "" {
		t.Fatal("being eaten lately has no reason")
	}
	// a recipe eaten lately but never cooked keeps its bonus and its penalty
	d := rc(4, 0)
	got = recipes.Recommend([]api.Recipe{d, b}, map[int64]string{4: "2026-10-13", 2: "2026-10-01"}, api.DietGoalNone, nil, today)
	sameIDs(t, got, 2, 4)
}

func TestRecommendRatingAndHistory(t *testing.T) {
	loved := cooked(rc(1, 1), 2, 4.5)
	exact := cooked(rc(2, 2), 2, 4)
	meh := cooked(rc(3, 3), 2, 3.9)
	fresh := rc(4, 4)
	forgotten := cooked(rc(5, 5), 1, 0)
	almost := cooked(rc(6, 6), 1, 0)
	last := map[int64]string{1: "2026-10-01", 2: "2026-10-01", 3: "2026-10-01", 5: "2026-09-04", 6: "2026-09-15"}
	got := recipes.Recommend([]api.Recipe{loved, exact, meh, fresh, forgotten, almost}, last, api.DietGoalNone, nil, today)
	if r := reasons(t, got, 1); r != "你打过 4.5 分" {
		t.Fatalf("loved: %q", r)
	}
	if r := reasons(t, got, 2); r != "你打过 4 分" {
		t.Fatalf("exact: %q", r)
	}
	if r := reasons(t, got, 3); r != "" {
		t.Fatalf("meh: %q", r)
	}
	if r := reasons(t, got, 4); r != "还没做过" {
		t.Fatalf("fresh: %q", r)
	}
	// 40 days since 09-04; 29 days since 09-15 is not long enough
	if r := reasons(t, got, 5); r != "有 40 天没吃了" {
		t.Fatalf("forgotten: %q", r)
	}
	if r := reasons(t, got, 6); r != "" {
		t.Fatalf("almost: %q", r)
	}
	// 2 for the rating, then 1 for the history; ties go to the recent one
	sameIDs(t, got, 2, 1, 5, 4, 6, 3)
	exactly30 := recipes.Recommend([]api.Recipe{cooked(rc(7, 1), 1, 0)}, map[int64]string{7: "2026-09-14"}, api.DietGoalNone, nil, today)
	if r := reasons(t, exactly30, 7); r != "有 30 天没吃了" {
		t.Fatalf("30 days: %q", r)
	}
}

func TestRecommendWeightGainWithoutGoal(t *testing.T) {
	change := 1.2
	light := cooked(rc(1, 1, "low_fat"), 1, 0)
	veg := cooked(rc(2, 2, "veg_rich"), 1, 0)
	oily := cooked(rc(3, 3, "oily"), 1, 0)
	plain := cooked(rc(4, 4), 1, 0)
	last := map[int64]string{1: "2026-10-01", 2: "2026-10-01", 3: "2026-10-01", 4: "2026-10-01"}
	all := []api.Recipe{oily, plain, light, veg}

	if g := recipes.EffectiveGoal(api.DietGoalNone, &change); g != "lose_half" {
		t.Fatalf("effective goal: %s", g)
	}
	got := recipes.Recommend(all, last, api.DietGoalNone, &change, today)
	sameIDs(t, got, 2, 1, 4, 3)
	if r := reasons(t, got, 1); r != "最近 30 天体重涨了 1.2 kg" {
		t.Fatalf("reason: %q", r)
	}
	if reasons(t, got, 3) != "" {
		t.Fatal("a penalty has no reason")
	}

	// only half a point: a recipe with a rating (+2) beats the light one (+1.5), while a full goal would not
	loved := cooked(rc(5, 0), 2, 4.5)
	last[5] = "2026-10-01"
	half := recipes.Recommend([]api.Recipe{light, loved}, last, api.DietGoalNone, &change, today)
	sameIDs(t, half, 5, 1)
	full := recipes.Recommend([]api.Recipe{light, loved}, last, api.DietGoalLose, &change, today)
	sameIDs(t, full, 1, 5)
	// and a half penalty: oily (-1.5) is behind a plain recipe (0) but a rating 2 minus 3 would lose against 0 only with a full penalty
	lovedOily := cooked(rc(6, 0, "oily"), 2, 4.5)
	last[6] = "2026-10-01"
	sameIDs(t, recipes.Recommend([]api.Recipe{plain, lovedOily}, last, api.DietGoalNone, &change, today), 6, 4)
	sameIDs(t, recipes.Recommend([]api.Recipe{plain, lovedOily}, last, api.DietGoalLose, &change, today), 4, 6)

	// a smaller rise, a fall, or a set goal: no change in the rules
	for _, c := range []float64{0.9, 0, -2} {
		c := c
		if g := recipes.EffectiveGoal(api.DietGoalNone, &c); g != "none" {
			t.Fatalf("change %v: %s", c, g)
		}
	}
	if g := recipes.EffectiveGoal(api.DietGoalNone, nil); g != "none" {
		t.Fatalf("no data: %s", g)
	}
	if g := recipes.EffectiveGoal(api.DietGoalGain, &change); g != "gain" {
		t.Fatalf("goal set: %s", g)
	}
	if g := recipes.EffectiveGoal(api.DietGoalSteady, &change); g != "steady" {
		t.Fatalf("goal set: %s", g)
	}
	one := 1.0
	if g := recipes.EffectiveGoal(api.DietGoalNone, &one); g != "lose_half" {
		t.Fatalf("1 kg counts: %s", g)
	}
}

func TestRecommendLimitAndTies(t *testing.T) {
	var all []api.Recipe
	for i := int64(1); i <= 9; i++ {
		all = append(all, cooked(rc(i, int(i)), 1, 0))
	}
	last := map[int64]string{}
	for i := int64(1); i <= 9; i++ {
		last[i] = "2026-10-01"
	}
	got := recipes.Recommend(all, last, api.DietGoalNone, nil, today)
	sameIDs(t, got, 9, 8, 7, 6, 5, 4)
	if got := recipes.Recommend(nil, nil, api.DietGoalNone, nil, today); got == nil || len(got) != 0 {
		t.Fatalf("empty: %#v", got)
	}
}

func TestRecommendEndpoint(t *testing.T) {
	r := setup(t)
	// no recipes: an empty list, with the context
	status, raw := r.env.Do(http.MethodGet, "/recipes/recommend", nil, nil)
	if status != 200 || !strings.Contains(string(raw), `"items":[]`) || !strings.Contains(string(raw), `"weightChange30d":null`) {
		t.Fatalf("empty: %d %s", status, raw)
	}

	light := r.simple(t, "清蒸鱼", "low_fat", "high_protein")
	oily := r.simple(t, "炸鸡", "oily")
	r.simple(t, "白粥")
	r.putSettings(t, map[string]any{"dietGoal": "lose"})
	var rec api.RecipeRecommendations
	r.env.MustDo(http.MethodGet, "/recipes/recommend", nil, &rec)
	if len(rec.Items) != 3 || rec.Items[0].Recipe.Id != light.Id || rec.Items[2].Recipe.Id != oily.Id {
		t.Fatalf("lose: %+v", ids(rec.Items))
	}
	if strings.Join(rec.Items[0].Reasons, "|") != "少油，适合减重|还没做过" {
		t.Fatalf("reasons: %v", rec.Items[0].Reasons)
	}
	if rec.Context.DietGoal != api.DietGoalLose || rec.Context.EffectiveGoal != "lose" || rec.Context.WeightChange30d != nil || rec.Context.MissingNutrition != 1 {
		t.Fatalf("context: %+v", rec.Context)
	}

	// eaten yesterday goes behind
	r.meal(t, map[string]any{"slot": "dinner", "recipeId": light.Id, "cooked": true, "date": day(-1)})
	r.env.MustDo(http.MethodGet, "/recipes/recommend", nil, &rec)
	// light: +3 -4 = -1, oily: -3 +1 = -2, plain: +1, so plain, light, oily
	if got := ids(rec.Items); got[1] != light.Id || got[2] != oily.Id {
		t.Fatalf("recently eaten: %v", got)
	}
	if rec.Items[1].Recipe.CookedCount != 1 {
		t.Fatalf("the recipe carries its statistics: %+v", rec.Items[1].Recipe)
	}

	// no goal, weight went up 1.2 kg over the window
	r.putSettings(t, map[string]any{"dietGoal": "none"})
	r.withBody(contracts.BodyPoint{Date: day(-20), Value: 70}, contracts.BodyPoint{Date: day(-10), Value: 72}, contracts.BodyPoint{Date: day(-1), Value: 71.2})
	r.env.MustDo(http.MethodGet, "/recipes/recommend", nil, &rec)
	if rec.Context.EffectiveGoal != "lose_half" || rec.Context.DietGoal != api.DietGoalNone || rec.Context.WeightChange30d == nil || *rec.Context.WeightChange30d != 1.2 {
		t.Fatalf("context with weight: %+v", rec.Context)
	}
	// fewer than two records: no weight data
	r.withBody(contracts.BodyPoint{Date: day(-1), Value: 71.2})
	r.env.MustDo(http.MethodGet, "/recipes/recommend", nil, &rec)
	if rec.Context.EffectiveGoal != "none" || rec.Context.WeightChange30d != nil {
		t.Fatalf("one record: %+v", rec.Context)
	}
}

// ---------- AI actions ----------

func TestMealAIActions(t *testing.T) {
	r := setup(t)
	h := r.withHabits(nil)
	r.putSettings(t, map[string]any{"habitId": 3, "dietGoal": "lose"})
	run := func(name, args string) (string, error) {
		out, err := r.env.App.Deps.Actions.Run(context.Background(), name, json.RawMessage(args))
		raw, _ := json.Marshal(out)
		return string(raw), err
	}
	rec := r.simple(t, "番茄炒蛋", "veg_rich")

	out, err := run("meals.log", `{"slot":"dinner","recipeId":`+idStr(rec.Id)+`,"cooked":true,"rating":5,"note":"好吃"}`)
	if err != nil || !strings.Contains(out, `"title":"番茄炒蛋"`) || !strings.Contains(out, `"date":"`+day(0)+`"`) || !strings.Contains(out, `"cooked":true`) {
		t.Fatalf("log: %s %v", out, err)
	}
	if got := h.got(); len(got) != 1 || got[0] != (checkin{3, 1, "recipes"}) {
		t.Fatalf("the action checks in too: %+v", got)
	}
	if out, err = run("meals.log", `{"slot":"lunch","title":"面条","date":"`+day(-3)+`"}`); err != nil || !strings.Contains(out, `"title":"面条"`) {
		t.Fatalf("log by name: %s %v", out, err)
	}
	for _, bad := range []string{`{"slot":"lunch"}`, `{"slot":"brunch","title":"a"}`, `{"slot":"lunch","title":"a","rating":9}`, `{"slot":"lunch","title":"a","date":"` + day(2) + `"}`, `{"title":"a"}`, `{"slot":"lunch","title":"a","recipeId":999}`} {
		if _, err = run("meals.log", bad); err == nil {
			t.Fatalf("%s must be refused", bad)
		}
	}
	if n := len(r.meals(t, "").Items); n != 2 {
		t.Fatalf("saved: %d", n)
	}

	out, err = run("meals.list", `{}`)
	if err != nil || !strings.Contains(out, "番茄炒蛋") || !strings.Contains(out, "面条") {
		t.Fatalf("list default 7 days: %s %v", out, err)
	}
	if out, err = run("meals.list", `{"days":2}`); err != nil || !strings.Contains(out, "番茄炒蛋") || strings.Contains(out, "面条") {
		t.Fatalf("list 2 days: %s %v", out, err)
	}
	if out, err = run("meals.list", ``); err != nil || !strings.Contains(out, "面条") {
		t.Fatalf("list without arguments: %s %v", out, err)
	}
	if _, err = run("meals.list", `{"days":500}`); err != nil {
		t.Fatalf("more than 90 is cut: %v", err)
	}
	if _, err = run("meals.list", `{"days":0}`); err == nil {
		t.Fatal("0 days must be refused")
	}

	out, err = run("recipes.recommend", `{}`)
	if err != nil || !strings.Contains(out, `"title":"番茄炒蛋"`) || !strings.Contains(out, `"reasons"`) || !strings.Contains(out, `"effectiveGoal":"lose"`) {
		t.Fatalf("recommend: %s %v", out, err)
	}
	if strings.Contains(out, `"steps"`) {
		t.Fatalf("recommend must be short: %s", out)
	}
}
