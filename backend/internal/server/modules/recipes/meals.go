package recipes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/db"
)

const (
	maxMealTitle = 100
	maxMealNote  = 1000
	// defaultMealDays is how many days GET /meals shows without from and to.
	defaultMealDays = 30
	// maxMealDays is the longest range GET /meals gives.
	maxMealDays = 366
	// habitSource is the source name of the check-ins made for cooked meals.
	habitSource = "recipes"
)

// ---------- dates ----------

// civil returns the calendar day of t in loc as UTC midnight, so that days
// can be added and compared without zone shifts.
func civil(t time.Time, loc *time.Location) time.Time {
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

// loc is the server time zone, the same one the scheduler and the other
// modules count days in.
func (m *Module) loc() *time.Location {
	if l := m.d.Scheduler.Location(); l != nil {
		return l
	}
	return time.Local
}

func (m *Module) today() time.Time { return civil(m.now(), m.loc()) }

func dayKey(t time.Time) string { return t.Format(time.DateOnly) }

// parseDay reads a YYYY-MM-DD date. label names the field in the error.
func parseDay(label, s string) (time.Time, error) {
	t, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, httpx.Invalid(label + "的格式不对，要写成 YYYY-MM-DD")
	}
	return t, nil
}

// ---------- statistics of the recipes ----------

// mealStat is what the meals say about one recipe.
type mealStat struct {
	cookedCount  int
	lastCookedOn string
	ratingSum    int
	ratingCount  int
}

// avgRating is the mean of the given ratings with one decimal, nil without any.
func (s mealStat) avgRating() *float32 {
	if s.ratingCount == 0 {
		return nil
	}
	return ptr(float32(math.Round(float64(s.ratingSum)/float64(s.ratingCount)*10) / 10))
}

// mealStats reads the statistics of every recipe in one query.
func (m *Module) mealStats(ctx context.Context) (map[int64]mealStat, error) {
	rows, err := m.q.RecipeMealStats(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]mealStat, len(rows))
	for _, r := range rows {
		if r.RecipeID == nil {
			continue
		}
		out[*r.RecipeID] = mealStat{
			cookedCount: int(r.CookedCount), lastCookedOn: r.LastCookedOn,
			ratingSum: int(r.RatingSum), ratingCount: int(r.RatingCount),
		}
	}
	return out, nil
}

// ---------- stored form ----------

type mealDraft struct {
	date     string
	slot     string
	recipeID *int64
	title    string
	cooked   bool
	rating   *int
	note     string
}

func mealView(row db.Meal) api.Meal {
	v := api.Meal{
		Id: row.ID, Date: row.Date, Slot: api.MealSlot(row.Slot), RecipeId: row.RecipeID,
		Title: row.Title, Cooked: row.Cooked, Note: row.Note, CreatedAt: row.CreatedAt,
	}
	if row.Rating != nil {
		v.Rating = ptr(int(*row.Rating))
	}
	return v
}

func draftOfMeal(v api.Meal) mealDraft {
	return mealDraft{
		date: v.Date, slot: string(v.Slot), recipeID: v.RecipeId, title: v.Title,
		cooked: v.Cooked, rating: v.Rating, note: v.Note,
	}
}

func int64Ptr(p *int) *int64 { return intPtr(p) }

// validateMeal checks every field. An empty title becomes the title of the
// recipe; without a recipe it is an error.
func (m *Module) validateMeal(ctx context.Context, d mealDraft) (mealDraft, error) {
	var err error
	if strings.TrimSpace(d.date) == "" {
		d.date = dayKey(m.today())
	} else {
		day, err := parseDay("日期", d.date)
		if err != nil {
			return d, err
		}
		if day.After(m.today()) {
			return d, httpx.Invalid("不能记以后的日子")
		}
		d.date = dayKey(day)
	}
	if !api.MealSlot(d.slot).Valid() {
		return d, httpx.Invalid("餐次要选早饭、午饭、晚饭或加餐")
	}
	if d.rating != nil && (*d.rating < 1 || *d.rating > 5) {
		return d, httpx.Invalid("打分要在 1 到 5 之间")
	}
	if d.title, err = cut("名称", d.title, maxMealTitle); err != nil {
		return d, err
	}
	if d.note, err = cut("备注", d.note, maxMealNote); err != nil {
		return d, err
	}
	if d.recipeID != nil {
		row, err := m.q.GetRecipe(ctx, *d.recipeID)
		if errors.Is(err, sql.ErrNoRows) {
			return d, httpx.Invalid("选的菜谱不存在")
		}
		if err != nil {
			return d, err
		}
		if d.title == "" {
			d.title = row.Title
		}
	}
	if d.title == "" {
		return d, httpx.Invalid("名称不能为空")
	}
	return d, nil
}

// ---------- list ----------

type mealList struct {
	Items   []api.Meal      `json:"items"`
	Weights []api.DayWeight `json:"weights"`
}

// mealRange reads from and to; without them the last 30 days up to today.
func (m *Module) mealRange(from, to *string) (time.Time, time.Time, error) {
	today := m.today()
	var start, end time.Time
	var err error
	if to != nil && strings.TrimSpace(*to) != "" {
		if end, err = parseDay("结束日期", *to); err != nil {
			return start, end, err
		}
	} else {
		end = today
	}
	if from != nil && strings.TrimSpace(*from) != "" {
		if start, err = parseDay("开始日期", *from); err != nil {
			return start, end, err
		}
	} else {
		start = end.AddDate(0, 0, -(defaultMealDays - 1))
	}
	if start.After(end) {
		return start, end, httpx.Invalid("开始日期不能晚于结束日期")
	}
	if int(end.Sub(start).Hours()/24)+1 > maxMealDays {
		return start, end, httpx.Invalid("一次最多看 366 天")
	}
	return start, end, nil
}

func (m *Module) listMeals(ctx context.Context, start, end time.Time) (mealList, error) {
	rows, err := m.q.ListMeals(ctx, db.ListMealsParams{Date: dayKey(start), Date_2: dayKey(end)})
	if err != nil {
		return mealList{}, err
	}
	out := mealList{Items: make([]api.Meal, len(rows)), Weights: []api.DayWeight{}}
	for i, r := range rows {
		out.Items[i] = mealView(r)
	}
	for _, p := range m.bodyWeights(ctx, start, end) {
		out.Weights = append(out.Weights, api.DayWeight{Date: p.Date, Weight: float32(p.Value)})
	}
	return out, nil
}

// bodyWeights gives the weight records from start to end (both included). It
// is empty when there is no body data, the habits module is hidden or the
// source fails: weights only decorate the answer.
func (m *Module) bodyWeights(ctx context.Context, start, end time.Time) []contracts.BodyPoint {
	src, ok := module.Lookup[contracts.BodySeries](m.d.Registry, contracts.BodySeriesKey)
	if !ok || m.habitsHidden(ctx) {
		return nil
	}
	// Noon, so a zone difference between the two modules cannot move the day.
	loc := m.loc()
	at := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, loc) }
	pts, err := src.Daily(ctx, "weight", at(start), at(end))
	if err != nil {
		m.d.Log.Warn("recipes: body weight unavailable", "err", err)
		return nil
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].Date < pts[j].Date })
	return pts
}

// ListMeals implements api.ServerInterface.
func (m *Module) ListMeals(w http.ResponseWriter, r *http.Request, params api.ListMealsParams) {
	start, end, err := m.mealRange(params.From, params.To)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.listMeals(r.Context(), start, end)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// ---------- create ----------

func (m *Module) auditMeal(ctx context.Context, action string, id int64, title, via string, err error) {
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

// recipeChanged tells the page that the statistics of a recipe changed.
func (m *Module) recipeChanged(ids ...*int64) {
	seen := map[int64]bool{}
	for _, id := range ids {
		if id == nil || seen[*id] {
			continue
		}
		seen[*id] = true
		m.d.Bus.Publish("recipe.updated", map[string]any{"id": *id})
	}
}

// CreateMeal implements api.ServerInterface.
func (m *Module) CreateMeal(w http.ResponseWriter, r *http.Request) {
	var body api.MealInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	v, err := m.createMeal(ctx, body)
	m.auditMeal(ctx, "meal.create", v.Id, v.Title, "", err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, v)
}

// createMeal saves a meal, publishes the events and checks in the habit.
func (m *Module) createMeal(ctx context.Context, in api.MealInput) (api.Meal, error) {
	d, err := m.validateMeal(ctx, mealDraft{
		date: str(in.Date), slot: string(in.Slot), recipeID: in.RecipeId, title: str(in.Title),
		cooked: in.Cooked != nil && *in.Cooked, rating: in.Rating, note: str(in.Note),
	})
	if err != nil {
		return api.Meal{}, err
	}
	row, err := m.q.InsertMeal(ctx, db.InsertMealParams{
		Date: d.date, Slot: d.slot, RecipeID: d.recipeID, Title: d.title, Cooked: d.cooked,
		Rating: int64Ptr(d.rating), Note: d.note, CreatedAt: m.now().UTC(),
	})
	if err != nil {
		return api.Meal{}, err
	}
	v := mealView(row)
	m.d.Bus.Publish("meal.created", v)
	m.recipeChanged(v.RecipeId)
	if v.Cooked {
		m.checkinHabit(ctx)
	}
	return v, nil
}

func (m *Module) habitsHidden(ctx context.Context) bool {
	h, ok := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
	return ok && h != nil && h.Hidden(ctx, "habits")
}

// checkinHabit checks in the habit chosen in the settings, once. Nothing here
// may fail the meal: a missing module, no habit set or a refused check-in
// (for example a deleted habit) is skipped, the last one with a log line.
func (m *Module) checkinHabit(ctx context.Context) {
	id, err := m.habitID(ctx)
	if err != nil {
		m.d.Log.Warn("recipes: reading the habit setting failed", "err", err)
		return
	}
	if id == nil {
		return
	}
	habits, ok := module.Lookup[contracts.Habits](m.d.Registry, contracts.HabitsKey)
	if !ok || m.habitsHidden(ctx) {
		return
	}
	if err := habits.Checkin(ctx, *id, 1, habitSource); err != nil {
		m.d.Log.Warn("recipes: habit check-in failed", "habit", *id, "err", err)
	}
}

// ---------- update ----------

// readMealPatch decodes a PATCH body. recipeId and rating may be null to clear
// them, which the generated type cannot tell from "not given".
func readMealPatch(w http.ResponseWriter, r *http.Request) (p api.MealPatch, clearRecipe, clearRating bool, err error) {
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
	return p, isNull("recipeId"), isNull("rating"), nil
}

// UpdateMeal implements api.ServerInterface.
func (m *Module) UpdateMeal(w http.ResponseWriter, r *http.Request, id api.MealId) {
	p, clearRecipe, clearRating, err := readMealPatch(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	v, err := m.updateMeal(ctx, id, p, clearRecipe, clearRating)
	m.auditMeal(ctx, "meal.update", id, v.Title, "", err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) getMeal(ctx context.Context, id int64) (db.Meal, error) {
	row, err := m.q.GetMeal(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, httpx.ErrNotFound
	}
	return row, err
}

func (m *Module) updateMeal(ctx context.Context, id int64, p api.MealPatch, clearRecipe, clearRating bool) (api.Meal, error) {
	cur, err := m.getMeal(ctx, id)
	if err != nil {
		return api.Meal{}, err
	}
	old := mealView(cur)
	d := draftOfMeal(old)
	if p.Date != nil {
		d.date = *p.Date
	}
	if p.Slot != nil {
		d.slot = string(*p.Slot)
	}
	switch {
	case p.RecipeId != nil:
		d.recipeID = p.RecipeId
	case clearRecipe:
		d.recipeID = nil
	}
	if p.Title != nil {
		d.title = *p.Title
	}
	if p.Cooked != nil {
		d.cooked = *p.Cooked
	}
	switch {
	case p.Rating != nil:
		d.rating = p.Rating
	case clearRating:
		d.rating = nil
	}
	if p.Note != nil {
		d.note = *p.Note
	}
	d, err = m.validateMeal(ctx, d)
	if err != nil {
		return api.Meal{Title: old.Title}, err
	}
	row, err := m.q.UpdateMeal(ctx, db.UpdateMealParams{
		Date: d.date, Slot: d.slot, RecipeID: d.recipeID, Title: d.title, Cooked: d.cooked,
		Rating: int64Ptr(d.rating), Note: d.note, ID: id,
	})
	if err != nil {
		return api.Meal{}, err
	}
	v := mealView(row)
	m.d.Bus.Publish("meal.updated", v)
	m.recipeChanged(old.RecipeId, v.RecipeId)
	return v, nil
}

// ---------- delete ----------

// DeleteMeal implements api.ServerInterface. The habit check-in stays.
func (m *Module) DeleteMeal(w http.ResponseWriter, r *http.Request, id api.MealId) {
	ctx := r.Context()
	cur, err := m.getMeal(ctx, id)
	if err == nil {
		var n int64
		n, err = m.q.DeleteMeal(ctx, id)
		if err == nil && n == 0 {
			err = httpx.ErrNotFound
		}
	}
	m.auditMeal(ctx, "meal.delete", id, cur.Title, "", err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("meal.deleted", map[string]any{"id": id})
	m.recipeChanged(cur.RecipeID)
	w.WriteHeader(http.StatusNoContent)
}
