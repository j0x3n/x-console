package storage_test

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
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestAIStorageActions(t *testing.T) {
	env := testutil.New(t)
	status := callAIAction(t, env, "storage.status", `{}`).(map[string]any)
	if status["backend"] != "local" || status["usage"] != nil || status["s3"] != nil {
		t.Fatalf("状态: %+v", status)
	}
	if err := env.App.Deps.Settings.Set(context.Background(), "storage.migration", map[string]any{"state": "failed", "target": "local"}); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.New(env.App.Deps); err != nil {
		t.Fatal(err)
	}
	status = callAIAction(t, env, "storage.status", `{}`).(map[string]any)
	if status["migration"].(map[string]any)["state"] != "failed" {
		t.Fatalf("重建后仍读取旧实例: %+v", status)
	}
	for i := 1; i <= 3; i++ {
		_, err := env.App.Deps.DB.Exec(`INSERT INTO storage_remotes(id,kind,name,config,show_in_drive,created_at,updated_at) VALUES(?,'webdav','测试网盘','{"url":"https://example.com","username":"secret-name"}',1,?,?)`, i, time.Now(), time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
	remotes := callAIAction(t, env, "storage.remotes", `{"limit":1}`).(map[string]any)
	if len(remotes["items"].([]any)) != 1 || remotes["truncated"] != true {
		t.Fatalf("网盘限量: %+v", remotes)
	}
	remote := remotes["items"].([]any)[0].(map[string]any)
	if remote["ready"] != true || remote["account"] != nil || remote["config"] != nil {
		t.Fatalf("账号信息泄漏: %+v", remote)
	}
	checkAIActionInputs(t, env, "storage.status", "storage.remotes")
	module.Provide[contracts.HiddenModules](env.App.Deps.Registry, contracts.HiddenModulesKey, aiHiddenModules{"drive": true})
	remotes = callAIAction(t, env, "storage.remotes", `{}`).(map[string]any)
	if len(remotes["items"].([]any)) != 0 {
		t.Fatalf("隐藏网盘泄漏: %+v", remotes)
	}
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
