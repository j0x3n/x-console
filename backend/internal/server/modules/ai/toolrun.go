package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

// B47 built-in agents: a model with the actions the agent's access level
// allows (the B43 filter), run to the end without asking anybody.

type modelOverrideKey struct{}

// withModel makes resolveLLM use ref instead of the configured model.
func withModel(ctx context.Context, ref api.ModelRef) context.Context {
	return context.WithValue(ctx, modelOverrideKey{}, ref)
}

func modelOverride(ctx context.Context) (api.ModelRef, bool) {
	ref, ok := ctx.Value(modelOverrideKey{}).(api.ModelRef)
	return ref, ok
}

var _ contracts.ToolRunner = (*Module)(nil)

// RunTools implements contracts.ToolRunner.
func (m *Module) RunTools(ctx context.Context, in contracts.ToolRun) (string, error) {
	provider, model, ok := strings.Cut(in.Model, ":")
	pid, err := strconv.ParseInt(provider, 10, 64)
	if !ok || err != nil || model == "" {
		return "", errors.New("模型格式不对，应该是 <供应商 id>:<模型 id>")
	}
	ctx = withModel(ctx, api.ModelRef{ProviderId: pid, Model: model})
	ctx = contracts.WithAIUsage(ctx, in.Source, in.Ref)
	ctx = auth.WithoutVault(ctx)
	allowed := map[string]actions.Action{}
	var tools []llm.Tool
	for _, a := range m.d.Actions.List() {
		if !json.Valid(a.Input) || !actions.AllowedFor(a, in.Access, nil) {
			continue
		}
		name := strings.ReplaceAll(a.Name, ".", "__")
		allowed[name] = a
		desc := a.Title
		if a.Description != "" && a.Description != a.Title {
			desc += "。" + a.Description
		}
		tools = append(tools, llm.Tool{Name: name, Description: desc, Parameters: a.Input})
	}
	turns := in.MaxTurns
	if turns <= 0 {
		turns = 20
	}
	history := []llm.Message{{Role: "user", Content: in.Prompt}}
	for turn := 0; turn < turns; turn++ {
		res, err := m.llm.Complete(ctx, llm.Request{Purpose: "agent", System: in.System, Messages: history, Tools: tools})
		if err != nil {
			return "", err
		}
		if len(res.ToolCalls) == 0 {
			return strings.TrimSpace(res.Text), nil
		}
		history = append(history, llm.Message{Role: "assistant", Content: res.Text, ToolCalls: res.ToolCalls})
		for _, call := range res.ToolCalls {
			content, isErr := "", false
			if a, ok := allowed[call.Name]; !ok {
				content, isErr = "没有这个工具，或者这个 Agent 不能用它", true
			} else {
				args := call.Arguments
				if len(args) == 0 || string(args) == "null" {
					args = json.RawMessage("{}")
				}
				out, err := a.Run(ctx, args)
				m.d.Audit.Record(ctx, "ai_agent.action", a.Name, map[string]any{"ref": in.Ref}, err)
				if err != nil {
					content, isErr = err.Error(), true
				} else {
					raw, _ := json.Marshal(out)
					content = string(raw)
				}
			}
			if isErr {
				content = "错误：" + content
			}
			history = append(history, llm.Message{Role: "tool", ToolCallID: call.ID, Content: content})
		}
	}
	return "", fmt.Errorf("调用工具超过 %d 轮，已经停止", turns)
}
