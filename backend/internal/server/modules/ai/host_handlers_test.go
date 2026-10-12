package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestHostConversationAndPermission(t *testing.T) {
	env := testutil.New(t)
	if status, _ := env.Do("GET", "/ai/host-agent/missing/conversations", nil, nil); status != 404 {
		t.Fatalf("missing host: %d", status)
	}
	hostID := env.Agent("server", nil, nil)
	path := "/ai/host-agent/" + hostID + "/conversations"
	var created api.Conversation
	env.MustDo("POST", path, map[string]any{"title": "检查磁盘"}, &created)
	if created.HostId == nil || *created.HostId != hostID || created.Permission == nil || *created.Permission != api.Confirm {
		t.Fatalf("created: %+v", created)
	}
	var floating []api.Conversation
	env.MustDo("GET", "/ai/conversations", nil, &floating)
	if len(floating) != 0 {
		t.Fatalf("host conversation leaked into floating list: %+v", floating)
	}
	var listed []api.Conversation
	env.MustDo("GET", path, nil, &listed)
	if len(listed) != 1 || listed[0].Id != created.Id {
		t.Fatalf("host list: %+v", listed)
	}
	permissionPath := fmt.Sprintf("/ai/conversations/%d/permission", created.Id)
	env.MustDo("PUT", permissionPath, map[string]any{"mode": "read_auto"}, nil)
	var detail api.ConversationDetail
	env.MustDo("GET", fmt.Sprintf("/ai/conversations/%d", created.Id), nil, &detail)
	if detail.Conversation.Permission == nil || *detail.Conversation.Permission != api.ReadAuto {
		t.Fatalf("detail permission: %+v", detail.Conversation)
	}
	if status, _ := env.Do("PUT", permissionPath, map[string]any{"mode": "invalid"}, nil); status != 400 {
		t.Fatalf("invalid mode: %d", status)
	}
	var floatingCreated api.Conversation
	env.MustDo("POST", "/ai/conversations", map[string]any{}, &floatingCreated)
	if status, _ := env.Do("PUT", fmt.Sprintf("/ai/conversations/%d/permission", floatingCreated.Id), map[string]any{"mode": "read_auto"}, nil); status != http.StatusBadRequest {
		t.Fatalf("floating permission: %d", status)
	}
}

func TestHostPermissionExpiresWithoutNewMessages(t *testing.T) {
	env := testutil.New(t)
	svc, ok := module.Lookup[contracts.LLM](env.App.Deps.Registry, contracts.LLMKey)
	if !ok {
		t.Fatal("AI module missing")
	}
	var clock atomic.Int64
	clock.Store(time.Now().Unix())
	svc.(*ai.Module).SetNowForTest(func() time.Time { return time.Unix(clock.Load(), 0) })
	hostID := env.Agent("server", nil, nil)
	var conversation api.Conversation
	env.MustDo("POST", "/ai/host-agent/"+hostID+"/conversations", map[string]any{}, &conversation)
	path := fmt.Sprintf("/ai/conversations/%d", conversation.Id)
	env.MustDo("PUT", path+"/permission", map[string]any{"mode": "all_auto"}, nil)
	var detail api.ConversationDetail
	env.MustDo("GET", path, nil, &detail)
	if detail.Conversation.Permission == nil || *detail.Conversation.Permission != api.AllAuto {
		t.Fatalf("mode before expiry: %+v", detail.Conversation.Permission)
	}
	clock.Add(int64(2 * time.Hour / time.Second))
	env.MustDo("GET", path, nil, &detail)
	if detail.Conversation.Permission == nil || *detail.Conversation.Permission != api.Confirm {
		t.Fatalf("mode after expiry: %+v", detail.Conversation.Permission)
	}
}

func TestHostPermissionSurvivesModuleRestart(t *testing.T) {
	env := testutil.New(t)
	hostID := env.Agent("server", nil, nil)
	var created api.Conversation
	env.MustDo("POST", "/ai/host-agent/"+hostID+"/conversations", map[string]any{"mode": "read_auto"}, &created)
	env.MustDo("PUT", fmt.Sprintf("/ai/conversations/%d/permission", created.Id), map[string]any{"mode": "confirm"}, nil)
	env.MustDo("PUT", fmt.Sprintf("/ai/conversations/%d/permission", created.Id), map[string]any{"mode": "read_auto"}, nil)
	fresh, err := ai.New(env.App.Deps)
	if err != nil {
		t.Fatal(err)
	}
	read := func() api.ConversationDetail {
		rr := httptest.NewRecorder()
		fresh.(*ai.Module).GetAiConversation(rr, httptest.NewRequest("GET", "/", nil), created.Id)
		if rr.Code != 200 {
			t.Fatalf("重启后读取：%d %s", rr.Code, rr.Body)
		}
		var detail api.ConversationDetail
		if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
			t.Fatal(err)
		}
		return detail
	}
	if detail := read(); detail.Conversation.Permission == nil || *detail.Conversation.Permission != api.ReadAuto {
		t.Fatalf("重启后权限：%+v", detail.Conversation)
	}
	env.Elevate()
	env.MustDo("PUT", fmt.Sprintf("/ai/conversations/%d/permission", created.Id), map[string]any{"mode": "all_auto"}, nil)
	fresh.(*ai.Module).SetNowForTest(func() time.Time { return time.Now().Add(2 * time.Hour) })
	if detail := read(); detail.Conversation.Permission == nil || *detail.Conversation.Permission != api.Confirm {
		t.Fatalf("过期权限：%+v", detail.Conversation)
	}
}

func TestHostAllAutoRenewsOnlyBeforeExpiry(t *testing.T) {
	env, c, _, _ := hostScenario(t, "df -h", func(_ context.Context, _ protocol.ExecParams) (any, error) {
		return protocol.ExecResult{ExitCode: 0}, nil
	})
	svc, _ := module.Lookup[contracts.LLM](env.App.Deps.Registry, contracts.LLMKey)
	var clock atomic.Int64
	clock.Store(time.Now().Unix())
	svc.(*ai.Module).SetNowForTest(func() time.Time { return time.Unix(clock.Load(), 0) })
	path := fmt.Sprintf("/ai/conversations/%d", c.Id)
	env.MustDo("PUT", path+"/permission", map[string]any{"mode": "all_auto"}, nil)
	clock.Add(int64(time.Hour / time.Second))
	env.MustDo("POST", path+"/messages", map[string]any{"text": "查看磁盘"}, nil)
	await(t, env, c.Id, func(d api.ConversationDetail) bool { return !d.Running && len(d.Messages) >= 4 })
	clock.Add(int64(90 * time.Minute / time.Second))
	var detail api.ConversationDetail
	env.MustDo("GET", path, nil, &detail)
	if *detail.Conversation.Permission != api.AllAuto {
		t.Fatal("新消息没有续期")
	}
	clock.Add(int64(30 * time.Minute / time.Second))
	// 过期后直接发消息，也不能恢复全部自动。
	env.MustDo("POST", path+"/messages", map[string]any{"text": "再看一次"}, nil)
	await(t, env, c.Id, func(d api.ConversationDetail) bool { return !d.Running && len(d.Messages) >= 6 })
	env.MustDo("GET", path, nil, &detail)
	if *detail.Conversation.Permission != api.Confirm {
		t.Fatal("过期权限被消息恢复")
	}
	var stored string
	if err := env.App.Deps.DB.QueryRow("SELECT host_permission FROM ai_conversations WHERE id=?", c.Id).Scan(&stored); err != nil || stored != "confirm" {
		t.Fatalf("过期权限没有存库：%s %v", stored, err)
	}
}

func TestHostCreatePermissionValidationAndElevation(t *testing.T) {
	env := testutil.New(t)
	hostID := env.Agent("server", nil, nil)
	if _, err := env.App.Deps.DB.Exec("UPDATE sessions SET elevated_until=NULL"); err != nil {
		t.Fatal(err)
	}
	path := "/ai/host-agent/" + hostID + "/conversations"
	for _, tc := range []struct {
		mode   string
		status int
	}{{"invalid", 400}, {"all_auto", 403}} {
		if status, _ := env.Do("POST", path, map[string]any{"mode": tc.mode}, nil); status != tc.status {
			t.Fatalf("%s：%d", tc.mode, status)
		}
	}
	env.Elevate()
	var created api.Conversation
	env.MustDo("POST", path, map[string]any{"mode": "all_auto"}, &created)
	if created.Permission == nil || *created.Permission != api.AllAuto {
		t.Fatalf("创建权限：%+v", created)
	}
}
