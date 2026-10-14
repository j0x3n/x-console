package recipes

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/api"
)

// parseSchema is what the model must return. It matches api.RecipeInput
// without the source fields.
const parseSchema = `{"type":"object","properties":{
"title":{"type":"string"},
"summary":{"type":"string"},
"servings":{"type":"string"},
"totalMinutes":{"type":["integer","null"]},
"ingredients":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"amount":{"type":"string"},"note":{"type":"string"}},"required":["name","amount","note"],"additionalProperties":false}},
"steps":{"type":"array","items":{"type":"object","properties":{
"title":{"type":"string"},
"text":{"type":"string"},
"timers":{"type":"array","items":{"type":"object","properties":{"label":{"type":"string"},"seconds":{"type":"integer"},"maxSeconds":{"type":["integer","null"]}},"required":["label","seconds","maxSeconds"],"additionalProperties":false}},
"ingredients":{"type":"array","items":{"type":"integer"}}},
"required":["title","text","timers","ingredients"],"additionalProperties":false}},
"tips":{"type":"string"},
"tags":{"type":"array","items":{"type":"string"}},
"nutrition":{"type":"array","items":{"type":"string","enum":["high_protein","veg_rich","low_fat","oily","high_carb","sugary"]}}},
"required":["title","summary","servings","totalMinutes","ingredients","steps","tips","tags","nutrition"],"additionalProperties":false}`

const parseSystem = `你是烹饪教程整理员。把用户给的教程原文整理成结构化的菜谱，只返回符合 JSON Schema 的 JSON。

规则：
- 菜名、简介、份量、大概用时（分钟）照原文整理，不要编。原文没写的，字符串留空，用时写 null。
- ingredients 按原文列出全部食材。amount 写用量，比如“1 汤匙”“1500 克”。note 写处理方法，比如“切段”“拍松”。用量和处理方法要分开写。原文没有的留空字符串。
- steps 按原文的顺序，不要合并，不要漏步骤。title 是步骤的小标题，原文没有就留空。text 是这一步的正文。
- 原文的“备注”“要点”里，如果明确说的是某一步的时间，要放进那一步的 timers。比如“小公鸡炖 20 分钟，老公鸡炖 40 分钟以上”，放到“加开水炖”这一步，seconds 是 1200，maxSeconds 是 2400。
- timers 里每项有 label（计时的名字，比如“炖煮”）、seconds（秒）、maxSeconds（时间是范围时的上限，不是范围就写 null）。“一两分钟”这种取小的值做 seconds，大的做 maxSeconds。“半小时”是 1800 秒。“40 分钟以上”只写 seconds。鸡龄、保存期这种不是烹饪计时的时间不要写。
- 每一步的 ingredients 是这一步用到的食材，写 ingredients 数组里的下标，从 0 开始。只写这一步明确要加的，不要重复。
- 原文剩下的说明、要点、来源链接，放进 tips，保留 Markdown。不要丢内容。不要把已经放进步骤的内容再重复写一遍。
- tags 给 3 到 6 个，包括口味、菜系、主料、做法，比如“辣”“鲁菜”“鸡肉”“炖”。每个不超过 20 字。
- nutrition 是营养分类，按食材和用量估，可以多选，不要编造原文没有的食材。只能从下表选：
  - high_protein：主料是肉、蛋、豆制品、鱼虾
  - veg_rich：蔬菜占用量的一半以上
  - low_fat：用油少，不油炸，不用肥肉
  - oily：用油多、油炸、肥肉多
  - high_carb：米、面、土豆这类主食占比大
  - sugary：糖、蜂蜜用得多
  拿不准的不选。
- 用原文的语言。`

// aiRecipe is the answer of the model. Numbers are read as floats because
// models sometimes write 1800.0.
type aiRecipe struct {
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	Servings     string   `json:"servings"`
	TotalMinutes *float64 `json:"totalMinutes"`
	Ingredients  []struct {
		Name   string `json:"name"`
		Amount string `json:"amount"`
		Note   string `json:"note"`
	} `json:"ingredients"`
	Steps []struct {
		Title  string `json:"title"`
		Text   string `json:"text"`
		Timers []struct {
			Label      string   `json:"label"`
			Seconds    float64  `json:"seconds"`
			MaxSeconds *float64 `json:"maxSeconds"`
		} `json:"timers"`
		Ingredients []float64 `json:"ingredients"`
	} `json:"steps"`
	Tips      string   `json:"tips"`
	Tags      []string `json:"tags"`
	Nutrition []string `json:"nutrition"`
}

var (
	errAIUnavailable = httpx.NewError(http.StatusNotImplemented, "feature_unavailable", "还没有配置 AI，请先到设置里配置模型")
	errAIIncomplete  = httpx.NewError(http.StatusBadGateway, "ai_incomplete", "AI 返回的菜谱不完整，再试一次")
)

// llmClient returns the model, or errAIUnavailable when none is set up.
func (m *Module) llmClient(ctx context.Context) (contracts.LLM, error) {
	client, ok := module.Lookup[contracts.LLM](m.d.Registry, contracts.LLMKey)
	if !ok || !client.Available(ctx) {
		return nil, errAIUnavailable
	}
	return client, nil
}

// ParseRecipe implements api.ServerInterface.
func (m *Module) ParseRecipe(w http.ResponseWriter, r *http.Request) {
	var body api.RecipeParseInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.parse(r.Context(), body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) parse(ctx context.Context, in api.RecipeParseInput) (api.RecipeInput, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return api.RecipeInput{}, httpx.Invalid("教程原文是空的")
	}
	if utf8.RuneCountInString(text) > maxParseText {
		return api.RecipeInput{}, httpx.Invalid("教程原文太长，最多 20000 字")
	}
	client, err := m.llmClient(ctx)
	if err != nil {
		return api.RecipeInput{}, err
	}
	var answer aiRecipe
	// The model call is "agent", the main model: the answer cross-references
	// ingredient indexes, which the fast model gets wrong more often.
	err = client.CompleteJSON(contracts.WithAIUsage(ctx, "recipes", ""), "agent", parseSystem, text, json.RawMessage(parseSchema), &answer)
	if err != nil {
		return api.RecipeInput{}, httpx.NewError(http.StatusBadGateway, "ai_failed", "AI 整理失败："+err.Error())
	}
	d, err := validate(repair(answer))
	if err != nil || (len(d.steps) == 0 && len(d.ingredients) == 0) {
		return api.RecipeInput{}, errAIIncomplete
	}
	d.sourceNoteID = in.SourceNoteId
	return inputOf(d), nil
}

func inputOf(d draft) api.RecipeInput {
	in := api.RecipeInput{
		Title: d.title, Summary: &d.summary, Servings: &d.servings, TotalMinutes: d.totalMinutes,
		Ingredients: &d.ingredients, Steps: &d.steps, Tips: &d.tips, Tags: &d.tags, SourceNoteId: d.sourceNoteID,
		SourceUrl: &d.sourceURL,
	}
	nutrition := nutritionValues(d.nutrition)
	in.Nutrition = &nutrition
	return in
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		return strings.TrimSpace(string(r[:max]))
	}
	return s
}

// whole rounds a number from the model to an int, and says if it is usable.
func whole(f float64) (int, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) || f > 1e9 || f < -1e9 {
		return 0, false
	}
	return int(math.Round(f)), true
}

// repair fixes what a model often gets slightly wrong, so that a good answer
// is not thrown away: text that is too long is cut, indexes and timers that
// are out of range are dropped, tags are made unique. validate still decides
// whether the result is usable.
func repair(a aiRecipe) draft {
	d := draft{
		title: clip(a.Title, maxTitle), summary: clip(a.Summary, maxSummary), servings: clip(a.Servings, maxServings),
		tips: clip(a.Tips, maxTips),
	}
	if a.TotalMinutes != nil {
		if n, ok := whole(*a.TotalMinutes); ok && n >= 1 && n <= maxTotalMinutes {
			d.totalMinutes = &n
		}
	}
	// ingredients without a name go away; the indexes of the steps follow
	remap := map[int]int{}
	for i, in := range a.Ingredients {
		name := clip(in.Name, maxIngredientName)
		if name == "" || len(d.ingredients) >= maxIngredients {
			continue
		}
		remap[i] = len(d.ingredients)
		amount, note := clip(in.Amount, maxAmount), clip(in.Note, maxIngredientNote)
		d.ingredients = append(d.ingredients, api.RecipeIngredient{Name: name, Amount: &amount, Note: &note})
	}
	for _, s := range a.Steps {
		text := clip(s.Text, maxStepText)
		if text == "" || len(d.steps) >= maxSteps {
			continue
		}
		title := clip(s.Title, maxStepTitle)
		step := api.RecipeStep{Title: &title, Text: text}
		timers := []api.RecipeTimer{}
		for _, t := range s.Timers {
			sec, ok := whole(t.Seconds)
			if !ok || sec < 1 || sec > maxTimerSeconds || len(timers) >= maxStepTimers {
				continue
			}
			label := clip(t.Label, maxTimerLabel)
			if label == "" {
				label = "计时"
			}
			timer := api.RecipeTimer{Label: label, Seconds: sec}
			if t.MaxSeconds != nil {
				if mx, ok := whole(*t.MaxSeconds); ok && mx > sec && mx <= maxTimerSeconds {
					timer.MaxSeconds = &mx
				}
			}
			timers = append(timers, timer)
		}
		step.Timers = &timers
		idx, seen := []int{}, map[int]bool{}
		for _, f := range s.Ingredients {
			old, ok := whole(f)
			if !ok {
				continue
			}
			k, ok := remap[old]
			if !ok || seen[k] {
				continue
			}
			seen[k] = true
			idx = append(idx, k)
		}
		step.Ingredients = &idx
		d.steps = append(d.steps, step)
	}
	tags := make([]string, 0, len(a.Tags))
	for _, t := range a.Tags {
		tags = append(tags, clip(t, maxTagLen))
	}
	d.tags = cleanTags(tags)
	if len(d.tags) > maxTags {
		d.tags = d.tags[:maxTags]
	}
	d.nutrition = repairNutrition(a.Nutrition)
	return d
}

// repairNutrition drops the values that are not a nutrition class and the repeats.
func repairNutrition(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if !isNutrition(v) || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
