package focusblock_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestAIFocusBlockActions(t *testing.T) {
	env := testutil.New(t)
	pc := env.Agent("desktop", []string{protocol.CapFocusBlock}, nil)
	server := env.Agent("server", []string{protocol.CapFocusBlock}, nil)
	if err := env.App.Deps.Settings.Set(context.Background(), "focusblock.settings", map[string]any{"enabled": false, "domains": []string{"a.example", "b.example"}, "apps": []string{}, "haStart": "scene.secret"}); err != nil {
		t.Fatal(err)
	}
	status := callAIAction(t, env, "focusblock.status", `{}`).(map[string]any)
	if status["blocking"] != false {
		t.Fatalf("状态: %+v", status)
	}
	settings := callAIAction(t, env, "focusblock.settings", `{"limit":1}`).(map[string]any)
	if len(settings["domains"].([]any)) != 1 || settings["domainsTruncated"] != true {
		t.Fatalf("设置限量: %+v", settings)
	}
	module.Provide[contracts.HiddenModules](env.App.Deps.Registry, contracts.HiddenModulesKey, aiHiddenModules{"home": true})
	settings = callAIAction(t, env, "focusblock.settings", `{}`).(map[string]any)
	if settings["haStart"] != "" {
		t.Fatalf("隐藏场景泄漏: %+v", settings)
	}
	module.Provide[contracts.HiddenModules](env.App.Deps.Registry, contracts.HiddenModulesKey, aiHiddenModules{"pc": true})
	settings = callAIAction(t, env, "focusblock.settings", `{}`).(map[string]any)
	computers := settings["computers"].([]any)
	if len(computers) != 1 || computers[0].(map[string]any)["id"] != server {
		t.Fatalf("隐藏电脑泄漏: %+v", computers)
	}
	if err := env.App.Deps.Settings.Set(context.Background(), "focusblock.settings", map[string]any{"enabled": false, "hostIds": []string{pc, server}}); err != nil {
		t.Fatal(err)
	}
	settings = callAIAction(t, env, "focusblock.settings", `{}`).(map[string]any)
	ids := settings["hostIds"].([]any)
	if len(ids) != 1 || ids[0] != server {
		t.Fatalf("隐藏电脑编号泄漏: %+v", ids)
	}
	callAIAction(t, env, "focusblock.status", `{}`)
	checkAIActionInputs(t, env, "focusblock.status", "focusblock.settings")
}

func callAIAction(t *testing.T, env *testutil.Env, name, input string) any {
	t.Helper()
	until := time.Now().Add(time.Hour)
	ctx := auth.WithSession(context.Background(), &auth.Session{Username: "jo", VaultUntil: &until})
	result, err := env.App.Deps.Actions.Run(ctx, name, json.RawMessage(input))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func checkAIActionInputs(t *testing.T, env *testutil.Env, names ...string) {
	t.Helper()
	until := time.Now().Add(time.Hour)
	ctx := auth.WithSession(context.Background(), &auth.Session{VaultUntil: &until})
	for _, name := range names {
		a, ok := env.App.Deps.Actions.Get(ctx, name)
		if !ok {
			t.Fatalf("操作未注册: %s", name)
		}
		wantEffect := actions.Read
		if name == "backup.export" {
			wantEffect = actions.Dangerous
		}
		if a.Effect != wantEffect {
			t.Fatalf("%s 的 Effect 不正确: %s", name, a.Effect)
		}
		for _, raw := range []string{`{"unexpected":true}`, `null`, `{} {}`, `{"limit":51}`, `{"limit":0}`, `{"limit":-1}`} {
			_, err := a.Run(ctx, json.RawMessage(raw))
			var failure *httpx.Error
			if !errors.As(err, &failure) || failure.Status != 400 {
				t.Fatalf("%s 无效参数未返回 400: %s %v", name, raw, err)
			}
		}
	}
}

type aiHiddenModules map[string]bool

func (h aiHiddenModules) Hidden(ctx context.Context, name string) bool {
	return h[name] && !auth.VaultUnlocked(ctx)
}

func checkAIHiddenModule(t *testing.T, env *testutil.Env, side string, names ...string) {
	t.Helper()
	module.Provide[contracts.HiddenModules](env.App.Deps.Registry, contracts.HiddenModulesKey, aiHiddenModules{side: true})
	until := time.Now().Add(time.Hour)
	ctx := auth.WithSession(context.Background(), &auth.Session{VaultUntil: &until})
	for _, name := range names {
		a, ok := env.App.Deps.Actions.Get(ctx, name)
		if !ok {
			t.Fatalf("找不到操作 %s", name)
		}
		raw := json.RawMessage(`{}`)
		var spec struct {
			Required []string `json:"required"`
		}
		_ = json.Unmarshal(a.Input, &spec)
		fields := map[string]any{}
		for _, key := range spec.Required {
			if key == "q" {
				fields[key] = "测试"
			} else {
				fields[key] = 1
			}
		}
		if len(fields) > 0 {
			raw, _ = json.Marshal(fields)
		}
		_, err := a.Run(ctx, raw)
		var failure *httpx.Error
		if !errors.As(err, &failure) || failure.Status != 404 {
			t.Fatalf("%s 隐藏内容未被阻止: %v", name, err)
		}
	}
}
