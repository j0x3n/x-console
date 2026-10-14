package recipes

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/api"
)

// Hooks for the integration tests in package recipes_test, which cannot
// live in this package because testutil imports app, which imports recipes.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func CheckTimers(m *Module, ctx context.Context, now time.Time) error { return m.checkTimers(ctx, now) }

// Recommend runs the scoring without a server.
func Recommend(recs []api.Recipe, lastEaten map[int64]string, goal api.DietGoal, change *float64, today time.Time) []api.RecipeRecommendation {
	return recommend(recommendInput{recipes: recs, lastEaten: lastEaten, goal: goal, weightChange: change, today: today})
}

// EffectiveGoal is the goal the scoring uses.
func EffectiveGoal(goal api.DietGoal, change *float64) string { return effectiveGoal(goal, change) }
