package backup_test

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
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestAIBackupActions(t *testing.T) {
	env := testutil.New(t)
	store := files.Local{Root: env.App.Deps.Config.BackupsDir()}
	for _, name := range []string{"first.tar.gz", "second.tar.gz"} {
		if err := store.Put(context.Background(), name, strings.NewReader("测试备份"), -1); err != nil {
			t.Fatal(err)
		}
	}
	page := callAIAction(t, env, "backup.list", `{"limit":1}`).(map[string]any)
	if len(page["items"].([]any)) != 1 || page["truncated"] != true {
		t.Fatalf("备份限量: %+v", page)
	}
	status := callAIAction(t, env, "backup.status", `{}`).(map[string]any)
	if status["state"] != "idle" {
		t.Fatalf("状态: %+v", status)
	}
	action, ok := env.App.Deps.Actions.Get(context.Background(), "backup.export")
	if !ok || action.Effect != actions.Dangerous {
		t.Fatal("备份必须标为高危")
	}
	ctx := auth.WithSession(context.Background(), &auth.Session{Username: "jo"})
	if _, err := action.Run(ctx, json.RawMessage(`{}`)); !errors.Is(err, httpx.ErrElevationRequired) {
		var failure *httpx.Error
		if !errors.As(err, &failure) || failure.Status != 403 {
			t.Fatalf("未验证时应拒绝备份: %v", err)
		}
	}
	until := time.Now().Add(time.Minute)
	ctx = auth.WithSession(context.Background(), &auth.Session{Username: "jo", ElevatedUntil: &until})
	if _, err := action.Run(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		status = callAIAction(t, env, "backup.status", `{}`).(map[string]any)
		if status["state"] != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("备份没有完成")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status["state"] != "done" || status["backupId"] == nil {
		t.Fatalf("备份结果: %+v", status)
	}
	var audits int
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM audit_log WHERE action='backup.export'").Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("备份审计: %d %v", audits, err)
	}
	checkAIActionInputs(t, env, "backup.list", "backup.status", "backup.export")
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
