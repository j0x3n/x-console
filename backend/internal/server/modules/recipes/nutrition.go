package recipes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/db"
)

// maxFillRecipes is how many recipes one fill call sends to the model.
const maxFillRecipes = 20

// maxEstimateText caps the text sent to the model for one recipe, in runes.
const maxEstimateText = 6000

const nutritionSchema = `{"type":"object","properties":{
"nutrition":{"type":"array","items":{"type":"string","enum":["high_protein","veg_rich","low_fat","oily","high_carb","sugary"]}}},
"required":["nutrition"],"additionalProperties":false}`

const nutritionSystem = `你是营养分类员。按菜谱的食材、用量和做法，估这道菜属于哪几个营养分类，只返回符合 JSON Schema 的 JSON。

只能从下表选，可以多选，至少选一个最接近的：
- high_protein：主料是肉、蛋、豆制品、鱼虾
- veg_rich：蔬菜占用量的一半以上
- low_fat：用油少，不油炸，不用肥肉
- oily：用油多、油炸、肥肉多
- high_carb：米、面、土豆这类主食占比大
- sugary：糖、蜂蜜用得多

这只是粗分类，不用算克数和热量。只按菜谱里写的内容判断，不要编造。`

// aiNutrition is the answer of the model for one recipe.
type aiNutrition struct {
	Nutrition []string `json:"nutrition"`
}

// estimateText is what the model reads about a recipe.
func estimateText(row db.Recipe) string {
	var b strings.Builder
	b.WriteString("菜名：" + row.Title + "\n")
	if row.Summary != "" {
		b.WriteString("简介：" + row.Summary + "\n")
	}
	if row.Servings != "" {
		b.WriteString("份量：" + row.Servings + "\n")
	}
	b.WriteString("食材：\n")
	for _, in := range parseJSON[api.RecipeIngredient](row.Ingredients) {
		b.WriteString("- " + in.Name)
		if a := str(in.Amount); a != "" {
			b.WriteString(" " + a)
		}
		if n := str(in.Note); n != "" {
			b.WriteString("（" + n + "）")
		}
		b.WriteString("\n")
	}
	b.WriteString("做法：\n")
	for i, s := range parseJSON[api.RecipeStep](row.Steps) {
		b.WriteString(strconv.Itoa(i+1) + ". " + s.Text + "\n")
	}
	return clip(b.String(), maxEstimateText)
}

// estimate asks the model for the nutrition classes of one recipe. The
// purpose is "fast": this is a coarse classification, a cheap model is enough.
func (m *Module) estimate(ctx context.Context, client contracts.LLM, row db.Recipe) ([]string, error) {
	var answer aiNutrition
	ctx = contracts.WithAIUsage(ctx, "recipes", strconv.FormatInt(row.ID, 10))
	err := client.CompleteJSON(ctx, "fast", nutritionSystem, estimateText(row), json.RawMessage(nutritionSchema), &answer)
	if err != nil {
		return nil, httpx.NewError(http.StatusBadGateway, "ai_failed", "AI 估算失败："+err.Error())
	}
	got := repairNutrition(answer.Nutrition)
	if len(got) == 0 {
		return nil, httpx.NewError(http.StatusBadGateway, "ai_incomplete", "AI 没有给出营养分类，再试一次")
	}
	return got, nil
}

// EstimateRecipeNutrition implements api.ServerInterface. It saves nothing.
func (m *Module) EstimateRecipeNutrition(w http.ResponseWriter, r *http.Request, id api.RecipeId) {
	ctx := r.Context()
	row, err := m.get(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	client, err := m.llmClient(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	got, err := m.estimate(ctx, client, row)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"nutrition": nutritionValues(got)})
}

// FillRecipeNutrition implements api.ServerInterface. It saves the classes of
// up to 20 recipes that have none. A recipe the model fails on is counted and
// skipped, the others go on.
func (m *Module) FillRecipeNutrition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	client, err := m.llmClient(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Two fills at once would send the same recipes to the model twice.
	m.fillMu.Lock()
	defer m.fillMu.Unlock()
	rows, err := m.q.ListRecipesWithoutNutrition(ctx, maxFillRecipes)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	filled, failed := 0, 0
	for _, row := range rows {
		if ctx.Err() != nil {
			break
		}
		got, err := m.estimate(ctx, client, row)
		if err == nil {
			raw, _ := json.Marshal(got)
			err = m.q.UpdateRecipeNutrition(ctx, db.UpdateRecipeNutritionParams{Nutrition: string(raw), ID: row.ID})
		}
		if err != nil {
			m.d.Log.Warn("recipes: nutrition fill failed", "recipe", row.ID, "err", err)
			failed++
			continue
		}
		filled++
		// The event carries the whole recipe, like any other update.
		if cur, err := m.get(ctx, row.ID); err == nil {
			if v, err := m.viewOne(ctx, cur); err == nil {
				m.d.Bus.Publish("recipe.updated", v)
			}
		}
	}
	remaining, err := m.q.CountRecipesWithoutNutrition(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(ctx, "recipe.nutrition_fill", "", map[string]any{"filled": filled, "failed": failed}, nil)
	httpx.JSON(w, http.StatusOK, map[string]any{"filled": filled, "failed": failed, "remaining": remaining})
}
