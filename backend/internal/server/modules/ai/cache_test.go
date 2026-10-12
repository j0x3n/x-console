package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestConversationCachePrefixWithinVaultState(t *testing.T) {
	for _, host := range []bool{false, true} {
		t.Run(fmt.Sprintf("host=%t", host), func(t *testing.T) {
			env := testutil.New(t)
			var infoCalls atomic.Int32
			hostID := env.Agent("server", []string{protocol.CapSystemInfo, protocol.CapExec}, func(c *conn.Client) {
				c.Handle(protocol.MethodSystemInfo, func(context.Context, json.RawMessage) (any, error) {
					return protocol.SystemInfo{CPUCores: int(infoCalls.Add(1)), MemoryTotal: 1024}, nil
				})
			})
			env.MustDo("POST", "/vault/setup", map[string]any{"password": "hidden-password"}, nil)
			env.MustDo("PUT", "/vault/modules", map[string]any{"hidden": []string{"notes"}}, nil)
			env.MustDo("POST", "/ai/memories", map[string]any{"text": "缓存测试长期偏好"}, nil)
			var c api.Conversation
			path := "/ai/conversations"
			var hostRef *string
			if host {
				path = "/ai/host-agent/" + hostID + "/conversations"
				hostRef = &hostID
			}
			env.MustDo("POST", path, map[string]any{}, &c)
			svc, _ := module.Lookup[contracts.LLM](env.App.Deps.Registry, contracts.LLMKey)
			m := svc.(*ai.Module)
			fake := llm.NewFake()
			m.SetLLMForTest(fake)
			until := time.Now().Add(time.Hour)
			unlocked := auth.WithSession(context.Background(), &auth.Session{VaultUntil: &until})
			for i, ctx := range []context.Context{context.Background(), context.Background(), unlocked, unlocked, context.Background(), context.Background()} {
				caps := []string{protocol.CapSystemInfo, protocol.CapExec}
				if i%2 == 1 {
					caps[0], caps[1] = caps[1], caps[0]
				}
				raw, _ := json.Marshal(caps)
				if _, err := env.App.Deps.DB.Exec("UPDATE agents SET capabilities=? WHERE id=?", string(raw), hostID); err != nil {
					t.Fatal(err)
				}
				fake.Queue(llm.Result{Text: "完成"}, nil)
				if err := m.SaveTurnForTest(ctx, c.Id, hostRef, fmt.Sprintf("第 %d 轮", i)); err != nil {
					t.Fatal(err)
				}
				if err := m.GenerateForTest(ctx, c.Id); err != nil {
					t.Fatal(err)
				}
			}
			calls := fake.Calls
			for i := 1; i < len(calls); i++ {
				if calls[i].System != calls[0].System {
					t.Fatal("机器信息或锁定状态改变了 system")
				}
				if i%2 == 1 && !reflect.DeepEqual(calls[i].Tools, calls[i-1].Tools) {
					t.Fatal("同一种锁定状态下连续两轮 tools 不同")
				}
				if calls[i].CacheKey != calls[0].CacheKey || !strings.HasSuffix(calls[i].CacheKey, fmt.Sprintf(":%d", c.Id)) {
					t.Fatalf("会话缓存键：%s", calls[i].CacheKey)
				}
				if !reflect.DeepEqual(calls[i].Messages[:len(calls[i-1].Messages)], calls[i-1].Messages) {
					t.Fatal("历史前缀被修改")
				}
			}
			for i, call := range calls {
				if strings.Count(call.System, "缓存测试长期偏好") != 1 || !strings.HasSuffix(call.System, "缓存测试长期偏好") {
					t.Fatal("记忆没有放在 system 末尾")
				}
				for _, message := range call.Messages {
					if strings.Contains(message.Content, "缓存测试长期偏好") || strings.Contains(message.Content, "记忆功能已关闭") {
						t.Fatal("用户消息历史重复保存了记忆")
					}
				}
				if !host {
					sawNotes := false
					for _, tool := range call.Tools {
						if strings.HasPrefix(tool.Name, "notes__") {
							sawNotes = true
						}
					}
					if sawNotes != (i/2 == 1) {
						t.Fatalf("第 %d 轮的笔记工具可见性不对", i)
					}
				}
			}
			if host {
				expected := fmt.Sprintf("能力 [%s %s]", protocol.CapExec, protocol.CapSystemInfo)
				for _, call := range calls {
					if !strings.Contains(call.Messages[len(call.Messages)-1].Content, expected) {
						t.Fatal("能力列表没有固定排序")
					}
				}
				if infoCalls.Load() != 6 || !strings.Contains(calls[0].Messages[0].Content, "CPU 1 核") || !strings.Contains(calls[1].Messages[len(calls[1].Messages)-1].Content, "CPU 2 核") {
					t.Fatal("机器信息没有随本轮用户消息更新")
				}
				if strings.Contains(calls[0].System, "CPU") {
					t.Fatal("机器信息仍在 system")
				}
			}
		})
	}
}

func TestMemoryChangesOnlySystemSuffix(t *testing.T) {
	for _, host := range []bool{false, true} {
		t.Run(fmt.Sprintf("host=%t", host), func(t *testing.T) {
			env := testutil.New(t)
			var hostRef *string
			path := "/ai/conversations"
			if host {
				hostID := env.Agent("server", nil, nil)
				hostRef = &hostID
				path = "/ai/host-agent/" + hostID + "/conversations"
			}
			var c api.Conversation
			env.MustDo("POST", path, map[string]any{}, &c)
			var memory api.AiMemory
			env.MustDo("POST", "/ai/memories", map[string]any{"text": "原来的长期偏好"}, &memory)
			svc, _ := module.Lookup[contracts.LLM](env.App.Deps.Registry, contracts.LLMKey)
			m := svc.(*ai.Module)
			fake := llm.NewFake()
			m.SetLLMForTest(fake)
			ctx := context.Background()
			for i := 0; i < 6; i++ {
				if i == 2 {
					env.MustDo("PATCH", fmt.Sprintf("/ai/memories/%d", memory.Id), map[string]any{"text": "修改后的长期偏好"}, nil)
				}
				if i == 4 {
					env.MustDo("PUT", "/ai/memories/enabled", map[string]any{"enabled": false}, nil)
				}
				fake.Queue(llm.Result{Text: "完成"}, nil)
				if err := m.SaveTurnForTest(ctx, c.Id, hostRef, fmt.Sprintf("第 %d 轮", i)); err != nil {
					t.Fatal(err)
				}
				if err := m.GenerateForTest(ctx, c.Id); err != nil {
					t.Fatal(err)
				}
			}
			calls := fake.Calls
			for i, call := range calls {
				if i%2 == 1 && call.System != calls[i-1].System {
					t.Fatal("记忆不变时 system 改变")
				}
				if !reflect.DeepEqual(call.Tools, calls[0].Tools) {
					t.Fatal("记忆变化改变了工具列表")
				}
				for _, message := range call.Messages {
					if strings.Contains(message.Content, "长期偏好") || strings.Contains(message.Content, "记忆功能已关闭") {
						t.Fatal("用户消息保存了记忆内容或关闭提示")
					}
				}
				if i > 0 && !reflect.DeepEqual(call.Messages[:len(calls[i-1].Messages)], calls[i-1].Messages) {
					t.Fatal("记忆变化改写了消息历史")
				}
			}
			if !strings.HasSuffix(calls[0].System, "原来的长期偏好") || !strings.HasSuffix(calls[2].System, "修改后的长期偏好") {
				t.Fatal("system 末尾没有最新记忆")
			}
			before, _, foundBefore := strings.Cut(calls[0].System, "\n\n## 关于用户的记忆")
			after, _, foundAfter := strings.Cut(calls[2].System, "\n\n## 关于用户的记忆")
			if !foundBefore || !foundAfter || before != after {
				t.Fatal("记忆修改影响了固定说明")
			}
			if strings.Contains(calls[4].System, "长期偏好") || strings.Contains(calls[4].System, "记忆功能已关闭") {
				t.Fatal("关闭记忆后仍发送记忆或关闭提示")
			}
		})
	}
}

func TestHiddenToolsAreOmittedAndBlocked(t *testing.T) {
	env := testutil.New(t)
	env.MustDo("POST", "/vault/setup", map[string]any{"password": "hidden-password"}, nil)
	env.MustDo("PUT", "/vault/modules", map[string]any{"hidden": []string{"notes"}}, nil)
	env.MustDo("POST", "/vault/lock", nil, nil)
	svc, _ := module.Lookup[contracts.LLM](env.App.Deps.Registry, contracts.LLMKey)
	m := svc.(*ai.Module)
	for _, builtin := range []bool{false, true} {
		fake := llm.NewFake()
		m.SetLLMForTest(fake)
		fake.Queue(llm.Result{ToolCalls: []llm.ToolCall{{ID: "hidden", Name: "notes__create", Arguments: json.RawMessage(`{"body":"不能写入"}`)}}}, nil)
		fake.Queue(llm.Result{Text: "完成"}, nil)
		if builtin {
			if _, err := m.RunTools(context.Background(), contracts.ToolRun{Model: "1:test", Access: "write", System: "固定说明", Prompt: "测试"}); err != nil {
				t.Fatal(err)
			}
		} else {
			var c api.Conversation
			env.MustDo("POST", "/ai/conversations", map[string]any{}, &c)
			if err := m.SaveTurnForTest(context.Background(), c.Id, nil, "测试"); err != nil {
				t.Fatal(err)
			}
			if err := m.GenerateForTest(context.Background(), c.Id); err != nil {
				t.Fatal(err)
			}
		}
		for _, call := range fake.Calls {
			for _, tool := range call.Tools {
				if strings.HasPrefix(tool.Name, "notes__") {
					t.Fatalf("锁定时暴露笔记工具：%s", tool.Name)
				}
			}
		}
		if !reflect.DeepEqual(fake.Calls[0].Tools, fake.Calls[1].Tools) || fake.Calls[0].System != fake.Calls[1].System {
			t.Fatal("执行中的前缀变化")
		}
		var count int
		if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM notes").Scan(&count); err != nil || count != 0 {
			t.Fatalf("隐藏模块被执行：%d %v", count, err)
		}
	}
}

func TestPolishCachePrefixAndUserRequest(t *testing.T) {
	env := testutil.New(t)
	svc, _ := module.Lookup[contracts.LLM](env.App.Deps.Registry, contracts.LLMKey)
	fake := llm.NewFake()
	svc.(*ai.Module).SetLLMForTest(fake)
	for _, request := range []string{"改成列表", "改成段落"} {
		fake.Queue(llm.Result{Text: "完成"}, nil)
		env.MustDo("POST", "/ai/polish", map[string]any{"text": "原文", "scene": "note", "prompt": request}, nil)
	}
	if fake.Calls[0].System != fake.Calls[1].System || fake.Calls[0].CacheKey != "polish:note" || !strings.Contains(fake.Calls[1].Messages[0].Content, "改成段落") {
		t.Fatalf("润色请求：%+v", fake.Calls)
	}
}
