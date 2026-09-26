# 后端开发约定

读完这篇再写 Go 代码。M0 的实现就是样板：`internal/server/core/`、`internal/server/app/app_test.go`。

## 目录

```
backend/
  cmd/server/main.go            服务端入口
  cmd/agent/main.go             代理入口，register() 里一行接一个功能包
  internal/server/
    app/                        组装：基础服务、路由、模块列表（modules.go）
    module/                     模块接口 Deps / Module / Starter / PublicPather / Registry
    auth/ audit/ secrets/ settings/ events/ scheduler/ notify/ agenthub/ ws/ httpx/
    core/                       M0 的接口实现和查询
    store/migrations/           所有迁移文件
    testutil/                   测试用的完整服务器
    modules/<模块>/             每个功能模块一个目录
  internal/agent/
    conn/ config/ sysinfo/      连接、配置、基础信息
    <功能包>/                   metrics、proc、pty、coding……
  pkg/protocol/                 消息定义。每个模块把方法常量写在 methods_<模块>.go
  pkg/rpc/                      Peer 和 Stream
  sqlc.yaml                     每个模块一项
```

## 新增一个模块的步骤

以“项目”模块为例，目录名 `projects`。

1. **写接口契约** `api/modules/projects.yaml`。
   - `servers: [{url: /api/v1}]`，路径不带前缀，比如 `/projects`。
   - 错误响应引用 `../common.yaml#/components/responses/Error`。
   - 分页参数引用 `../common.yaml#/components/parameters/Limit` 和 `Cursor`。
   - 每个操作写 `operationId`，用小驼峰。
2. **写迁移** `backend/internal/server/store/migrations/<UTC时间>_m5_projects.sql`。
   - 文件名前缀用当前 UTC 时间 `YYYYMMDDHHMMSS`，保证不和别人冲突。
   - 必须有 `-- +goose Up` 和 `-- +goose Down`。
   - 表名用复数，列名用蛇形。时间列类型写 `DATETIME`。
   - 外键加 `ON DELETE` 策略。常用查询加索引。
3. **写查询** `backend/internal/server/modules/projects/queries.sql`，然后在 `backend/sqlc.yaml` 加一项：
   ```yaml
   - engine: sqlite
     schema: internal/server/store/migrations
     queries: internal/server/modules/projects/queries.sql
     gen:
       go:
         package: db
         out: internal/server/modules/projects/db
         emit_pointers_for_null_types: true
   ```
   sqlc 处理不了的语句（比如 FTS5 虚拟表的 MATCH）直接写在 Go 里，用 `database/sql`。
4. **生成接口代码**：建 `modules/projects/api/cfg.yaml` 和 `gen.go`，照抄 `core/api/` 的写法，改文件名。
5. **写模块** `modules/projects/module.go`：
   ```go
   package projects

   type Module struct {
       d *module.Deps
       q *db.Queries
   }

   func New(d *module.Deps) (module.Module, error) {
       return &Module{d: d, q: db.New(d.DB)}, nil
   }

   func (m *Module) Name() string { return "projects" }

   func (m *Module) Mount(r chi.Router) {
       api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
   }

   var _ api.ServerInterface = (*Module)(nil)
   ```
   有后台任务就实现 `Start(ctx) error`。需要免登录的入口（Webhook）就实现 `PublicPaths() []string`。
6. **注册**：在 `internal/server/app/modules.go` 的 `constructors` 里加一行 `projects.New,`，按模块编号排序。
7. **生成代码**：`go generate ./...`。
8. **测试** `modules/projects/projects_test.go`，用 `testutil.New(t, projects.New)`。
9. **前端类型**：`cd web && npm run gen:api`。

## 处理器写法

```go
func (m *Module) CreateIssue(w http.ResponseWriter, r *http.Request, projectID int64) {
    var body api.CreateIssueJSONRequestBody
    if err := httpx.Decode(r, &body); err != nil {
        httpx.Fail(w, r, err)
        return
    }
    issue, err := m.createIssue(r.Context(), projectID, body)
    if err != nil {
        httpx.Fail(w, r, err)
        return
    }
    m.d.Bus.Publish("issue.created", issue)
    httpx.JSON(w, http.StatusCreated, issue)
}
```

规则：

- 处理器只做解析、调用、返回。业务逻辑放在普通方法里，方便测试和给别的模块复用。
- 返回错误一律 `httpx.Fail`。已知错误用 `httpx.ErrNotFound`、`httpx.Invalid("...")`、`httpx.NewError(status, code, msg)`。错误信息写中文，给人看。
- 未知错误直接返回，`httpx.Fail` 会记日志并回 500。不要把内部错误细节回给前端。
- `sql.ErrNoRows` 转成 `httpx.ErrNotFound`。
- 创建成功回 201，删除成功回 204，其余回 200。
- JSON 字段用小驼峰。时间用 RFC 3339。
- 列表用游标分页：`httpx.DecodeIDCursor`、`httpx.EncodeIDCursor`、`httpx.Limit`。返回 `{items, nextCursor}`。数据量天然很小的列表（比如项目列表）可以不分页。

## 基础服务怎么用

| 需求 | 用法 |
| --- | --- |
| 当前用户 | `auth.FromContext(ctx)` |
| 高危操作 | 处理器第一行 `if err := auth.RequireElevated(r.Context()); err != nil { httpx.Fail(w, r, err); return }` |
| 审计 | `m.d.Audit.Record(ctx, "host.exec", hostID, map[string]any{"cmd": cmd}, err)`。写操作都要记 |
| 事件 | `m.d.Bus.Publish("issue.updated", payload)`。payload 用和接口一样的 JSON 结构 |
| 订阅事件 | `ch, cancel := m.d.Bus.Subscribe("coding_task.", 64)`，在 `Start` 里起 goroutine，ctx 结束时 `cancel()` |
| 发通知 | `m.d.Notify.Send(ctx, notify.Notification{Kind: "reminder.due", Title: ..., Link: "/reminders", Source: "reminders", Actions: ...})` |
| 通知按钮回调 | `m.d.Notify.OnAction("reminder.", handler)`，按钮 id 形如 `reminder.done:42` |
| 配置 | `m.d.Settings.Get(ctx, "ha.url", &url)`；令牌用 `SetSecret`。键名 `<模块>.<名称>` |
| 定时任务 | `m.d.Scheduler.Every("monitors.check", time.Minute, fn)` 或 `Cron("brief.daily", "0 8 * * *", fn)`，在 `Start` 里注册 |
| 时区 | `m.d.Config.Location` |
| 调代理 | `m.d.Agents.Call(ctx, agentID, protocol.MethodXxx, params, &out)`；流用 `m.d.Agents.Open` |
| 代理事件 | `m.d.Agents.OnEvent(protocol.EventXxx, func(agentID string, raw json.RawMessage) {...})`，在 `New` 里注册 |
| 代理列表 | `m.d.Agents.List(ctx)`，`agent.Has(protocol.CapPTY)` 判断能力 |
| 模块互调 | 接口定义在 `internal/server/contracts`。提供方在 `New` 里 `module.Provide[contracts.Issues](m.d.Registry, contracts.IssuesKey, svc)`；使用方在用到时 `module.Lookup[contracts.Issues](...)`，取不到就返回 501 `feature_unavailable` |
| 动作目录 | `m.d.Actions.Register(actions.Action{...})`。AI 助手和自动化靠它调用你的功能，见下一节 |

## 动作目录（给 AI 助手和自动化用）

每个模块把“值得按名字触发”的操作注册成动作，比如建 Issue、建提醒、打卡、开关灯、重启服务。M12 会把它们变成 AI 的工具和自动化规则的动作，你不用管 M12 怎么用。

```go
d.Actions.Register(actions.Action{
    Name:        "reminders.create",
    Title:       "新建提醒",
    Description: "Create a reminder. `at` is RFC 3339 in the user's time zone. Returns the reminder id.",
    Input:       actions.Schema(`{"type":"object","properties":{"title":{"type":"string"},"at":{"type":"string","format":"date-time"},"rrule":{"type":"string"}},"required":["title","at"],"additionalProperties":false}`),
    Effect:      actions.Write,
    Run:         m.actionCreate,
})
```

- `Effect`：只读用 `actions.Read`；改数据用 `actions.Write`；执行命令、控制设备、关机用 `actions.Dangerous`。
- `Run` 里复用处理器背后的业务方法，同样写审计日志。
- 每个模块的动作清单写在自己的 `docs/modules/Mx.md` 里。
- 测试里至少跑一次你注册的每个动作。

## 调用外部服务

- 用 `net/http`，每个客户端设超时（一般 15 秒）。
- 令牌从 `Settings` 读，不要写死，不要打日志。
- 测试里用 `httptest.NewServer` 假装外部服务，不要在测试里连真实服务。
- 集成没配置时返回 `httpx.ErrIntegrationMissing`，前端据此显示“去设置”。

## 测试要求

- 每个模块至少覆盖：主要接口的正常路径、一个校验失败、一个 404、需要提升权限的接口在未提升时返回 403。
- 用 `testutil.New(t, yourmodule.New)` 起完整服务器，用 `env.MustDo` 调接口。
- 需要代理的功能用 `env.Agent("server", caps, register)` 起一个真实代理客户端，在 register 里挂上你的处理函数或假实现。
- 定时任务的逻辑写成普通函数，测试里直接调用，不要等调度器。
- 提交前：`gofmt -l .` 没输出，`go vet ./...` 通过，`go test -race ./...` 通过，`GOOS=windows go build ./cmd/agent` 通过。

## 命名

| 东西 | 规则 | 例子 |
| --- | --- | --- |
| 模块目录 | 英文复数或功能名 | `projects`、`habits`、`homeassistant` |
| 事件主题 | `<实体>.<动作>`，实体用单数 | `issue.created`、`host.offline` |
| 设置键 | `<模块>.<名称>` | `telegram.bot_token` |
| 审计动作 | `<实体>.<动作>` | `host.exec`、`coding_task.push` |
| 通知 kind | 同事件主题 | `reminder.due` |
| 代理方法 | `<领域>.<动作>` | `proc.list`、`pty.open` |
