package recipes

import (
	"context"
	"encoding/json"
	"math"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/api"
)

const recipeInputSchema = `{"type":"object","properties":{
"title":{"type":"string"},"summary":{"type":"string"},"servings":{"type":"string"},"totalMinutes":{"type":"integer","minimum":1,"maximum":10080},
"ingredients":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"amount":{"type":"string"},"note":{"type":"string"}},"required":["name"],"additionalProperties":false}},
"steps":{"type":"array","items":{"type":"object","properties":{"title":{"type":"string"},"text":{"type":"string"},
"timers":{"type":"array","items":{"type":"object","properties":{"label":{"type":"string"},"seconds":{"type":"integer"},"maxSeconds":{"type":"integer"}},"required":["label","seconds"],"additionalProperties":false}},
"ingredients":{"type":"array","items":{"type":"integer"}}},"required":["text"],"additionalProperties":false}},
"tips":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}},"sourceUrl":{"type":"string"}},
"required":["title"],"additionalProperties":false}`

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "recipes.search",
		Title: "查找菜谱",
		Description: "Search the user's recipes. `q` matches the title, summary, ingredient names and tags. `tag` keeps only recipes with that tag. " +
			"Each result has id, title, tags, totalMinutes (minutes, null if unknown) and ingredients (names only). Use recipes.get for the whole recipe.",
		Input:  actions.Schema(`{"type":"object","properties":{"q":{"type":"string"},"tag":{"type":"string"}},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct{ Q, Tag string }
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, err
				}
			}
			items, _, err := m.list(ctx, in.Q, in.Tag)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(items))
			for _, r := range items {
				names := make([]string, len(r.Ingredients))
				for i, ing := range r.Ingredients {
					names[i] = ing.Name
				}
				out = append(out, map[string]any{"id": r.Id, "title": r.Title, "tags": r.Tags, "totalMinutes": r.TotalMinutes, "ingredients": names})
			}
			return map[string]any{"items": out}, nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "recipes.get",
		Title: "查看菜谱",
		Description: "Get one whole recipe by `id`: ingredients (name, amount, note), steps (title, text, timers in seconds, " +
			"ingredients as indexes into the ingredient list), tips in Markdown and tags.",
		Input:  actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"],"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct{ ID int64 }
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
			row, err := m.get(ctx, in.ID)
			if err != nil {
				return nil, err
			}
			return m.view(row), nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "recipes.create",
		Title: "新建菜谱",
		Description: "Save a recipe the user gave or asked for. Tidy the text into this structure first. title is required. " +
			"Ingredients have name, amount (like \"1 汤匙\") and note (like \"切段\"). Steps have text, an optional title, timers " +
			"(label, seconds, optional maxSeconds when the time is a range) and ingredients (indexes into the ingredient list, from 0). " +
			"Put other remarks in tips as Markdown. Give 3 to 6 tags such as taste, cuisine and main ingredient.",
		Input:  actions.Schema(recipeInputSchema),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in api.RecipeInput
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
			v, err := m.create(ctx, in)
			m.auditRecipe(ctx, "recipe.create", v.Id, in.Title, "ai", err)
			if err != nil {
				return nil, err
			}
			return v, nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "cook_timers.start",
		Title: "开始做菜计时",
		Description: "Start a cooking timer that notifies the user when it runs out. `label` names it (like \"炖煮\"), " +
			"`minutes` is the length and can have decimals (0.5 is 30 seconds). Optional `recipeId` and `stepIndex` (from 0) link it to a recipe step. At most 20 timers can be open.",
		Input:  actions.Schema(`{"type":"object","properties":{"label":{"type":"string"},"minutes":{"type":"number","exclusiveMinimum":0},"recipeId":{"type":"integer"},"stepIndex":{"type":"integer","minimum":0}},"required":["label","minutes"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Label     string  `json:"label"`
				Minutes   float64 `json:"minutes"`
				RecipeID  *int64  `json:"recipeId"`
				StepIndex *int    `json:"stepIndex"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
			if math.IsNaN(in.Minutes) || in.Minutes <= 0 || in.Minutes > maxTimerSeconds/60 {
				return nil, httpx.Invalid("计时的时长要在 1 秒到 24 小时之间")
			}
			v, err := m.startTimer(ctx, api.CookTimerInput{
				Label: in.Label, Seconds: int(math.Round(in.Minutes * 60)), RecipeId: in.RecipeID, StepIndex: in.StepIndex,
			})
			m.auditTimer(ctx, "cook_timer.create", v.Id, map[string]any{"label": in.Label, "via": "ai"}, err)
			if err != nil {
				return nil, err
			}
			return v, nil
		},
	})
}
