package recipes

import (
	"context"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/api"
)

const (
	// maxRecommend is how many recipes the recommendation gives.
	maxRecommend = 6
	// weightWindowDays is how far back the weight change looks, today included.
	weightWindowDays = 30
	// weightGainKg is the rise in weight that makes a recommendation without a
	// goal lean to losing weight. It is a rule of thumb, not from a study.
	weightGainKg = 1.0
	// recentEatenDays: a recipe eaten within this many days goes to the back.
	recentEatenDays = 7
	// forgottenDays: a recipe cooked before and not eaten for this long gets a bonus.
	forgottenDays = 30
)

// effectiveLoseHalf is the goal when none is set but the weight went up.
const effectiveLoseHalf = "lose_half"

// recommendInput is everything the scoring needs, so tests can call it
// without a server.
type recommendInput struct {
	recipes   []api.Recipe     // with their statistics
	lastEaten map[int64]string // recipe id -> date of the last meal of any kind
	goal      api.DietGoal
	// weightChange is the weight change over the last 30 days in kg, rounded
	// to one decimal. nil when there are fewer than two records.
	weightChange *float64
	today        time.Time // calendar day at UTC midnight
}

// effectiveGoal is the goal the scores are built from.
func effectiveGoal(goal api.DietGoal, weightChange *float64) string {
	if goal == api.DietGoalNone && weightChange != nil && *weightChange >= weightGainKg {
		return effectiveLoseHalf
	}
	return string(goal)
}

type scored struct {
	recipe  api.Recipe
	score   float64
	reasons []string
}

// recommend scores the recipes by the rules in docs/specs/B210.md and returns
// the best six with their reasons. Equal scores keep the recently updated first.
func recommend(in recommendInput) []api.RecipeRecommendation {
	effective := effectiveGoal(in.goal, in.weightChange)
	list := make([]scored, 0, len(in.recipes))
	for _, r := range in.recipes {
		list = append(list, scoreRecipe(in, effective, r))
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if !a.recipe.UpdatedAt.Equal(b.recipe.UpdatedAt) {
			return a.recipe.UpdatedAt.After(b.recipe.UpdatedAt)
		}
		return a.recipe.Id > b.recipe.Id
	})
	if len(list) > maxRecommend {
		list = list[:maxRecommend]
	}
	out := make([]api.RecipeRecommendation, len(list))
	for i, s := range list {
		out[i] = api.RecipeRecommendation{Recipe: s.recipe, Reasons: s.reasons}
	}
	return out
}

func hasNutrition(r api.Recipe, kinds ...string) []string {
	var got []string
	for _, k := range kinds {
		if slices.Contains(r.Nutrition, api.RecipeNutrition(k)) {
			got = append(got, k)
		}
	}
	return got
}

var nutritionNames = map[string]string{"low_fat": "少油", "veg_rich": "蔬菜多"}

func joinNames(kinds []string) string {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = nutritionNames[k]
	}
	return strings.Join(names, "、")
}

// num writes a number with at most one decimal and no trailing zero.
func num(v float64) string {
	return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64)
}

func scoreRecipe(in recommendInput, effective string, r api.Recipe) scored {
	s := scored{recipe: r, reasons: []string{}}
	switch effective {
	case string(api.DietGoalLose), effectiveLoseHalf:
		unit := 1.0
		if effective == effectiveLoseHalf {
			unit = 0.5
		}
		if good := hasNutrition(r, "low_fat", "veg_rich"); len(good) > 0 {
			s.score += 3 * unit
			if effective == effectiveLoseHalf {
				s.reasons = append(s.reasons, "最近 30 天体重涨了 "+strconv.FormatFloat(*in.weightChange, 'f', 1, 64)+" kg")
			} else {
				s.reasons = append(s.reasons, joinNames(good)+"，适合减重")
			}
		}
		if len(hasNutrition(r, "oily", "sugary")) > 0 {
			s.score -= 3 * unit
		}
	case string(api.DietGoalGain):
		if len(hasNutrition(r, "high_protein")) > 0 {
			s.score += 3
			s.reasons = append(s.reasons, "高蛋白，适合增肌")
		}
	case string(api.DietGoalSteady):
		if len(hasNutrition(r, "veg_rich")) > 0 {
			s.score++
			s.reasons = append(s.reasons, "蔬菜多")
		}
	}
	if r.AvgRating != nil && *r.AvgRating >= 4 {
		s.score += 2
		s.reasons = append(s.reasons, "你打过 "+num(float64(*r.AvgRating))+" 分")
	}
	days, eaten := -1, false
	if last := in.lastEaten[r.Id]; last != "" {
		if d, err := time.Parse(time.DateOnly, last); err == nil {
			days, eaten = int(in.today.Sub(d).Hours()/24), true
		}
	}
	if eaten && days < recentEatenDays {
		s.score -= 4
	}
	if r.CookedCount == 0 {
		s.score++
		s.reasons = append(s.reasons, "还没做过")
	} else if eaten && days >= forgottenDays {
		s.score++
		s.reasons = append(s.reasons, "有 "+strconv.Itoa(days)+" 天没吃了")
	}
	return s
}

// weightChange30d is the last weight of the last 30 days minus the first,
// rounded to one decimal. nil with fewer than two records.
func (m *Module) weightChange30d(ctx context.Context) *float64 {
	today := m.today()
	pts := m.bodyWeights(ctx, today.AddDate(0, 0, -(weightWindowDays-1)), today)
	if len(pts) < 2 {
		return nil
	}
	return ptr(math.Round((pts[len(pts)-1].Value-pts[0].Value)*10) / 10)
}

// recommendations builds the answer of GET /recipes/recommend.
func (m *Module) recommendations(ctx context.Context) (api.RecipeRecommendations, error) {
	var out api.RecipeRecommendations
	items, _, err := m.list(ctx, "", "")
	if err != nil {
		return out, err
	}
	set, err := m.loadSettings(ctx)
	if err != nil {
		return out, err
	}
	rows, err := m.q.RecipeLastEaten(ctx)
	if err != nil {
		return out, err
	}
	eaten := make(map[int64]string, len(rows))
	for _, r := range rows {
		if r.RecipeID != nil {
			eaten[*r.RecipeID] = r.LastEatenOn
		}
	}
	change := m.weightChange30d(ctx)
	out.Items = recommend(recommendInput{recipes: items, lastEaten: eaten, goal: set.DietGoal, weightChange: change, today: m.today()})
	out.Context.DietGoal = set.DietGoal
	out.Context.EffectiveGoal = api.RecipeRecommendationsContextEffectiveGoal(effectiveGoal(set.DietGoal, change))
	if change != nil {
		out.Context.WeightChange30d = ptr(float32(*change))
	}
	for _, r := range items {
		if len(r.Nutrition) == 0 {
			out.Context.MissingNutrition++
		}
	}
	return out, nil
}

// RecommendRecipes implements api.ServerInterface.
func (m *Module) RecommendRecipes(w http.ResponseWriter, r *http.Request) {
	out, err := m.recommendations(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
