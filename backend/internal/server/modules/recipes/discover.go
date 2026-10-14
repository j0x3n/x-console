package recipes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/db"
)

// Finding new recipes on the internet (B211). The search itself is the AI
// module's contracts.WebSearch; this file builds the request from the taste
// of the user, cleans every dish it gets back and keeps them as candidates.

const (
	defaultDiscoverCount = 3
	maxDiscoverCount     = 5
	maxHint              = 100
	maxReason            = 100
	// dailyDiscoverLimit is how many times a day (server time zone) the
	// button works. A failed search counts too.
	dailyDiscoverLimit = 10
	// profileTop is how many tags and nutrition classes the taste profile lists.
	profileTop = 5
	// candidateKeepDays is how long a candidate that was dealt with is kept.
	candidateKeepDays = 30
	// maxKnownTitles is how many known dish names go into the request.
	maxKnownTitles = 300
	maxSources     = 20
	candidateEvery = 24 * time.Hour
)

var errTooManyRuns = httpx.NewError(http.StatusTooManyRequests, "too_many_requests", "今天已经找了 10 次，明天再找")

// ---------- the request to the model ----------

var nutritionLabels = map[string]string{
	"high_protein": "高蛋白", "veg_rich": "蔬菜多", "low_fat": "少油",
	"oily": "油多", "high_carb": "主食多", "sugary": "糖多",
}

var dietGoalLabels = map[api.DietGoal]string{
	api.DietGoalNone: "不设", api.DietGoalLose: "减重", api.DietGoalGain: "增肌", api.DietGoalSteady: "保持",
}

const discoverSystem = `你是家常菜谱的搜索整理员。上网找用户可能喜欢的新菜谱，按网页原文整理，只返回符合 JSON Schema 的 JSON。

规则：
- 只找家常能做的菜。不要找需要特殊设备或很难买到的食材的菜。
- 按网页原文整理，不要编用量。原文没写用量的，amount 写“适量”。
- 每道菜的 sourceUrl 必须是你真的读到的那个网页的地址，不要自己拼地址。
- reason 用一句话说为什么符合用户的口味或这次的要求，不超过 100 字。
- 不要找用户已经有的菜，也不要在这次的结果里重复。
- 找不到合适的就少给几道，不要凑数。

每道菜的整理格式：
- ingredients 按原文列出全部食材。amount 写用量，比如“1 汤匙”“500 克”。note 写处理方法，比如“切段”。用量和处理方法分开写。
- steps 按原文的顺序，不要合并。title 是步骤的小标题，没有就留空。text 是这一步的正文。
- 原文明确写了某一步的时间，放进那一步的 timers。label 是计时的名字，seconds 是秒，maxSeconds 是时间有范围时的上限，没有范围写 null。
- 每一步的 ingredients 是这一步用到的食材，写 ingredients 数组里的下标，从 0 开始。只写这一步明确要加的，不要重复。
- tips 放原文剩下的要点，保留 Markdown。没有就留空。
- tags 给 3 到 6 个，包括口味、菜系、主料、做法。每个不超过 20 字。
- nutrition 是营养分类，按食材和用量估，可以多选。只能从下表选：
  - high_protein：主料是肉、蛋、豆制品、鱼虾
  - veg_rich：蔬菜占用量的一半以上
  - low_fat：用油少，不油炸，不用肥肉
  - oily：用油多、油炸、肥肉多
  - high_carb：米、面、土豆这类主食占比大
  - sugary：糖、蜂蜜用得多
  拿不准的不选。
- servings、totalMinutes 照原文，没写的字符串留空、用时写 null。
- 用中文。原文是外文的，翻译成中文。`

// discoverSchema is {"dishes": [dish]}. A dish is what parseSchema describes
// plus sourceUrl and reason.
var discoverSchema = func() json.RawMessage {
	var dish map[string]any
	if err := json.Unmarshal([]byte(parseSchema), &dish); err != nil {
		panic(err)
	}
	props := dish["properties"].(map[string]any)
	props["sourceUrl"] = map[string]any{"type": "string"}
	props["reason"] = map[string]any{"type": "string"}
	dish["required"] = append(dish["required"].([]any), "sourceUrl", "reason")
	raw, err := json.Marshal(map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"dishes": map[string]any{"type": "array", "items": dish}},
		"required":             []string{"dishes"},
		"additionalProperties": false,
	})
	if err != nil {
		panic(err)
	}
	return raw
}()

// aiDish is one dish in the answer.
type aiDish struct {
	aiRecipe
	SourceURL string `json:"sourceUrl"`
	Reason    string `json:"reason"`
}

type discoverAnswer struct {
	Dishes []aiDish `json:"dishes"`
}

// tasteProfile is what the user likes, from the meals they cooked and rated 4 or more.
type tasteProfile struct {
	tags      []string
	nutrition []string
	goal      api.DietGoal
}

// topCounts returns the most common keys, ties by name. The spelling of the
// first one seen is kept.
func topCounts(counts map[string]int, names map[string]string, n int) []string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return names[keys[i]] < names[keys[j]]
	})
	if len(keys) > n {
		keys = keys[:n]
	}
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = names[k]
	}
	return out
}

// profile builds the taste profile. Without any liked dish it falls back to
// all recipes.
func (m *Module) profile(ctx context.Context, recipes []api.Recipe) (tasteProfile, error) {
	p := tasteProfile{tags: []string{}, nutrition: []string{}, goal: api.DietGoalNone}
	set, err := m.loadSettings(ctx)
	if err != nil {
		return p, err
	}
	p.goal = set.DietGoal
	liked, err := m.q.LikedRecipeIDs(ctx)
	if err != nil {
		return p, err
	}
	likedSet := make(map[int64]bool, len(liked))
	for _, id := range liked {
		likedSet[id] = true
	}
	use := recipes
	if len(likedSet) > 0 {
		use = nil
		for _, r := range recipes {
			if likedSet[r.Id] {
				use = append(use, r)
			}
		}
	}
	tagCount, tagName, nutCount := map[string]int{}, map[string]string{}, map[string]int{}
	for _, r := range use {
		for _, t := range r.Tags {
			k := strings.ToLower(t)
			tagCount[k]++
			if _, ok := tagName[k]; !ok {
				tagName[k] = t
			}
		}
		for _, n := range r.Nutrition {
			nutCount[string(n)]++
		}
	}
	p.tags = topCounts(tagCount, tagName, profileTop)
	nutName := make(map[string]string, len(nutCount))
	for k := range nutCount {
		nutName[k] = k
	}
	p.nutrition = topCounts(nutCount, nutName, profileTop)
	return p, nil
}

// titleKey is how dish names are compared: no case, no spaces.
func titleKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), ""))
}

// discoverTask is the user message of the search.
func discoverTask(p tasteProfile, hint string, count int, known []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "请找 %d 道新的家常菜谱。\n\n", count)
	b.WriteString("用户的口味：")
	if len(p.tags) == 0 && len(p.nutrition) == 0 {
		b.WriteString("还没有记录，找常见的家常菜。\n")
	} else {
		if len(p.tags) > 0 {
			b.WriteString("常做的菜的标签有 " + strings.Join(p.tags, "、") + "。")
		}
		if len(p.nutrition) > 0 {
			labels := make([]string, len(p.nutrition))
			for i, n := range p.nutrition {
				labels[i] = nutritionLabels[n]
			}
			b.WriteString("常见的营养分类有 " + strings.Join(labels, "、") + "。")
		}
		b.WriteString("\n")
	}
	b.WriteString("饮食目标：" + dietGoalLabels[p.goal] + "。\n")
	if hint != "" {
		b.WriteString("这次的要求：" + hint + "\n")
	}
	if len(known) > 0 {
		b.WriteString("\n用户已经有这些菜，不要重复：" + strings.Join(known, "、") + "\n")
	}
	return b.String()
}

// ---------- the candidates ----------

func (m *Module) candidateView(row db.RecipeCandidate) api.RecipeCandidate {
	v := api.RecipeCandidate{
		Id: row.ID, Reason: row.Reason, Sources: parseJSON[api.RecipeSource](row.Sources),
		Status: api.RecipeCandidateStatus(row.Status), RecipeId: row.RecipeID, CreatedAt: row.CreatedAt,
	}
	if err := json.Unmarshal([]byte(row.Draft), &v.Draft); err != nil {
		v.Draft = api.RecipeInput{}
	}
	return v
}

func (m *Module) pendingCandidates(ctx context.Context) ([]api.RecipeCandidate, error) {
	rows, err := m.q.ListRecipeCandidatesByStatus(ctx, "pending")
	if err != nil {
		return nil, err
	}
	out := make([]api.RecipeCandidate, len(rows))
	for i, row := range rows {
		out[i] = m.candidateView(row)
	}
	return out, nil
}

// sourceKey makes two spellings of one page equal: no scheme, no www, no
// query or fragment, no trailing slash.
func sourceKey(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	return host + strings.TrimRight(u.EscapedPath(), "/")
}

// ---------- the daily limit ----------

// useRun counts one search for today and says how many are left. It returns
// errTooManyRuns when the day's limit is used up.
func (m *Module) useRun(ctx context.Context) (int, error) {
	var runs int
	err := m.d.DB.QueryRowContext(ctx, `INSERT INTO recipe_discover_runs (day, runs) VALUES (?1, 1)
ON CONFLICT(day) DO UPDATE SET runs = runs + 1 WHERE runs < ?2 RETURNING runs`, dayKey(m.today()), dailyDiscoverLimit).Scan(&runs)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errTooManyRuns
	}
	return dailyDiscoverLimit - runs, err
}

func (m *Module) runsLeft(ctx context.Context) (int, error) {
	runs, err := m.q.GetRecipeDiscoverRuns(ctx, dayKey(m.today()))
	if errors.Is(err, sql.ErrNoRows) {
		return dailyDiscoverLimit, nil
	}
	if err != nil {
		return 0, err
	}
	return max(dailyDiscoverLimit-int(runs), 0), nil
}

// ---------- POST /recipes/discover ----------

// webSearch is the search of the AI module, or nil when there is none.
func (m *Module) webSearch() contracts.WebSearch {
	ws, _ := module.Lookup[contracts.WebSearch](m.d.Registry, contracts.WebSearchKey)
	return ws
}

func errSearchOff() error {
	return httpx.NewError(http.StatusNotImplemented, "feature_unavailable", contracts.ErrWebSearchOff.Error())
}

// DiscoverRecipes implements api.ServerInterface.
func (m *Module) DiscoverRecipes(w http.ResponseWriter, r *http.Request) {
	var body api.RecipeDiscoverInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	items, hint, count, err := m.discover(ctx, body)
	// The audit is written for every search that reached the model.
	if !errors.Is(err, errTooManyRuns) {
		m.d.Audit.Record(ctx, "recipe.discover", "", map[string]any{"hint": hint, "count": count, "kept": len(items)}, err)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (m *Module) discover(ctx context.Context, in api.RecipeDiscoverInput) (items []api.RecipeCandidate, hint string, count int, err error) {
	hint = strings.TrimSpace(str(in.Hint))
	if utf8.RuneCountInString(hint) > maxHint {
		return nil, hint, 0, httpx.Invalid("这次的要求太长了，最多 100 字")
	}
	count = defaultDiscoverCount
	if in.Count != nil {
		count = *in.Count
	}
	if count < 1 || count > maxDiscoverCount {
		return nil, hint, count, httpx.Invalid("数量要在 1 到 5 之间")
	}
	ws := m.webSearch()
	if ws == nil || ws.Mode(ctx) == "off" {
		return nil, hint, count, errSearchOff()
	}

	recipes, _, err := m.list(ctx, "", "")
	if err != nil {
		return nil, hint, count, err
	}
	pending, err := m.pendingCandidates(ctx)
	if err != nil {
		return nil, hint, count, err
	}
	profile, err := m.profile(ctx, recipes)
	if err != nil {
		return nil, hint, count, err
	}
	known := map[string]bool{}
	var names []string
	for _, r := range recipes {
		known[titleKey(r.Title)] = true
		names = append(names, r.Title)
	}
	for _, c := range pending {
		known[titleKey(c.Draft.Title)] = true
		names = append(names, c.Draft.Title)
	}
	if len(names) > maxKnownTitles {
		names = names[:maxKnownTitles]
	}

	// From here on the search costs money, so it counts even when it fails.
	if _, err := m.useRun(ctx); err != nil {
		return nil, hint, count, err
	}
	var answer discoverAnswer
	sources, err := ws.Research(contracts.WithAIUsage(ctx, "recipes", ""), contracts.WebResearch{
		Purpose:    "recipes.discover",
		System:     discoverSystem,
		Task:       discoverTask(profile, hint, count, names),
		Schema:     discoverSchema,
		MaxSources: count + 1,
	}, &answer)
	if errors.Is(err, contracts.ErrWebSearchOff) {
		return nil, hint, count, errSearchOff()
	}
	if err != nil {
		return nil, hint, count, httpx.NewError(http.StatusBadGateway, "web_search_failed", "联网找菜谱失败："+err.Error())
	}

	shown := make([]api.RecipeSource, 0, len(sources))
	byKey := map[string]bool{}
	for _, s := range sources {
		if len(shown) >= maxSources {
			break
		}
		shown = append(shown, api.RecipeSource{Title: clip(s.Title, 200), Url: s.URL})
		byKey[sourceKey(s.URL)] = true
	}
	sourcesJSON, _ := json.Marshal(shown)

	items = []api.RecipeCandidate{}
	for _, dish := range answer.Dishes {
		if len(items) >= count {
			break
		}
		d, ok := cleanDish(dish, byKey)
		if !ok || known[titleKey(d.title)] {
			continue
		}
		known[titleKey(d.title)] = true
		draftJSON, _ := json.Marshal(inputOf(d))
		now := m.now().UTC()
		row, err := m.q.InsertRecipeCandidate(ctx, db.InsertRecipeCandidateParams{
			Draft: string(draftJSON), Reason: clip(dish.Reason, maxReason), Sources: string(sourcesJSON),
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return items, hint, count, err
		}
		v := m.candidateView(row)
		m.d.Bus.Publish("recipe_candidate.created", v)
		items = append(items, v)
	}
	return items, hint, count, nil
}

// cleanDish repairs and checks one dish of the answer. It says false for a
// dish that cannot be used. A source address that is not one of the pages
// the search used is removed: the model may not have read it.
func cleanDish(dish aiDish, sources map[string]bool) (draft, bool) {
	d := repair(dish.aiRecipe)
	d.sourceURL = ""
	if u := strings.TrimSpace(dish.SourceURL); u != "" && len(u) <= maxURL && (len(sources) == 0 || sources[sourceKey(u)]) {
		if p, err := url.Parse(u); err == nil && (p.Scheme == "http" || p.Scheme == "https") && p.Host != "" {
			d.sourceURL = u
		}
	}
	d, err := validate(d)
	if err != nil || (len(d.steps) == 0 && len(d.ingredients) == 0) {
		return d, false
	}
	return d, true
}

// ---------- GET /recipe-candidates ----------

// ListRecipeCandidates implements api.ServerInterface.
func (m *Module) ListRecipeCandidates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := m.pendingCandidates(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	recipes, _, err := m.list(ctx, "", "")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := m.profile(ctx, recipes)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	left, err := m.runsLeft(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	mode := "off"
	if ws := m.webSearch(); ws != nil {
		mode = ws.Mode(ctx)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "profile": api.RecipeTasteProfile{
		Tags: p.tags, Nutrition: nutritionValues(p.nutrition), DietGoal: p.goal,
		WebSearch: api.RecipeTasteProfileWebSearch(mode), RunsLeft: left,
	}})
}

// ---------- PATCH /recipe-candidates/{id} ----------

// UpdateRecipeCandidate implements api.ServerInterface.
func (m *Module) UpdateRecipeCandidate(w http.ResponseWriter, r *http.Request, id api.CandidateId) {
	var body api.RecipeCandidatePatch
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	v, err := m.updateCandidate(ctx, id, body)
	m.d.Audit.Record(ctx, "recipe_candidate.update", strconv.FormatInt(id, 10), map[string]any{"status": string(body.Status), "recipeId": body.RecipeId}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) updateCandidate(ctx context.Context, id int64, p api.RecipeCandidatePatch) (api.RecipeCandidate, error) {
	if !p.Status.Valid() {
		return api.RecipeCandidate{}, httpx.Invalid("状态只能改成 accepted 或 dismissed")
	}
	if p.RecipeId != nil && p.Status != api.RecipeCandidatePatchStatusAccepted {
		return api.RecipeCandidate{}, httpx.Invalid("只有收进菜单的候选才能带菜谱")
	}
	row, err := m.q.GetRecipeCandidate(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return api.RecipeCandidate{}, httpx.ErrNotFound
	}
	if err != nil {
		return api.RecipeCandidate{}, err
	}
	if row.Status != "pending" {
		return api.RecipeCandidate{}, httpx.NewError(http.StatusConflict, "conflict", "这个候选已经处理过了")
	}
	if p.RecipeId != nil {
		if _, err := m.get(ctx, *p.RecipeId); err != nil {
			if errors.Is(err, httpx.ErrNotFound) {
				return api.RecipeCandidate{}, httpx.Invalid("菜谱不存在")
			}
			return api.RecipeCandidate{}, err
		}
	}
	row, err = m.q.UpdateRecipeCandidateStatus(ctx, db.UpdateRecipeCandidateStatusParams{
		Status: string(p.Status), RecipeID: p.RecipeId, UpdatedAt: m.now().UTC(), ID: id,
	})
	if errors.Is(err, sql.ErrNoRows) {
		// Someone else changed it between the read and the write.
		return api.RecipeCandidate{}, httpx.NewError(http.StatusConflict, "conflict", "这个候选已经处理过了")
	}
	if err != nil {
		return api.RecipeCandidate{}, err
	}
	v := m.candidateView(row)
	m.d.Bus.Publish("recipe_candidate.updated", v)
	return v, nil
}

// ---------- cleanup ----------

// cleanCandidates removes candidates that were dealt with more than 30 days
// ago, and the old counters of the daily limit.
func (m *Module) cleanCandidates(ctx context.Context) error {
	cutoff := m.now().UTC().AddDate(0, 0, -candidateKeepDays)
	if _, err := m.q.DeleteOldRecipeCandidates(ctx, cutoff); err != nil {
		return err
	}
	return m.q.DeleteOldRecipeDiscoverRuns(ctx, dayKey(m.today().AddDate(0, 0, -candidateKeepDays)))
}
