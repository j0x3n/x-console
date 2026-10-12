package maintenance_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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

type aiActionReporter struct{}

func (aiActionReporter) Usage(context.Context) ([]contracts.StorageUsage, error) {
	return []contracts.StorageUsage{{Key: "b188.notes", Module: "notes", Bytes: 123, Files: 1, Available: true}}, nil
}

type aiActionCleaner struct{}

func (aiActionCleaner) Scan(context.Context) ([]contracts.CleanupItem, error) {
	return []contracts.CleanupItem{{ID: "visible", Kind: "b188_visible", Module: "notes", Bytes: 2}, {ID: "hidden", Kind: "b188_hidden", Module: "notes", Hidden: true, Name: "隐藏附件", Bytes: 9}}, nil
}
func (aiActionCleaner) Clean(context.Context, []string) (contracts.CleanupResult, error) {
	return contracts.CleanupResult{}, nil
}

func TestAIMaintenanceActions(t *testing.T) {
	env := testutil.New(t)
	module.Provide[contracts.StorageReporter](env.App.Deps.Registry, contracts.MaintenanceStoragePrefix+"b188", aiActionReporter{})
	module.Provide[contracts.Cleaner](env.App.Deps.Registry, contracts.MaintenanceCleanerPrefix+"b188", aiActionCleaner{})
	env.MustDo(http.MethodPost, "/maintenance/scan", nil, nil)
	deadline := time.Now().Add(10 * time.Second)
	var status map[string]any
	for {
		status = callAIAction(t, env, "maintenance.status", `{}`).(map[string]any)
		if status["state"] != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("扫描未完成")
		}
		time.Sleep(10 * time.Millisecond)
	}
	raw, _ := json.Marshal(status)
	if strings.Contains(string(raw), "b188_hidden") || strings.Contains(string(raw), "隐藏附件") {
		t.Fatalf("隐藏扫描结果泄漏: %s", raw)
	}
	callAIAction(t, env, "maintenance.status", `{"cleanup":true}`)
	overview := callAIAction(t, env, "maintenance.overview", `{}`).(map[string]any)
	if overview["process"] == nil || overview["dataDir"] != nil {
		t.Fatalf("概况: %+v", overview)
	}
	module.Provide[contracts.HiddenModules](env.App.Deps.Registry, contracts.HiddenModulesKey, aiHiddenModules{"notes": true})
	overview = callAIAction(t, env, "maintenance.overview", `{}`).(map[string]any)
	raw, _ = json.Marshal(overview)
	if strings.Contains(string(raw), "b188.notes") {
		t.Fatalf("缓存中的隐藏存储泄漏: %s", raw)
	}
	checkAIActionInputs(t, env, "maintenance.overview", "maintenance.status")
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
