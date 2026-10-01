package aiagents

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	kindClaude  = "claude_code"
	kindCodex   = "codex"
	kindBuiltin = "builtin"
)

func isCLI(kind string) bool { return kind == kindClaude || kind == kindCodex }

func repoIDsOf(raw string) []int64 {
	var ids []int64
	_ = json.Unmarshal([]byte(raw), &ids)
	if ids == nil {
		ids = []int64{}
	}
	return ids
}

// monthStart is the first moment of this month in the user's time zone.
func (m *Module) monthStart() time.Time {
	loc := m.d.Config.Location
	if loc == nil {
		loc = time.UTC
	}
	now := m.now().In(loc)
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).UTC()
}

func (m *Module) monthCost(ctx context.Context, id int64) (float64, error) {
	return m.q.AgentMonthCost(ctx, db.AgentMonthCostParams{Since: m.monthStart(), AgentID: &id})
}

type taskCounts struct{ running, queued int }

func (m *Module) taskCounts(ctx context.Context) (map[int64]taskCounts, error) {
	rows, err := m.q.AgentTaskCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := map[int64]taskCounts{}
	for _, r := range rows {
		if r.AiAgentID == nil {
			continue
		}
		c := out[*r.AiAgentID]
		if r.Status == "running" {
			c.running = int(r.N)
		} else {
			c.queued = int(r.N)
		}
		out[*r.AiAgentID] = c
	}
	return out, nil
}

func (m *Module) runnerNames(ctx context.Context) map[string]string {
	out := map[string]string{}
	rows, err := m.q.AgentNames(ctx)
	if err != nil {
		return out
	}
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out
}

func (m *Module) toAgent(ctx context.Context, a db.AiAgent, counts map[int64]taskCounts, names map[string]string) (api.AiAgent, error) {
	cost, err := m.monthCost(ctx, a.ID)
	if err != nil {
		return api.AiAgent{}, err
	}
	out := api.AiAgent{Id: a.ID, Name: a.Name, Avatar: a.Avatar, Color: a.Color, Kind: api.AiAgentKind(a.Kind),
		Model: a.Model, Instructions: a.Instructions, RunnerAgentId: a.RunnerAgentID, Access: api.AiAgentAccess(a.Access),
		CliPermission: api.CliPermission(a.CliPermission), RepoIds: repoIDsOf(a.RepoIds), MaxParallel: int(a.MaxParallel),
		MonthlyBudgetUsd: a.MonthlyBudgetUsd, AutoBuild: a.AutoBuild == 1, BuildRetries: int(a.BuildRetries),
		Enabled: a.Enabled == 1, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt, MonthCostUsd: cost,
		RunningTasks: counts[a.ID].running + m.busy(a.ID), QueuedTasks: counts[a.ID].queued}
	over := a.MonthlyBudgetUsd != nil && cost >= *a.MonthlyBudgetUsd
	out.OverBudget = &over
	if a.RunnerAgentID != nil {
		if name, ok := names[*a.RunnerAgentID]; ok {
			out.RunnerName = &name
		}
	}
	return out, nil
}

func (m *Module) agentRow(ctx context.Context, id int64) (db.AiAgent, error) {
	a, err := m.q.GetAgent(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return a, httpx.ErrNotFound
	}
	return a, err
}

func (m *Module) agent(ctx context.Context, id int64) (api.AiAgent, error) {
	a, err := m.agentRow(ctx, id)
	if err != nil {
		return api.AiAgent{}, err
	}
	counts, err := m.taskCounts(ctx)
	if err != nil {
		return api.AiAgent{}, err
	}
	return m.toAgent(ctx, a, counts, m.runnerNames(ctx))
}

func (m *Module) ListAiAgents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := m.q.ListAgents(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	counts, err := m.taskCounts(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	names := m.runnerNames(ctx)
	out := make([]api.AiAgent, 0, len(rows))
	for _, a := range rows {
		v, err := m.toAgent(ctx, a, counts, names)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		out = append(out, v)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) GetAiAgent(w http.ResponseWriter, r *http.Request, id int64) {
	a, err := m.agent(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, a)
}

// fields is an agent being created or changed, before it is stored.
type fields struct {
	db.AiAgent
	repoIDs []int64
}

// apply copies the given input onto f. present holds the JSON keys that
// were sent, to tell a null apart from a missing field.
func apply(f *fields, in api.AiAgentInput, present map[string]bool) {
	if in.Name != nil {
		f.Name = strings.TrimSpace(*in.Name)
	}
	if in.Avatar != nil {
		f.Avatar = strings.TrimSpace(*in.Avatar)
	}
	if in.Color != nil {
		f.Color = strings.TrimSpace(*in.Color)
	}
	if in.Model != nil {
		f.Model = strings.TrimSpace(*in.Model)
	}
	if in.Instructions != nil {
		f.Instructions = strings.TrimSpace(*in.Instructions)
	}
	if present["runnerAgentId"] {
		f.RunnerAgentID = nil
		if in.RunnerAgentId != nil && strings.TrimSpace(*in.RunnerAgentId) != "" {
			v := strings.TrimSpace(*in.RunnerAgentId)
			f.RunnerAgentID = &v
		}
	}
	if in.Access != nil {
		f.Access = string(*in.Access)
	}
	if in.CliPermission != nil {
		f.CliPermission = string(*in.CliPermission)
	}
	if in.RepoIds != nil {
		f.repoIDs = *in.RepoIds
	}
	if in.MaxParallel != nil {
		f.MaxParallel = int64(*in.MaxParallel)
	}
	if present["monthlyBudgetUsd"] {
		f.MonthlyBudgetUsd = in.MonthlyBudgetUsd
	}
	if in.AutoBuild != nil {
		f.AutoBuild = boolInt(*in.AutoBuild)
	}
	if in.BuildRetries != nil {
		f.BuildRetries = int64(*in.BuildRetries)
	}
	if in.Enabled != nil {
		f.Enabled = boolInt(*in.Enabled)
	}
}

// validate checks f. before is the stored agent when changing one; the
// sensitive changes (full CLI permission, repository access) then need a
// fresh verification.
func (m *Module) validate(ctx context.Context, f *fields, before *fields) error {
	if f.Name == "" || len([]rune(f.Name)) > 40 {
		return httpx.Invalid("名字要 1 到 40 个字")
	}
	if len([]rune(f.Avatar)) > 4 {
		return httpx.Invalid("头像只放一个 emoji")
	}
	if f.Kind != kindClaude && f.Kind != kindCodex && f.Kind != kindBuiltin {
		return httpx.Invalid("类型只能是 claude_code、codex 或 builtin")
	}
	if f.Access != "read" && f.Access != "write" && f.Access != "write_delete" {
		return httpx.Invalid("权限只能是 read、write 或 write_delete")
	}
	if f.CliPermission != "workspace" && f.CliPermission != "full" {
		return httpx.Invalid("命令行权限只能是 workspace 或 full")
	}
	if f.MaxParallel < 1 || f.MaxParallel > 10 {
		return httpx.Invalid("同时任务数在 1 到 10 之间")
	}
	if f.BuildRetries < 0 || f.BuildRetries > 5 {
		return httpx.Invalid("构建失败后重试次数在 0 到 5 之间")
	}
	if f.MonthlyBudgetUsd != nil && *f.MonthlyBudgetUsd < 0 {
		return httpx.Invalid("预算不能是负数")
	}
	if len(f.Model) > 200 || len(f.Instructions) > 20000 {
		return httpx.Invalid("模型或固定说明太长了")
	}
	if f.Kind == kindBuiltin {
		if provider, model, ok := strings.Cut(f.Model, ":"); !ok || provider == "" || model == "" {
			return httpx.Invalid("内置 Agent 要选一个模型")
		}
	}
	if f.RunnerAgentID != nil {
		if !isCLI(f.Kind) {
			f.RunnerAgentID = nil
		} else {
			a, err := m.d.Agents.Get(ctx, *f.RunnerAgentID)
			if err != nil {
				return httpx.Invalid("默认机器不存在")
			}
			if a.Online && !a.Has(protocol.CapCoding) {
				return httpx.Invalid("这台机器的代理不支持编码任务")
			}
		}
	}
	slices.Sort(f.repoIDs)
	f.repoIDs = slices.Compact(f.repoIDs)
	if len(f.repoIDs) > 0 {
		known, err := m.q.RepoIDs(ctx)
		if err != nil {
			return err
		}
		kept := f.repoIDs[:0]
		for _, id := range f.repoIDs {
			switch {
			case slices.Contains(known, id):
				kept = append(kept, id)
			case before != nil && slices.Contains(before.repoIDs, id):
				// The repository was deleted since: drop it quietly.
			default:
				return httpx.Invalid("仓库 " + strconv.FormatInt(id, 10) + " 不存在")
			}
		}
		f.repoIDs = kept
	}
	raw, _ := json.Marshal(f.repoIDs)
	f.RepoIds = string(raw)
	// Sensitive changes need a fresh verification.
	sensitive := f.CliPermission == "full" && (before == nil || before.CliPermission != "full")
	if before == nil {
		sensitive = sensitive || len(f.repoIDs) > 0
	} else if !slices.Equal(f.repoIDs, before.repoIDs) {
		sensitive = true
	}
	if sensitive {
		return auth.RequireElevated(ctx)
	}
	return nil
}

// decodeInput reads the body and remembers which keys it had.
func decodeInput(r *http.Request) (api.AiAgentInput, map[string]bool, error) {
	var raw map[string]json.RawMessage
	if err := httpx.Decode(r, &raw); err != nil {
		return api.AiAgentInput{}, nil, err
	}
	present := map[string]bool{}
	for k := range raw {
		present[k] = true
	}
	all, _ := json.Marshal(raw)
	var in api.AiAgentInput
	if err := json.Unmarshal(all, &in); err != nil {
		return in, nil, httpx.Invalid("参数格式不正确：" + err.Error())
	}
	return in, present, nil
}

func (m *Module) CreateAiAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, present, err := decodeInput(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var id int64
	err = func() (err error) {
		defer func() {
			m.d.Audit.Record(ctx, "ai_agent.create", strconv.FormatInt(id, 10), map[string]any{"kind": in.Kind}, err)
		}()
		if in.Kind == nil {
			return httpx.Invalid("请选择类型")
		}
		f := fields{AiAgent: db.AiAgent{Kind: string(*in.Kind), Access: "write", CliPermission: "workspace",
			MaxParallel: 1, AutoBuild: 1, BuildRetries: 2, Enabled: 1}, repoIDs: []int64{}}
		apply(&f, in, present)
		if err := m.validate(ctx, &f, nil); err != nil {
			return err
		}
		now := m.now()
		a, err := m.q.CreateAgent(ctx, db.CreateAgentParams{Name: f.Name, Avatar: f.Avatar, Color: f.Color, Kind: f.Kind,
			Model: f.Model, Instructions: f.Instructions, RunnerAgentID: f.RunnerAgentID, Access: f.Access,
			CliPermission: f.CliPermission, RepoIds: f.RepoIds, MaxParallel: f.MaxParallel, MonthlyBudgetUsd: f.MonthlyBudgetUsd,
			AutoBuild: f.AutoBuild, BuildRetries: f.BuildRetries, Enabled: f.Enabled, CreatedAt: now, UpdatedAt: now})
		id = a.ID
		return err
	}()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a, err := m.agent(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("ai_agent.changed", a)
	httpx.JSON(w, http.StatusCreated, a)
}

func (m *Module) UpdateAiAgent(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	in, present, err := decodeInput(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err = func() (err error) {
		defer func() {
			m.d.Audit.Record(ctx, "ai_agent.update", strconv.FormatInt(id, 10), nil, err)
		}()
		row, err := m.agentRow(ctx, id)
		if err != nil {
			return err
		}
		if in.Kind != nil && string(*in.Kind) != row.Kind {
			return httpx.Invalid("类型建好后不能改")
		}
		before := fields{AiAgent: row, repoIDs: repoIDsOf(row.RepoIds)}
		f := fields{AiAgent: row, repoIDs: repoIDsOf(row.RepoIds)}
		apply(&f, in, present)
		if err := m.validate(ctx, &f, &before); err != nil {
			return err
		}
		_, err = m.q.UpdateAgent(ctx, db.UpdateAgentParams{Name: f.Name, Avatar: f.Avatar, Color: f.Color, Model: f.Model,
			Instructions: f.Instructions, RunnerAgentID: f.RunnerAgentID, Access: f.Access, CliPermission: f.CliPermission,
			RepoIds: f.RepoIds, MaxParallel: f.MaxParallel, MonthlyBudgetUsd: f.MonthlyBudgetUsd, AutoBuild: f.AutoBuild,
			BuildRetries: f.BuildRetries, Enabled: f.Enabled, UpdatedAt: m.now(), ID: id})
		return err
	}()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a, err := m.agent(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("ai_agent.changed", a)
	httpx.JSON(w, http.StatusOK, a)
}

func (m *Module) DeleteAiAgent(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	err := func() (err error) {
		defer func() { m.d.Audit.Record(ctx, "ai_agent.delete", strconv.FormatInt(id, 10), nil, err) }()
		counts, err := m.taskCounts(ctx)
		if err != nil {
			return err
		}
		if c := counts[id]; c.running+c.queued > 0 {
			return httpx.NewError(http.StatusConflict, "conflict", "这个 Agent 还有任务在跑或在排队，先取消")
		}
		tx, err := m.d.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		q := m.q.WithTx(tx)
		if err := q.DetachAgentTasks(ctx, &id); err != nil {
			return err
		}
		n, err := q.DeleteAgent(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return httpx.ErrNotFound
		}
		return tx.Commit()
	}()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("ai_agent.changed", map[string]any{"id": id, "deleted": true})
	httpx.NoContent(w)
}

// Get implements contracts.AIAgents.
func (m *Module) Get(ctx context.Context, id int64) (contracts.AIAgent, error) {
	a, err := m.agentRow(ctx, id)
	if err != nil {
		return contracts.AIAgent{}, err
	}
	out := contracts.AIAgent{ID: a.ID, Name: a.Name, Kind: a.Kind, Model: a.Model, Instructions: a.Instructions,
		CLIPermission: a.CliPermission, RepoIDs: repoIDsOf(a.RepoIds), MaxParallel: int(a.MaxParallel),
		AutoBuild: a.AutoBuild == 1, BuildRetries: int(a.BuildRetries), Enabled: a.Enabled == 1}
	if a.RunnerAgentID != nil {
		out.RunnerAgentID = *a.RunnerAgentID
	}
	if a.MonthlyBudgetUsd != nil {
		cost, err := m.monthCost(ctx, id)
		if err != nil {
			return out, err
		}
		out.OverBudget = cost >= *a.MonthlyBudgetUsd
	}
	return out, nil
}
