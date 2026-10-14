package recipes

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/recipes/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Setting keys (B210).
const (
	keyHabitID  = "recipes.habit_id"
	keyDietGoal = "recipes.diet_goal"
	keyStaples  = "recipes.staples"

	maxStaples   = 50
	maxStapleLen = 20
)

// defaultStaples are the seasonings that never count as missing.
var defaultStaples = []string{"盐", "糖", "生抽", "老抽", "料酒", "醋", "食用油", "花生油", "淀粉", "胡椒粉", "鸡精", "味精", "水", "开水"}

// getSetting reads a setting; ok is false when it was never set.
func (m *Module) getSetting(ctx context.Context, key string, v any) (bool, error) {
	err := m.d.Settings.Get(ctx, key, v)
	if errors.Is(err, settings.ErrNotSet) {
		return false, nil
	}
	return err == nil, err
}

// habitID is the habit to check in, nil when none is chosen.
func (m *Module) habitID(ctx context.Context) (*int64, error) {
	var id int64
	ok, err := m.getSetting(ctx, keyHabitID, &id)
	if err != nil || !ok || id <= 0 {
		return nil, err
	}
	return &id, nil
}

func (m *Module) loadSettings(ctx context.Context) (api.RecipeSettings, error) {
	out := api.RecipeSettings{DietGoal: api.DietGoalNone, Staples: append([]string{}, defaultStaples...)}
	var err error
	if out.HabitId, err = m.habitID(ctx); err != nil {
		return out, err
	}
	var goal string
	if ok, err := m.getSetting(ctx, keyDietGoal, &goal); err != nil {
		return out, err
	} else if ok && api.DietGoal(goal).Valid() {
		out.DietGoal = api.DietGoal(goal)
	}
	var staples []string
	if ok, err := m.getSetting(ctx, keyStaples, &staples); err != nil {
		return out, err
	} else if ok {
		out.Staples = cleanTags(staples)
	}
	return out, nil
}

// GetRecipeSettings implements api.ServerInterface.
func (m *Module) GetRecipeSettings(w http.ResponseWriter, r *http.Request) {
	out, err := m.loadSettings(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// readSettingsInput decodes a PUT body. habitId may be null to clear it,
// which the generated type cannot tell from "not given".
func readSettingsInput(w http.ResponseWriter, r *http.Request) (p api.RecipeSettingsInput, clearHabit bool, err error) {
	raw, rerr := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if rerr != nil {
		return p, false, httpx.Invalid("请求体格式不正确")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if derr := dec.Decode(&p); derr != nil {
		return p, false, httpx.Invalid("请求体格式不正确: " + derr.Error())
	}
	keys := map[string]json.RawMessage{}
	_ = json.Unmarshal(raw, &keys)
	v, ok := keys["habitId"]
	return p, ok && strings.TrimSpace(string(v)) == "null", nil
}

// PutRecipeSettings implements api.ServerInterface.
func (m *Module) PutRecipeSettings(w http.ResponseWriter, r *http.Request) {
	in, clearHabit, err := readSettingsInput(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	err = m.saveSettings(ctx, in, clearHabit)
	m.d.Audit.Record(ctx, "recipe.settings", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.loadSettings(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// saveSettings checks everything first, then writes, so a refused body
// changes nothing.
func (m *Module) saveSettings(ctx context.Context, in api.RecipeSettingsInput, clearHabit bool) error {
	if in.HabitId != nil && *in.HabitId < 1 {
		return httpx.Invalid("习惯不正确")
	}
	if in.DietGoal != nil && !in.DietGoal.Valid() {
		return httpx.Invalid("饮食目标要选不设、减重、增肌或保持")
	}
	var staples []string
	if in.Staples != nil {
		staples = cleanTags(*in.Staples)
		if len(staples) > maxStaples {
			return httpx.Invalid("常备调料最多 50 个")
		}
		for _, s := range staples {
			if utf8.RuneCountInString(s) > maxStapleLen {
				return httpx.Invalid("常备调料“" + s + "”太长了，最多 20 字")
			}
		}
	}
	switch {
	case in.HabitId != nil:
		if err := m.d.Settings.Set(ctx, keyHabitID, *in.HabitId); err != nil {
			return err
		}
	case clearHabit:
		if err := m.d.Settings.Delete(ctx, keyHabitID); err != nil {
			return err
		}
	}
	if in.DietGoal != nil {
		if err := m.d.Settings.Set(ctx, keyDietGoal, string(*in.DietGoal)); err != nil {
			return err
		}
	}
	if in.Staples != nil {
		if err := m.d.Settings.Set(ctx, keyStaples, staples); err != nil {
			return err
		}
	}
	return nil
}
