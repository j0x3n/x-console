package coding

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// B42：把 Claude Code 和 Codex 在代理上跑出来的用量记进 AI 用量。

// executorUsage is the token count in a "result" status or error event.
type executorUsage struct {
	input, cached, cacheWrite, output int64
	cost                              *float64
}

// parseExecutorUsage reads the usage from an event's data. Claude Code
// reports Anthropic style counts (input without cache reads and writes)
// and a total cost; Codex reports OpenAI style counts (input includes the
// cached part).
func parseExecutorUsage(executor string, raw json.RawMessage) (executorUsage, bool) {
	var d struct {
		Code    string   `json:"code"`
		CostUSD *float64 `json:"costUsd"`
		Usage   struct {
			Input       int64 `json:"input_tokens"`
			Output      int64 `json:"output_tokens"`
			CacheRead   int64 `json:"cache_read_input_tokens"`
			CacheCreate int64 `json:"cache_creation_input_tokens"`
			Cached      int64 `json:"cached_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &d) != nil || d.Code != "result" {
		return executorUsage{}, false
	}
	u := d.Usage
	if u.Input == 0 && u.Output == 0 && u.CacheRead == 0 && u.CacheCreate == 0 {
		return executorUsage{}, false
	}
	out := executorUsage{output: u.Output}
	if executor == protocol.ExecutorClaude {
		out.input = u.Input + u.CacheRead + u.CacheCreate
		out.cached, out.cacheWrite = u.CacheRead, u.CacheCreate
		if d.CostUSD != nil && *d.CostUSD > 0 {
			out.cost = d.CostUSD
		}
	} else {
		out.input, out.cached = u.Input, u.Cached
	}
	return out, true
}

// noteUsage remembers the model and records the usage of a finished run.
func (m *Module) noteUsage(ctx context.Context, run *taskRun, ev protocol.CodingEvent) {
	if ev.Kind != protocol.CodingEventStatus && ev.Kind != protocol.CodingEventError {
		return
	}
	var d struct {
		Code  string `json:"code"`
		Model string `json:"model"`
	}
	if json.Unmarshal(ev.Data, &d) != nil {
		return
	}
	if d.Code == "session_started" && d.Model != "" {
		run.model = d.Model
		return
	}
	u, ok := parseExecutorUsage(run.executor, ev.Data)
	if !ok {
		return
	}
	rec, ok := module.Lookup[contracts.AIUsageRecorder](m.d.Registry, contracts.AIUsageKey)
	if !ok {
		return
	}
	provider, model := "Claude Code", run.model
	if run.executor == protocol.ExecutorCodex {
		provider = "Codex"
	}
	if model == "" {
		model = run.executor
	}
	rec.RecordExternalUsage(context.WithoutCancel(ctx), provider, model, "coding", strconv.FormatInt(run.id, 10),
		u.input, u.cached, u.cacheWrite, u.output, m.now().Sub(run.started), u.cost)
}
