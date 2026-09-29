package ai_test

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
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
