# 开发计划与任务卡

## 批次

| 批次 | 内容 | 状态 |
| --- | --- | --- |
| 0 | 文档、仓库结构、M0 基础、前端外壳、协议、contracts、actions | 已完成 |
| 1 | A：M5 项目 + M6 备忘；B：M7 提醒通知 + M8 习惯；C：代理 + M2 服务器 + M3 本机；D：M9 Home Assistant | 进行中 |
| 2 | E：M4 编码任务；F：M10 运维监控；G：M11 日历早报番茄钟；H：M13 GitHub + Linear | 未开始 |
| 3 | I：M12 AI 助手与自动化；J：M1 首页、命令面板增强、PWA、部署、端到端测试 | 未开始 |

每批结束后统一合并、全量测试、浏览器冒烟，再开下一批。

## 子代理工作规则

1. **先读文档**：`docs/README.md`、`03-backend.md`、`04-agent-protocol.md`（涉及代理时）、`05-frontend.md`、自己的 `modules/Mx.md`。M0 的代码是样板。
2. **只改自己的地盘**：
   - `api/modules/<模块>.yaml`
   - `backend/internal/server/modules/<模块>/`
   - `backend/internal/server/store/migrations/<时间>_m<编号>_*.sql`
   - `backend/internal/agent/<功能包>/`、`backend/pkg/protocol/methods_<模块>.go`（涉及代理时）
   - `web/src/features/<模块>/`、`web/src/api/gen/<模块>.ts`、`web/public/`（仅 M7 的 service worker 和 J 的 PWA 文件）
   - 自己的 `docs/modules/Mx.md`（按实际实现更新）
3. **共享文件只加行**：`backend/internal/server/app/modules.go`、`backend/sqlc.yaml`、`backend/cmd/agent/main.go` 的 `register()` 和 `capabilities()`、`web/src/features/settings/tabs.tsx`、`web/src/app/TopbarActions.tsx`、`web/src/app/GlobalPanels.tsx`、`web/src/lib/i18n.ts`（不要改）、`go.mod`/`go.sum`、`web/package.json`/`package-lock.json`。
4. **不要改基础代码**：`internal/server/{app,auth,audit,secrets,settings,events,scheduler,notify,agenthub,ws,httpx,module,actions,contracts,testutil,core}`、`pkg/rpc`、`web/src/{api,auth,app,components}`。确实需要改时，改动要小、要向后兼容，并在汇报里单独列出理由。contracts 的签名不能改，只能在汇报里提议。
5. **自己验证**：
   ```bash
   cd backend && go generate ./... && gofmt -l . && go vet ./... && go test -race ./... && GOOS=windows GOARCH=amd64 go build ./cmd/agent
   cd web && npm run gen:api && npm run typecheck && npm test && npm run build
   ```
   全部通过才算完成。
6. **提交**：在自己的分支上提交，信息用英文，格式 `feat(<模块>): ...`。可以多个提交。不要推送，不要改别的分支。
7. **汇报**：完成后回复：做了什么、改了哪些共享文件、测试结果（贴命令输出的最后几行）、和文档不一致的地方、没做完或建议后续做的事。

## 任务卡

### A：M5 项目 + M6 备忘（批次 1）

- 规格：`modules/M5.md`、`modules/M6.md`。
- 后端模块 `projects` 和 `notes`，两个独立目录。
- M5 提供 `contracts.Issues` 和 `contracts.IssueSync`；M6 提供 `contracts.Notes`。
- M6 的“转提醒”用 `contracts.Reminders`（B 同批开发，取不到时返回 501，测试里注册一个假实现）。
- 注册文档里列出的动作。
- 前端 `features/projects`、`features/notes`，替换占位页。
- 验收：M5、M6 文档的验收条目。

### B：M7 提醒与通知 + M8 习惯（批次 1）

- 规格：`modules/M7.md`、`modules/M8.md`。
- 后端模块 `reminders`（含渠道、路由、Telegram webhook）和 `habits`。
- M7 提供 `contracts.Reminders`；M8 提供 `contracts.Habits`。
- M8 的 HA 联动用 `contracts.HomeAssistant`（D 同批开发）。
- 前端 `features/reminders`、`features/habits`，设置页加“通知”标签，`web/public/sw.js`。
- 外部服务（Telegram、Bark、Server酱、Web Push）在测试里全部用 httptest 假服务器。

### C：代理 + M2 服务器 + M3 本机（批次 1）

- 规格：`modules/M2-M3.md`、`04-agent-protocol.md`。
- 代理端功能包和协议方法，Linux 和 Windows 两套实现。
- 后端模块 `hosts`，提供 `contracts.Hosts`。
- 前端 `features/servers`、`features/pc`，终端用 xterm.js。
- 验收时本地真实跑一个 Linux 代理连接服务端，确认指标、进程、服务、终端、文件都能用。

### D：M9 Home Assistant（批次 1）

- 规格：`modules/M9.md`。
- 后端模块 `homeassistant`，提供 `contracts.HomeAssistant`（含 `WatchEntity`）。
- 测试用 httptest 实现一个假 HA（REST + WebSocket）。
- 前端 `features/home`，设置页加“Home Assistant”标签。
- 第二步（时间允许）：通过代理访问内网 HA 的 `http.proxy` / `ws.proxy`。

### E：M4 编码任务（批次 2）

- 规格：`modules/M4.md`。依赖 C 的代理、A 的 `contracts.Issues`、H 的 `contracts.GitHub`（同批，取不到时隐藏“建 PR”）。
- 在 M5 的 Issue 详情页加“交给编码助手”按钮（小改动）。
- 测试用假执行器脚本，不调用真实的 claude 或 codex。

### F：M10 运维监控（批次 2）

- 规格：`modules/M10.md`。依赖 C 的 hosts 和代理。
- Docker 部分要改 `features/servers` 的详情标签（小改动）。

### G：M11 日历、早报、番茄钟（批次 2）

- 规格：`modules/M11.md`。
- 早报用到的 contracts 取不到时跳过对应部分。

### H：M13 GitHub + Linear（批次 2）

- 规格：`modules/M13.md`。
- 提供 `contracts.GitHub`。Linear 同步只通过 `contracts.IssueSync`。

### I：M12 AI 助手与自动化（批次 3）

- 规格：`modules/M12.md`。
- 开发前先用 claude-api 技能读 Go SDK 文档。
- 测试用假 Anthropic 服务器。

### J：M1 首页与收尾（批次 3）

- 规格：`modules/M1.md`、`01-product.md` 的全局部分。
- PWA：manifest、图标、安装提示（service worker 与 B 的 `sw.js` 合并）。
- 手机适配检查：所有页面 390px 宽。
- 部署：检查 `deploy/` 下的文件能构建和启动，补 GitHub Actions 的 CI（后端测试、前端测试构建、交叉编译代理）。
- 端到端测试：Playwright 跑主流程（登录、建 Issue、写备忘、建提醒、看服务器、开终端、打卡）。

## 变更记录

跨模块的接口调整记在这里。

| 日期 | 变更 | 原因 |
| --- | --- | --- |
| 2026-09-27 | 新增 `contracts.IssueSync`、`HomeAssistant.WatchEntity` | Linear 同步和 HA 联动需要 |
