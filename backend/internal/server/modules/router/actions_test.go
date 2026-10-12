package router_test

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
)

func TestAIRouterActions(t *testing.T) {
	env, _, _, routerClock := setup(t)
	status := callAIAction(t, env, "router.status", `{}`).(map[string]any)
	if status["hostname"] == nil {
		t.Fatalf("状态: %+v", status)
	}
	clients := callAIAction(t, env, "router.clients", `{"limit":1}`).(map[string]any)
	if len(clients["items"].([]any)) != 1 || clients["itemsTruncated"] != true {
		t.Fatalf("客户端限量: %+v", clients)
	}
	for i := 1; i <= 3; i++ {
		if _, err := env.App.Deps.DB.Exec("INSERT INTO router_traffic(at,seconds,rx,tx) VALUES(?,60,100,50)", routerClock.t.Add(-time.Duration(i)*time.Hour).Unix()); err != nil {
			t.Fatal(err)
		}
	}
	traffic := callAIAction(t, env, "router.traffic", `{"range":"7d","limit":1}`).(map[string]any)
	if traffic["range"] != "7d" || traffic["pointsTruncated"] != true || len(traffic["points"].([]any)) != 1 || traffic["rxBytes"] != float64(300) {
		t.Fatalf("时间范围: %+v", traffic)
	}
	point := traffic["points"].([]any)[0].(map[string]any)
	if point["at"] != routerClock.t.Add(-time.Hour).Format(time.RFC3339) {
		t.Fatalf("未返回最新采样: %+v", point)
	}
	checkAIActionInputs(t, env, "router.status", "router.clients", "router.traffic")
	if _, err := env.App.Deps.Actions.Run(context.Background(), "router.traffic", json.RawMessage(`{"range":"全部"}`)); err == nil {
		t.Fatal("无效时间范围被接受")
	}
	checkAIHiddenModule(t, env, "router", "router.status", "router.clients", "router.traffic")
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
