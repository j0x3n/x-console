package assistant_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/assistant"
	"github.com/j0x3n/x-console/backend/internal/server/modules/assistant/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/automations"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type actionModule struct{}

func (*actionModule) Name() string     { return "assistant_test_action" }
func (*actionModule) Mount(chi.Router) {}

func fakeClaude(t *testing.T, actionName string) (*httptest.Server, *atomic.Int32, func() []string) {
	t.Helper()
	requests := []string{}
	var mu sync.Mutex
	calls := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("Claude request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		raw, _ := json.Marshal(body)
		mu.Lock()
		requests = append(requests, string(raw))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		start := `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}`
		fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", start)
		if calls.Add(1) == 1 {
			block := fmt.Sprintf(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_test","name":%q,"input":{}}}`, actionName)
			fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", block)
			fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
			fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":2}}\n\n")
		} else {
			fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"完成\"}}\n\n")
			fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
			fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":2}}\n\n")
		}
		fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	return server, calls, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), requests...)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for AI reply")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestToolExecutionAndRejection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		effect actions.Effect
	}{
		{"read runs automatically", actions.Read},
		{"write waits and rejection reaches Claude", actions.Write},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var executions atomic.Int32
			env := testutil.New(t, func(d *module.Deps) (module.Module, error) {
				d.Actions.Register(actions.Action{Name: "test.mark", Title: "测试", Description: "test action", Effect: tc.effect, Input: actions.Schema(`{"type":"object","properties":{}}`), Run: func(context.Context, json.RawMessage) (any, error) {
					executions.Add(1)
					return map[string]any{"ok": true}, nil
				}})
				return &actionModule{}, nil
			})
			srv, calls, requests := fakeClaude(t, "test__mark")
			defer srv.Close()
			asker, ok := module.Lookup[automations.Asker](env.App.Deps.Registry, automations.AskerKey)
			if !ok {
				t.Fatal("assistant module not registered")
			}
			asker.(*assistant.Module).SetTestEndpoint(srv.URL)
			env.Elevate()
			env.MustDo(http.MethodPut, "/ai/settings", map[string]any{"model": "test", "apiKey": "test-key"}, nil)
			var encrypted int
			if err := env.App.Deps.DB.QueryRow(`SELECT encrypted FROM settings WHERE key='ai.api_key'`).Scan(&encrypted); err != nil || encrypted != 1 {
				t.Fatalf("API key storage: encrypted=%d err=%v", encrypted, err)
			}
			var conversation api.Conversation
			env.MustDo(http.MethodPost, "/ai/conversations", map[string]any{}, &conversation)
			env.MustDo(http.MethodPost, "/ai/conversations/"+conversation.Id+"/messages", map[string]any{"text": "执行测试"}, nil)
			waitFor(t, func() bool { return calls.Load() >= 1 })
			if tc.effect == actions.Write {
				var detail api.ConversationDetail
				waitFor(t, func() bool {
					env.MustDo(http.MethodGet, "/ai/conversations/"+conversation.Id, nil, &detail)
					return len(detail.PendingActions) == 1
				})
				if executions.Load() != 0 || calls.Load() != 1 {
					t.Fatalf("write ran before confirmation: executions=%d calls=%d", executions.Load(), calls.Load())
				}
				env.MustDo(http.MethodPost, "/ai/actions/"+detail.PendingActions[0].Id+"/reject", nil, nil)
			}
			waitFor(t, func() bool { return calls.Load() >= 2 })
			var detail api.ConversationDetail
			waitFor(t, func() bool {
				env.MustDo(http.MethodGet, "/ai/conversations/"+conversation.Id, nil, &detail)
				return len(detail.Messages) >= 4
			})
			if tc.effect == actions.Read && executions.Load() != 1 {
				t.Fatalf("read executions=%d", executions.Load())
			}
			if tc.effect == actions.Write {
				if executions.Load() != 0 || !strings.Contains(requests()[1], "用户拒绝了") {
					t.Fatalf("rejection missing from Claude request: executions=%d", executions.Load())
				}
			}
		})
	}
}
