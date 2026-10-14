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
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/api"
)

// ---------- helpers ----------

// fakeSearch stands in for the AI module's web search (B211).
type fakeSearch struct {
	mu      sync.Mutex
	mode    string
	dishes  []map[string]any
	sources []contracts.WebSource
	err     error
	calls   []contracts.WebResearch
}

func (f *fakeSearch) Mode(context.Context) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mode
}

func (f *fakeSearch) Research(_ context.Context, in contracts.WebResearch, out any) ([]contracts.WebSource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, in)
	if f.mode == "off" {
		return nil, contracts.ErrWebSearchOff
	}
	if f.err != nil {
		return nil, f.err
	}
	raw, _ := json.Marshal(map[string]any{"dishes": f.dishes})
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, err
	}
	return f.sources, nil
}

func (f *fakeSearch) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeSearch) last() contracts.WebResearch {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

func (r *rig) withSearch(mode string) *fakeSearch {
	f := &fakeSearch{mode: mode}
	module.Provide[contracts.WebSearch](r.env.App.Deps.Registry, contracts.WebSearchKey, f)
	return f
}

// dish is a good dish of the answer. extra replaces fields.
func dish(title string, extra map[string]any) map[string]any {
	d := map[string]any{
		"title": title, "summary": "清爽", "servings": "2 人", "totalMinutes": 20,
		"ingredients": []map[string]any{
			{"name": "黄瓜", "amount": "2 根", "note": "拍碎"},
			{"name": "盐", "amount": "适量", "note": ""},
		},
		"steps": []map[string]any{
			{"title": "切", "text": "黄瓜拍碎切段", "timers": []map[string]any{}, "ingredients": []int{0}},
			{"title": "拌", "text": "加盐拌匀", "timers": []map[string]any{{"label": "腌", "seconds": 600, "maxSeconds": nil}}, "ingredients": []int{0, 1}},
		},
		"tips": "", "tags": []string{"凉菜", "家常"}, "nutrition": []string{"veg_rich"},
		"sourceUrl": "https://a.example/cucumber", "reason": "你常做家常菜，这道是清爽的凉菜",
	}
	for k, v := range extra {
		d[k] = v
	}
	return d
}

type candidateList struct {
	Items   []api.RecipeCandidate  `json:"items"`
	Profile api.RecipeTasteProfile `json:"profile"`
}

func (r *rig) candidates(t *testing.T) candidateList {
	t.Helper()
	var out candidateList
	r.env.MustDo(http.MethodGet, "/recipe-candidates", nil, &out)
	return out
}

func (r *rig) discover(t *testing.T, body map[string]any) []api.RecipeCandidate {
	t.Helper()
	var out struct {
		Items []api.RecipeCandidate `json:"items"`
	}
	r.env.MustDo(http.MethodPost, "/recipes/discover", body, &out)
	return out.Items
}

func (r *rig) patchCandidate(t *testing.T, id int64, body map[string]any) api.RecipeCandidate {
	t.Helper()
	var v api.RecipeCandidate
	r.env.MustDo(http.MethodPatch, "/recipe-candidates/"+idStr(id), body, &v)
	return v
}

func (r *rig) auditCount(t *testing.T, action string) int {
	t.Helper()
	var n int
	if err := r.env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = ?`, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// ---------- tests ----------

func TestDiscoverNeedsWebSearch(t *testing.T) {
	r := setup(t)
	// the real AI module is there and web search is off by default
	r.fail(t, http.MethodPost, "/recipes/discover", map[string]any{}, 501, "还没有打开联网搜索")
	if got := r.candidates(t); got.Profile.WebSearch != "off" || got.Profile.RunsLeft != 10 || len(got.Items) != 0 {
		t.Fatalf("candidates: %+v", got)
	}

	f := r.withSearch("off")
	r.fail(t, http.MethodPost, "/recipes/discover", map[string]any{}, 501, "去 设置 → AI 里打开")
	if f.callCount() != 0 {
		t.Fatal("the search ran while it was off")
	}
	if got := r.candidates(t); got.Profile.RunsLeft != 10 {
		t.Fatalf("a refused search was counted: %+v", got.Profile)
	}
}

func TestDiscoverValidation(t *testing.T) {
	r := setup(t)
	f := r.withSearch("brave")
	for name, body := range map[string]map[string]any{
		"count 0":   {"count": 0},
		"count 6":   {"count": 6},
		"long hint": {"hint": strings.Repeat("汤", 101)},
	} {
		t.Run(name, func(t *testing.T) {
			r.fail(t, http.MethodPost, "/recipes/discover", body, 400, "")
		})
	}
	if f.callCount() != 0 || r.candidates(t).Profile.RunsLeft != 10 {
		t.Fatal("a refused request used the search")
	}
	// 100 characters are fine, and an empty body means 3 dishes
	r.discover(t, map[string]any{"hint": strings.Repeat("汤", 100)})
	r.discover(t, map[string]any{})
	if got := f.last(); got.MaxSources != 4 || !strings.Contains(got.Task, "请找 3 道") {
		t.Fatalf("defaults: %d %q", got.MaxSources, got.Task)
	}
}

func TestDiscoverKeepsCleanCandidates(t *testing.T) {
	r := setup(t)
	f := r.withSearch("brave")
	sub, cancel := r.env.App.Deps.Bus.Subscribe("recipe_candidate.", 16)
	defer cancel()

	// taste: two dishes cooked and rated 4 or more, one rated low
	r.create(t, map[string]any{"title": "番茄炒蛋", "tags": []string{"家常", "酸甜"}, "nutrition": []string{"high_protein"}})
	tofu := r.create(t, map[string]any{"title": "麻婆豆腐", "tags": []string{"家常", "辣", "川菜"}, "nutrition": []string{"high_protein"}})
	pork := r.create(t, map[string]any{"title": "红烧肉", "tags": []string{"甜", "红烧"}, "nutrition": []string{"oily"}})
	eggID := r.list(t, "?q=番茄").Items[0].Id
	r.meal(t, map[string]any{"slot": "dinner", "recipeId": eggID, "cooked": true, "rating": 5})
	r.meal(t, map[string]any{"slot": "dinner", "recipeId": tofu.Id, "cooked": true, "rating": 4})
	r.meal(t, map[string]any{"slot": "dinner", "recipeId": pork.Id, "cooked": true, "rating": 2})
	r.env.MustDo(http.MethodPut, "/recipes/settings", map[string]any{"dietGoal": "lose"}, nil)

	long := strings.Repeat("好", 150)
	f.sources = []contracts.WebSource{{Title: "黄瓜的做法", URL: "https://a.example/cucumber"}, {Title: "豆腐的做法", URL: "https://b.example/tofu?x=1"}}
	f.dishes = []map[string]any{
		dish("凉拌黄瓜", map[string]any{"reason": long, "sourceUrl": "https://www.a.example/cucumber/?utm=1"}),
		dish("番茄 炒蛋", nil), // already in the menu, spaces ignored
		dish("Cold Tofu", map[string]any{"sourceUrl": "https://nowhere.example/x"}), // not a page that was read
		dish("cold  TOFU", nil), // repeats the dish above
		dish("", nil),           // no name
		dish("空菜", map[string]any{"ingredients": []map[string]any{}, "steps": []map[string]any{}}),
		dish("拍黄瓜", map[string]any{"nutrition": []string{"veg_rich", "bogus"}, "totalMinutes": 99999}),
		dish("多余的菜", nil), // over the count
	}
	items := r.discover(t, map[string]any{"hint": " 想吃清淡的凉菜 ", "count": 3})
	if len(items) != 3 {
		t.Fatalf("kept %d: %+v", len(items), items)
	}
	a, b, c := items[0], items[1], items[2]
	if a.Draft.Title != "凉拌黄瓜" || a.Status != api.RecipeCandidateStatusPending || a.RecipeId != nil || a.Id == 0 {
		t.Fatalf("first: %+v", a)
	}
	if len([]rune(a.Reason)) != 100 {
		t.Fatalf("reason was not cut to 100 characters: %d", len([]rune(a.Reason)))
	}
	if a.Draft.SourceUrl == nil || *a.Draft.SourceUrl != "https://www.a.example/cucumber/?utm=1" {
		t.Fatalf("a source in another spelling must stay: %+v", a.Draft.SourceUrl)
	}
	if a.Draft.Ingredients == nil || len(*a.Draft.Ingredients) != 2 || a.Draft.Steps == nil || len(*a.Draft.Steps) != 2 {
		t.Fatalf("draft: %+v", a.Draft)
	}
	if timers := (*a.Draft.Steps)[1].Timers; timers == nil || len(*timers) != 1 || (*timers)[0].Seconds != 600 {
		t.Fatalf("timers: %+v", (*a.Draft.Steps)[1])
	}
	if len(a.Sources) != 2 || a.Sources[1].Url != "https://b.example/tofu?x=1" || a.Sources[0].Title != "黄瓜的做法" {
		t.Fatalf("sources: %+v", a.Sources)
	}
	if b.Draft.Title != "Cold Tofu" || b.Draft.SourceUrl == nil || *b.Draft.SourceUrl != "" {
		t.Fatalf("an address nobody read must be removed: %+v", b.Draft)
	}
	// a model slip is repaired, not a reason to lose the dish
	if c.Draft.Title != "拍黄瓜" || c.Draft.TotalMinutes != nil || c.Draft.Nutrition == nil || len(*c.Draft.Nutrition) != 1 {
		t.Fatalf("repaired: %+v", c.Draft)
	}

	// what the model was asked
	call := f.last()
	if call.Purpose != "recipes.discover" || call.MaxSources != 4 {
		t.Fatalf("request: %+v", call)
	}
	for _, want := range []string{"请找 3 道", "常做的菜的标签有 家常、川菜、辣、酸甜。", "常见的营养分类有 高蛋白。", "饮食目标：减重", "这次的要求：想吃清淡的凉菜", "不要重复：", "番茄炒蛋", "麻婆豆腐", "红烧肉"} {
		if !strings.Contains(call.Task, want) {
			t.Fatalf("task misses %q:\n%s", want, call.Task)
		}
	}
	if strings.Contains(call.Task, "标签有 家常、川菜、辣、酸甜、") || strings.Contains(call.Task, "红烧、") {
		t.Fatalf("a dish rated low shaped the taste:\n%s", call.Task)
	}
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Items struct {
				Required   []string                   `json:"required"`
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(call.Schema, &schema); err != nil {
		t.Fatal(err)
	}
	items0 := schema.Properties["dishes"].Items
	for _, key := range []string{"title", "ingredients", "steps", "nutrition", "sourceUrl", "reason"} {
		if _, ok := items0.Properties[key]; !ok {
			t.Fatalf("schema misses %s", key)
		}
	}
	if !strings.Contains(strings.Join(items0.Required, ","), "sourceUrl") || !strings.Contains(strings.Join(items0.Required, ","), "reason") {
		t.Fatalf("required: %v", items0.Required)
	}

	// events and audit
	for _, id := range []int64{a.Id, b.Id, c.Id} {
		select {
		case ev := <-sub:
			raw, _ := json.Marshal(ev.Data)
			if ev.Topic != "recipe_candidate.created" || !strings.Contains(string(raw), `"id":`+idStr(id)) {
				t.Fatalf("event: %s %s", ev.Topic, raw)
			}
		case <-time.After(time.Second):
			t.Fatal("no event")
		}
	}
	if r.auditCount(t, "recipe.discover") != 1 {
		t.Fatal("no audit record")
	}

	// the list: pending only, newest first, with the taste profile
	got := r.candidates(t)
	if len(got.Items) != 3 || got.Items[0].Id != c.Id || got.Items[2].Id != a.Id {
		t.Fatalf("list: %+v", got.Items)
	}
	p := got.Profile
	if strings.Join(p.Tags, ",") != "家常,川菜,辣,酸甜" || len(p.Nutrition) != 1 || p.Nutrition[0] != api.RecipeNutrition("high_protein") ||
		p.DietGoal != api.DietGoalLose || p.WebSearch != "brave" || p.RunsLeft != 9 {
		t.Fatalf("profile: %+v", p)
	}

	// a dish that is already waiting is not added again
	f.dishes = []map[string]any{dish("凉拌黄瓜", nil), dish("COLD TOFU", nil)}
	status, raw := r.env.Do(http.MethodPost, "/recipes/discover", map[string]any{}, nil)
	if status != 200 || !strings.Contains(string(raw), `"items":[]`) {
		t.Fatalf("nothing new: %d %s", status, raw)
	}
	if len(r.candidates(t).Items) != 3 {
		t.Fatal("a repeated dish was kept")
	}
}

func TestDiscoverWithoutAnyRatingUsesAllRecipes(t *testing.T) {
	r := setup(t)
	f := r.withSearch("openai")
	r.create(t, map[string]any{"title": "清炒时蔬", "tags": []string{"素", "清淡"}, "nutrition": []string{"veg_rich"}})
	r.create(t, map[string]any{"title": "白灼菜心", "tags": []string{"素"}, "nutrition": []string{"veg_rich", "low_fat"}})
	r.discover(t, map[string]any{})
	if task := f.last().Task; !strings.Contains(task, "常做的菜的标签有 素、清淡。") || !strings.Contains(task, "蔬菜多、少油") {
		t.Fatalf("task:\n%s", task)
	}
	if p := r.candidates(t).Profile; strings.Join(p.Tags, ",") != "素,清淡" || p.WebSearch != "openai" {
		t.Fatalf("profile: %+v", p)
	}
}

func TestDiscoverDailyLimit(t *testing.T) {
	r := setup(t)
	f := r.withSearch("brave")
	for i := 0; i < 10; i++ {
		if items := r.discover(t, map[string]any{}); len(items) != 0 {
			t.Fatalf("run %d: %+v", i, items)
		}
	}
	if left := r.candidates(t).Profile.RunsLeft; left != 0 {
		t.Fatalf("runs left: %d", left)
	}
	r.fail(t, http.MethodPost, "/recipes/discover", map[string]any{}, 429, "今天已经找了 10 次，明天再找")
	if f.callCount() != 10 {
		t.Fatalf("the eleventh search ran: %d", f.callCount())
	}

	// the day is the one of the server time zone (Shanghai), not UTC:
	// 2026-10-15 15:30 UTC is already the next UTC day but 23:30 in Shanghai
	r.now = time.Date(2026, 10, 15, 15, 30, 0, 0, time.UTC)
	r.fail(t, http.MethodPost, "/recipes/discover", map[string]any{}, 429, "明天再找")
	// 00:30 in Shanghai
	r.now = time.Date(2026, 10, 15, 16, 30, 0, 0, time.UTC)
	if left := r.candidates(t).Profile.RunsLeft; left != 10 {
		t.Fatalf("new day: %d", left)
	}
	r.discover(t, map[string]any{})
	if left := r.candidates(t).Profile.RunsLeft; left != 9 {
		t.Fatalf("one run: %d", left)
	}
}

func TestDiscoverFailureCountsAndIsReported(t *testing.T) {
	r := setup(t)
	f := r.withSearch("brave")
	f.err = errors.New("Brave 搜索次数用完了")
	r.fail(t, http.MethodPost, "/recipes/discover", map[string]any{}, 502, "Brave 搜索次数用完了")
	if left := r.candidates(t).Profile.RunsLeft; left != 9 {
		t.Fatalf("a failed search must count: %d", left)
	}
	if r.auditCount(t, "recipe.discover") != 1 {
		t.Fatal("a failed search must be in the audit log")
	}
	// the answer cannot be read at all: the search counts, nothing is kept
	f.err = nil
	f.dishes = []map[string]any{{"title": 5}}
	r.fail(t, http.MethodPost, "/recipes/discover", map[string]any{}, 502, "联网找菜谱失败")
}

func TestRecipeCandidateAcceptAndDismiss(t *testing.T) {
	r := setup(t)
	f := r.withSearch("openai")
	f.dishes = []map[string]any{dish("凉拌黄瓜", nil), dish("蒜蓉青菜", nil), dish("蛋花汤", nil)}
	sub, cancel := r.env.App.Deps.Bus.Subscribe("recipe_candidate.updated", 8)
	defer cancel()
	items := r.discover(t, map[string]any{})
	if len(items) != 3 {
		t.Fatalf("items: %+v", items)
	}
	cucumber, greens, soup := items[0], items[1], items[2]

	// the front end saves the draft as a recipe, then marks the candidate
	saved := r.create(t, map[string]any{"title": cucumber.Draft.Title, "sourceUrl": "https://a.example/cucumber"})
	v := r.patchCandidate(t, cucumber.Id, map[string]any{"status": "accepted", "recipeId": saved.Id})
	if v.Status != api.RecipeCandidateStatusAccepted || v.RecipeId == nil || *v.RecipeId != saved.Id || v.Draft.Title != "凉拌黄瓜" {
		t.Fatalf("accepted: %+v", v)
	}
	select {
	case ev := <-sub:
		raw, _ := json.Marshal(ev.Data)
		if !strings.Contains(string(raw), `"status":"accepted"`) {
			t.Fatalf("event: %s", raw)
		}
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
	if d := r.patchCandidate(t, greens.Id, map[string]any{"status": "dismissed"}); d.Status != api.RecipeCandidateStatusDismissed || d.RecipeId != nil {
		t.Fatalf("dismissed: %+v", d)
	}
	// accepted without a recipe id is fine too
	if a := r.patchCandidate(t, soup.Id, map[string]any{"status": "accepted"}); a.RecipeId != nil || a.Status != api.RecipeCandidateStatusAccepted {
		t.Fatalf("accepted alone: %+v", a)
	}

	// dealt with candidates leave the list, and cannot be changed again
	if got := r.candidates(t); len(got.Items) != 0 {
		t.Fatalf("list: %+v", got.Items)
	}
	r.fail(t, http.MethodPatch, "/recipe-candidates/"+idStr(cucumber.Id), map[string]any{"status": "dismissed"}, 409, "处理过了")
	r.fail(t, http.MethodPatch, "/recipe-candidates/"+idStr(greens.Id), map[string]any{"status": "accepted"}, 409, "")
	if r.auditCount(t, "recipe_candidate.update") != 5 {
		t.Fatalf("audit: %d", r.auditCount(t, "recipe_candidate.update"))
	}

	// errors
	r.fail(t, http.MethodPatch, "/recipe-candidates/99999", map[string]any{"status": "dismissed"}, 404, "")
	r.fail(t, http.MethodPatch, "/recipe-candidates/"+idStr(cucumber.Id), map[string]any{"status": "pending"}, 400, "")
	r.fail(t, http.MethodPatch, "/recipe-candidates/"+idStr(cucumber.Id), map[string]any{"status": "bogus"}, 400, "")
	r.fail(t, http.MethodPatch, "/recipe-candidates/"+idStr(cucumber.Id), map[string]any{}, 400, "")

	// a dismissed candidate cannot carry a recipe, and the recipe must exist
	f.dishes = []map[string]any{dish("皮蛋豆腐", nil)}
	next := r.discover(t, map[string]any{})[0]
	r.fail(t, http.MethodPatch, "/recipe-candidates/"+idStr(next.Id), map[string]any{"status": "dismissed", "recipeId": saved.Id}, 400, "")
	r.fail(t, http.MethodPatch, "/recipe-candidates/"+idStr(next.Id), map[string]any{"status": "accepted", "recipeId": 99999}, 400, "菜谱不存在")
	if got := r.candidates(t); len(got.Items) != 1 || got.Items[0].Id != next.Id {
		t.Fatalf("a refused patch changed the candidate: %+v", got.Items)
	}

	// deleting the recipe clears the link and keeps the candidate
	r.env.MustDo(http.MethodDelete, "/recipes/"+idStr(saved.Id), nil, nil)
	var link *int64
	if err := r.env.App.Deps.DB.QueryRow(`SELECT recipe_id FROM recipe_candidates WHERE id = ?`, cucumber.Id).Scan(&link); err != nil || link != nil {
		t.Fatalf("link after delete: %v %v", link, err)
	}
}

func TestRecipeCandidatesAreCleanedAfter30Days(t *testing.T) {
	r := setup(t)
	insert := func(status string, age time.Duration) int64 {
		t.Helper()
		at := r.now.Add(-age)
		res, err := r.env.App.Deps.DB.Exec(`INSERT INTO recipe_candidates (draft, reason, sources, status, created_at, updated_at) VALUES ('{"title":"x"}', '', '[]', ?, ?, ?)`, status, at, at)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	day := 24 * time.Hour
	oldAccepted := insert("accepted", 31*day)
	oldDismissed := insert("dismissed", 40*day)
	newDismissed := insert("dismissed", 29*day)
	oldPending := insert("pending", 90*day)
	for _, d := range []string{"2026-08-01", "2026-10-10"} {
		if _, err := r.env.App.Deps.DB.Exec(`INSERT INTO recipe_discover_runs (day, runs) VALUES (?, 3)`, d); err != nil {
			t.Fatal(err)
		}
	}
	if err := recipes.CleanCandidates(r.m, context.Background()); err != nil {
		t.Fatal(err)
	}
	exists := func(id int64) bool {
		var n int
		if err := r.env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM recipe_candidates WHERE id = ?`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	if exists(oldAccepted) || exists(oldDismissed) || !exists(newDismissed) || !exists(oldPending) {
		t.Fatalf("cleanup: accepted=%v dismissed=%v recent=%v pending=%v", exists(oldAccepted), exists(oldDismissed), exists(newDismissed), exists(oldPending))
	}
	var days int
	if err := r.env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM recipe_discover_runs`).Scan(&days); err != nil || days != 1 {
		t.Fatalf("run counters: %d %v", days, err)
	}
}
