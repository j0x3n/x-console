// Package recipes keeps the user's recipes and the cooking timers (B209).
// A recipe is structured data: ingredients, steps with timers, tags. The
// tutorial text it came from stays in a note. Timers live on the server so a
// finished timer notifies even when no page is open. See docs/specs/B209.md.
package recipes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/db"
)

const (
	maxTitle          = 100
	maxSummary        = 500
	maxServings       = 50
	maxTotalMinutes   = 10080
	maxIngredients    = 100
	maxIngredientName = 50
	maxAmount         = 30
	maxIngredientNote = 100
	maxSteps          = 100
	maxStepTitle      = 30
	maxStepText       = 2000
	maxStepTimers     = 5
	maxTimerLabel     = 30
	maxTimerSeconds   = 86400
	maxTips           = 20000
	maxTags           = 20
	maxTagLen         = 20
	maxURL            = 2000
	maxParseText      = 20000
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "recipes.module"

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time

	// timerMu serializes the changes of timers, so the 5 second check and a
	// request cannot both act on the same timer, and the limit holds.
	timerMu sync.Mutex
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now}
	d.Notify.OnAction("cook_timer.", m.handleAction)
	m.registerActions()
	module.Provide[*Module](d.Registry, ServiceKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "recipes" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the timer check.
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("recipes.timers", timerCheckEvery, func(ctx context.Context) error {
		return m.checkTimers(ctx, m.now())
	})
	return nil
}

func (m *Module) now() time.Time { return m.nowFn() }

// ---------- stored form ----------

// draft is a recipe as it is validated and stored.
type draft struct {
	title, summary, servings string
	totalMinutes             *int
	ingredients              []api.RecipeIngredient
	steps                    []api.RecipeStep
	tips                     string
	tags                     []string
	sourceNoteID             *int64
	sourceURL                string
}

func draftOf(r api.Recipe) draft {
	return draft{
		title: r.Title, summary: r.Summary, servings: r.Servings, totalMinutes: r.TotalMinutes,
		ingredients: r.Ingredients, steps: r.Steps, tips: r.Tips, tags: r.Tags,
		sourceNoteID: r.SourceNoteId, sourceURL: r.SourceUrl,
	}
}

func draftOfInput(in api.RecipeInput) draft {
	d := draft{title: in.Title, totalMinutes: in.TotalMinutes, sourceNoteID: in.SourceNoteId}
	d.summary, d.servings = str(in.Summary), str(in.Servings)
	d.tips, d.sourceURL = str(in.Tips), str(in.SourceUrl)
	if in.Ingredients != nil {
		d.ingredients = *in.Ingredients
	}
	if in.Steps != nil {
		d.steps = *in.Steps
	}
	if in.Tags != nil {
		d.tags = *in.Tags
	}
	return d
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ptr[T any](v T) *T { return &v }

func parseJSON[T any](s string) []T {
	out := []T{}
	if err := json.Unmarshal([]byte(s), &out); err != nil || out == nil {
		return []T{}
	}
	return out
}

func (m *Module) view(row db.Recipe) api.Recipe {
	v := api.Recipe{
		Id: row.ID, Title: row.Title, Summary: row.Summary, Servings: row.Servings,
		Ingredients: parseJSON[api.RecipeIngredient](row.Ingredients), Steps: parseJSON[api.RecipeStep](row.Steps),
		Tips: row.Tips, Tags: parseJSON[string](row.Tags), SourceUrl: row.SourceUrl,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.TotalMinutes != nil {
		v.TotalMinutes = ptr(int(*row.TotalMinutes))
	}
	v.SourceNoteId = row.SourceNoteID
	return v
}

// ---------- validation ----------

func cut(label, v string, max int) (string, error) {
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) > max {
		return "", httpx.Invalid(label + "太长了，最多 " + strconv.Itoa(max) + " 字")
	}
	return v, nil
}

// cleanTags trims, drops empty ones and repeats (ignoring case).
func cleanTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		key := strings.ToLower(t)
		if t == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	return out
}

func validTimerSeconds(label string, t api.RecipeTimer) error {
	if t.Seconds < 1 || t.Seconds > maxTimerSeconds {
		return httpx.Invalid(label + "的时长要在 1 到 86400 秒之间")
	}
	if t.MaxSeconds != nil && (*t.MaxSeconds <= t.Seconds || *t.MaxSeconds > maxTimerSeconds) {
		return httpx.Invalid(label + "的时长上限要大于时长，且不超过 86400 秒")
	}
	return nil
}

// validate checks every field and returns them cleaned. The step indexes are
// checked against the ingredients in the same draft.
func validate(d draft) (draft, error) {
	var err error
	if d.title, err = cut("菜名", d.title, maxTitle); err != nil {
		return d, err
	}
	if d.title == "" {
		return d, httpx.Invalid("菜名不能为空")
	}
	if d.summary, err = cut("简介", d.summary, maxSummary); err != nil {
		return d, err
	}
	if d.servings, err = cut("份量", d.servings, maxServings); err != nil {
		return d, err
	}
	if d.totalMinutes != nil && (*d.totalMinutes < 1 || *d.totalMinutes > maxTotalMinutes) {
		return d, httpx.Invalid("大概用时要在 1 到 10080 分钟之间")
	}
	if d.tips, err = cut("备注", d.tips, maxTips); err != nil {
		return d, err
	}
	if d.sourceNoteID != nil && *d.sourceNoteID < 1 {
		return d, httpx.Invalid("来源笔记不正确")
	}
	if d.sourceURL, err = cut("来源地址", d.sourceURL, maxURL); err != nil {
		return d, err
	}
	if d.sourceURL != "" {
		u, err := url.Parse(d.sourceURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return d, httpx.Invalid("来源地址要以 http:// 或 https:// 开头")
		}
	}
	d.tags = cleanTags(d.tags)
	if len(d.tags) > maxTags {
		return d, httpx.Invalid("标签最多 20 个")
	}
	for _, t := range d.tags {
		if utf8.RuneCountInString(t) > maxTagLen {
			return d, httpx.Invalid("标签“" + t + "”太长了，最多 20 字")
		}
	}
	if len(d.ingredients) > maxIngredients {
		return d, httpx.Invalid("食材最多 100 个")
	}
	ings := make([]api.RecipeIngredient, len(d.ingredients))
	for i, in := range d.ingredients {
		n := strconv.Itoa(i + 1)
		var out api.RecipeIngredient
		if out.Name, err = cut("第 "+n+" 个食材的名称", in.Name, maxIngredientName); err != nil {
			return d, err
		}
		if out.Name == "" {
			return d, httpx.Invalid("第 " + n + " 个食材的名称不能为空")
		}
		amount, err := cut("第 "+n+" 个食材的用量", str(in.Amount), maxAmount)
		if err != nil {
			return d, err
		}
		note, err := cut("第 "+n+" 个食材的处理方法", str(in.Note), maxIngredientNote)
		if err != nil {
			return d, err
		}
		out.Amount, out.Note = &amount, &note
		ings[i] = out
	}
	d.ingredients = ings
	if len(d.steps) > maxSteps {
		return d, httpx.Invalid("步骤最多 100 步")
	}
	steps := make([]api.RecipeStep, len(d.steps))
	for i, s := range d.steps {
		n := strconv.Itoa(i + 1)
		var out api.RecipeStep
		if out.Text, err = cut("第 "+n+" 步的正文", s.Text, maxStepText); err != nil {
			return d, err
		}
		if out.Text == "" {
			return d, httpx.Invalid("第 " + n + " 步的正文不能为空")
		}
		title, err := cut("第 "+n+" 步的小标题", str(s.Title), maxStepTitle)
		if err != nil {
			return d, err
		}
		out.Title = &title
		timers := []api.RecipeTimer{}
		if s.Timers != nil {
			timers = *s.Timers
		}
		if len(timers) > maxStepTimers {
			return d, httpx.Invalid("第 " + n + " 步的计时最多 5 个")
		}
		outTimers := make([]api.RecipeTimer, len(timers))
		for j, t := range timers {
			where := "第 " + n + " 步第 " + strconv.Itoa(j+1) + " 个计时"
			var ot api.RecipeTimer
			if ot.Label, err = cut(where+"的名字", t.Label, maxTimerLabel); err != nil {
				return d, err
			}
			if ot.Label == "" {
				return d, httpx.Invalid(where + "的名字不能为空")
			}
			if err := validTimerSeconds(where, t); err != nil {
				return d, err
			}
			ot.Seconds, ot.MaxSeconds = t.Seconds, t.MaxSeconds
			outTimers[j] = ot
		}
		out.Timers = &outTimers
		idx := []int{}
		if s.Ingredients != nil {
			idx = *s.Ingredients
		}
		seen := map[int]bool{}
		for _, k := range idx {
			if k < 0 || k >= len(ings) {
				return d, httpx.Invalid("第 " + n + " 步用到的食材超出了食材清单")
			}
			if seen[k] {
				return d, httpx.Invalid("第 " + n + " 步用到的食材重复了")
			}
			seen[k] = true
		}
		out.Ingredients = ptr(append([]int{}, idx...))
		steps[i] = out
	}
	d.steps = steps
	return d, nil
}

// ---------- list ----------

func (m *Module) list(ctx context.Context, q, tag string) ([]api.Recipe, []api.RecipeTagCount, error) {
	rows, err := m.q.ListRecipes(ctx)
	if err != nil {
		return nil, nil, err
	}
	q, tag = strings.ToLower(strings.TrimSpace(q)), strings.ToLower(strings.TrimSpace(tag))
	items := []api.Recipe{}
	counts := map[string]int{}
	names := map[string]string{}
	for _, row := range rows {
		v := m.view(row)
		for _, t := range v.Tags {
			k := strings.ToLower(t)
			counts[k]++
			if _, ok := names[k]; !ok {
				names[k] = t
			}
		}
		if tag != "" && !hasTag(v.Tags, tag) {
			continue
		}
		if q != "" && !matches(v, q) {
			continue
		}
		items = append(items, v)
	}
	tags := make([]api.RecipeTagCount, 0, len(counts))
	for k, n := range counts {
		tags = append(tags, api.RecipeTagCount{Tag: names[k], Count: n})
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Count != tags[j].Count {
			return tags[i].Count > tags[j].Count
		}
		return tags[i].Tag < tags[j].Tag
	})
	return items, tags, nil
}

func hasTag(tags []string, lower string) bool {
	for _, t := range tags {
		if strings.ToLower(t) == lower {
			return true
		}
	}
	return false
}

// matches looks for q (lower case) in the title, summary, ingredient names and tags.
func matches(v api.Recipe, q string) bool {
	var b strings.Builder
	b.WriteString(v.Title + "\n" + v.Summary)
	for _, in := range v.Ingredients {
		b.WriteString("\n" + in.Name)
	}
	for _, t := range v.Tags {
		b.WriteString("\n" + t)
	}
	return strings.Contains(strings.ToLower(b.String()), q)
}

// ListRecipes implements api.ServerInterface.
func (m *Module) ListRecipes(w http.ResponseWriter, r *http.Request, params api.ListRecipesParams) {
	items, tags, err := m.list(r.Context(), str(params.Q), str(params.Tag))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "tags": tags})
}

func (m *Module) get(ctx context.Context, id int64) (db.Recipe, error) {
	row, err := m.q.GetRecipe(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, httpx.ErrNotFound
	}
	return row, err
}

// GetRecipe implements api.ServerInterface.
func (m *Module) GetRecipe(w http.ResponseWriter, r *http.Request, id api.RecipeId) {
	row, err := m.get(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m.view(row))
}

// ---------- create, update, delete ----------

// CreateRecipe implements api.ServerInterface.
func (m *Module) CreateRecipe(w http.ResponseWriter, r *http.Request) {
	var body api.RecipeInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	v, err := m.create(ctx, body)
	m.auditRecipe(ctx, "recipe.create", v.Id, body.Title, "", err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, v)
}

// create saves a recipe and publishes the event.
func (m *Module) create(ctx context.Context, in api.RecipeInput) (api.Recipe, error) {
	d, err := validate(draftOfInput(in))
	if err != nil {
		return api.Recipe{}, err
	}
	ings, _ := json.Marshal(d.ingredients)
	steps, _ := json.Marshal(d.steps)
	tags, _ := json.Marshal(d.tags)
	now := m.now().UTC()
	row, err := m.q.InsertRecipe(ctx, db.InsertRecipeParams{
		Title: d.title, Summary: d.summary, Servings: d.servings, TotalMinutes: intPtr(d.totalMinutes),
		Ingredients: string(ings), Steps: string(steps), Tips: d.tips, Tags: string(tags),
		SourceNoteID: d.sourceNoteID, SourceUrl: d.sourceURL, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return api.Recipe{}, err
	}
	v := m.view(row)
	m.d.Bus.Publish("recipe.created", v)
	return v, nil
}

func intPtr(p *int) *int64 {
	if p == nil {
		return nil
	}
	return ptr(int64(*p))
}

func (m *Module) auditRecipe(ctx context.Context, action string, id int64, title, via string, err error) {
	target := ""
	if id != 0 {
		target = strconv.FormatInt(id, 10)
	}
	detail := map[string]any{"title": title}
	if via != "" {
		detail["via"] = via
	}
	m.d.Audit.Record(ctx, action, target, detail, err)
}

// readPatch decodes a PATCH body. totalMinutes and sourceNoteId may be null
// to clear them, which the generated type cannot tell from "not given", so
// the keys are looked up too.
func readPatch(w http.ResponseWriter, r *http.Request) (p api.RecipePatch, clearMinutes, clearNote bool, err error) {
	raw, rerr := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if rerr != nil {
		return p, false, false, httpx.Invalid("请求体格式不正确")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if derr := dec.Decode(&p); derr != nil {
		return p, false, false, httpx.Invalid("请求体格式不正确: " + derr.Error())
	}
	keys := map[string]json.RawMessage{}
	_ = json.Unmarshal(raw, &keys)
	isNull := func(k string) bool { v, ok := keys[k]; return ok && strings.TrimSpace(string(v)) == "null" }
	return p, isNull("totalMinutes"), isNull("sourceNoteId"), nil
}

// UpdateRecipe implements api.ServerInterface.
func (m *Module) UpdateRecipe(w http.ResponseWriter, r *http.Request, id api.RecipeId) {
	p, clearMinutes, clearNote, err := readPatch(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	v, err := m.update(ctx, id, p, clearMinutes, clearNote)
	m.auditRecipe(ctx, "recipe.update", id, v.Title, "", err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) update(ctx context.Context, id int64, p api.RecipePatch, clearMinutes, clearNote bool) (api.Recipe, error) {
	cur, err := m.get(ctx, id)
	if err != nil {
		return api.Recipe{}, err
	}
	d := draftOf(m.view(cur))
	if p.Title != nil {
		d.title = *p.Title
	}
	if p.Summary != nil {
		d.summary = *p.Summary
	}
	if p.Servings != nil {
		d.servings = *p.Servings
	}
	switch {
	case p.TotalMinutes != nil:
		d.totalMinutes = p.TotalMinutes
	case clearMinutes:
		d.totalMinutes = nil
	}
	if p.Ingredients != nil {
		d.ingredients = *p.Ingredients
	}
	if p.Steps != nil {
		d.steps = *p.Steps
	}
	if p.Tips != nil {
		d.tips = *p.Tips
	}
	if p.Tags != nil {
		d.tags = *p.Tags
	}
	switch {
	case p.SourceNoteId != nil:
		d.sourceNoteID = p.SourceNoteId
	case clearNote:
		d.sourceNoteID = nil
	}
	if p.SourceUrl != nil {
		d.sourceURL = *p.SourceUrl
	}
	d, err = validate(d)
	if err != nil {
		return api.Recipe{Title: cur.Title}, err
	}
	ings, _ := json.Marshal(d.ingredients)
	steps, _ := json.Marshal(d.steps)
	tags, _ := json.Marshal(d.tags)
	row, err := m.q.UpdateRecipe(ctx, db.UpdateRecipeParams{
		Title: d.title, Summary: d.summary, Servings: d.servings, TotalMinutes: intPtr(d.totalMinutes),
		Ingredients: string(ings), Steps: string(steps), Tips: d.tips, Tags: string(tags),
		SourceNoteID: d.sourceNoteID, SourceUrl: d.sourceURL, UpdatedAt: m.now().UTC(), ID: id,
	})
	if err != nil {
		return api.Recipe{}, err
	}
	v := m.view(row)
	m.d.Bus.Publish("recipe.updated", v)
	return v, nil
}

// DeleteRecipe implements api.ServerInterface. The timers of the recipe stay;
// the database clears their recipe_id.
func (m *Module) DeleteRecipe(w http.ResponseWriter, r *http.Request, id api.RecipeId) {
	ctx := r.Context()
	n, err := m.q.DeleteRecipe(ctx, id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.auditRecipe(ctx, "recipe.delete", id, "", "", err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("recipe.deleted", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}
