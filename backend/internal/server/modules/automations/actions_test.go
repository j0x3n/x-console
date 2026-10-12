package automations_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestAIAutomationActions(t *testing.T) {
	env := testutil.New(t)
	token, err := env.App.Deps.Secrets.Seal("secret-hook-token")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		action := "focus.current"
		if i == 2 {
			action = "notes.list"
		}
		steps, _ := json.Marshal([]any{map[string]any{"action": action, "input": map[string]any{"secret": "隐藏正文"}}})
		_, err = env.App.Deps.DB.Exec(`INSERT INTO automations(id,name,enabled,trigger,conditions,actions,webhook_token_enc,created_at,updated_at) VALUES(?,'测试规则',0,'{"type":"webhook"}','[]',?,?,?,?)`, i, string(steps), token, time.Now(), time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 3; i++ {
		_, err = env.App.Deps.DB.Exec(`INSERT INTO automation_runs(id,automation_id,started_at,trigger_data,steps,status) VALUES(?,1,?,'{"secret":"隐藏输入"}','[{"output":"隐藏结果"}]','ok')`, i, time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
	page := callAIAction(t, env, "automations.list", `{"limit":1}`).(map[string]any)
	if len(page["items"].([]any)) != 1 || page["truncated"] != true {
		t.Fatalf("规则限量: %+v", page)
	}
	detail := callAIAction(t, env, "automations.get", `{"id":1}`).(map[string]any)
	raw, _ := json.Marshal(detail)
	if strings.Contains(string(raw), "secret-hook-token") || strings.Contains(string(raw), "隐藏") {
		t.Fatalf("规则秘密泄漏: %s", raw)
	}
	runs := callAIAction(t, env, "automations.runs", `{"id":1,"limit":1}`).(map[string]any)
	if len(runs["items"].([]any)) != 1 || runs["truncated"] != true {
		t.Fatalf("记录限量: %+v", runs)
	}
	run := runs["items"].([]any)[0].(map[string]any)
	if run["triggerData"] != nil || run["steps"] != nil {
		t.Fatalf("原始结果泄漏: %+v", run)
	}
	module.Provide[contracts.HiddenModules](env.App.Deps.Registry, contracts.HiddenModulesKey, aiHiddenModules{"notes": true})
	page = callAIAction(t, env, "automations.list", `{}`).(map[string]any)
	if len(page["items"].([]any)) != 1 {
		t.Fatalf("隐藏规则泄漏: %+v", page)
	}
	for _, name := range []string{"automations.get", "automations.runs"} {
		if _, err := env.App.Deps.Actions.Run(context.Background(), name, json.RawMessage(`{"id":2}`)); err == nil {
			t.Fatalf("%s 返回了隐藏规则", name)
		}
	}
	checkAIActionInputs(t, env, "automations.list", "automations.get", "automations.runs")
	checkAIHiddenModule(t, env, "automations", "automations.list", "automations.get", "automations.runs")
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
