package vault_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/vault/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestHiddenModules(t *testing.T) {
	env := testutil.New(t)
	var avail api.AvailableModules
	env.MustDo(http.MethodGet, "/app/modules", nil, &avail)
	if len(avail.Modules) != 18 { // B65 加了 router，B115 加了 documents
		t.Fatalf("all modules before any setting: %v", avail.Modules)
	}
	// 没解锁时读写隐藏模块都像接口不存在
	checkCode(t, env, http.MethodGet, "/vault/modules", nil, 404, "not_found")
	checkCode(t, env, http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"github"}}, 404, "not_found")

	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, nil)
	checkCode(t, env, http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"today"}}, 400, "validation_failed")
	var hidden api.HiddenModules
	env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"github", "notes", "github"}}, &hidden)
	if !slices.Equal(hidden.Hidden, []api.ModuleId{api.Notes, api.Github}) {
		t.Fatalf("saved: %v", hidden.Hidden)
	}
	// 解锁时全部可用
	env.MustDo(http.MethodGet, "/app/modules", nil, &avail)
	if len(avail.Modules) != 18 { // B65 加了 router，B115 加了 documents
		t.Fatalf("unlocked: %v", avail.Modules)
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	env.MustDo(http.MethodGet, "/app/modules", nil, &avail)
	if len(avail.Modules) != 16 || slices.Contains(avail.Modules, api.Notes) || slices.Contains(avail.Modules, api.Github) {
		t.Fatalf("locked: %v", avail.Modules)
	}
	checkCode(t, env, http.MethodGet, "/vault/modules", nil, 404, "not_found")
}
