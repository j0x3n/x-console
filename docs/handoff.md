# 交接状态（B41 到 B48 这一批）

用户 2026-09-30 让 Claude 直接写 B41 到 B48 的代码，并要求实时更新这个文件。额度用完或会话中断时，接手的 AI 先读这个文件，从“当前”那一步继续。

## 规则

- 分支：`develop`（用户指定 Claude 在这个分支上开发）。
- 提交信息以任务编号开头。为了随时能交接，一个任务可能有多个提交，都以同一个编号开头。要撤掉某个任务：`git log --oneline --grep '^B41'` 找出全部提交，逐个 `git revert`。
- 每完成一个小步骤就提交并推送，同时更新下面的进度。
- 小步骤只跑 `scripts/check-quick.sh`。每个任务做完跑一次完整检查（见 AGENTS.md 常用命令）。
- 没有用户说“部署”，提交信息里不带部署标记。

## 顺序

B41 → B45 → B44 → B42 → B48 → B46 → B43 → B47（原因见 `docs/tasks.md` 的“任务说明”）。

## 进度

| 任务 | 状态 | 说明 |
| --- | --- | --- |
| B41 | 完成 | 全部检查、端到端、截图都过了 |
| B45 | 第一步完成，等用户真机测试 | 开关 `?safari=all`、`A` 到 `F`、`off`。用户测完把结果发回来后做第三步（定稿） |
| B44 | 完成 | 点击区域 44px 没做，记在“已知问题” |
| B42 | 完成 | 设置 → AI 用量；后端 `modules/ai/usage.go` |
| B48 | 完成 | 设置 → 安全；`auth.RequireStrictElevated` 给 B43、B47 用 |
| B46 | 完成 | 没做完的几项记在 `docs/tasks.md`“已知问题”里 |
| B43 | 完成 | 设置 → 远程访问；后端 `auth/tokens.go`、`modules/mcp`。网页版 AI 的 OAuth 没做，记在“已知问题” |
| B47 | 进行中 | 见下面“当前” |

## 当前

B47 进行中，分 6 步，每步一个提交（都以 `B47：` 开头）：

1. （已完成）后端：迁移 `m4_b47_ai_agents`（`ai_agents`、`git_connections`，`coding_repos`、`coding_tasks`、`issue_comments` 加列），新模块 `modules/aiagents`（Agent 增删改、Git 连接增删改和检查、列远端仓库），`contracts.GitConnections`。
2. （已完成）仓库按远端登记：`coding` 模块按连接登记仓库，代理新方法 `coding.ensure_repo`（clone 或 fetch，令牌用 `GIT_ASKPASS` 传，不落盘），Forgejo 开 PR，任务带 Agent（固定说明、模型、权限）。
3. 构建：仓库的构建步骤，代理新方法 `coding.build`，改完自动构建、失败带日志重试，产物用 `files.read` 拉回存云盘。
4. 看板：卡片分配给 Agent（建任务、加成员、开始和结束写评论、移列表），Git 回调（PR 合并后卡片到“已完成”），内置 Agent（ai 模块提供 `contracts.ToolRunner`，按 B43 的 `actions.AllowedFor` 过滤工具）。
5. 前端：左栏“Agent”，Agent 列表和表单、Git 连接、仓库按远端登记、任务详情的构建日志和产物、卡片上的“分配给 Agent”。
6. 端到端和截图，更新文档。

设计取舍（和规格不同的地方）：
- 任务开始前的 fetch 在派发任务时同步做（最多 3 分钟），这期间别的任务不会被派发。以后再改成异步。
- 内置 Agent 的模型存成 `providerId:modelId`，ai 模块通过 context 覆盖 B32 的模型选择。
- “只编译”任务（不经过 Agent 直接构建某个分支）这次不做，`coding_tasks.executor` 有 CHECK 约束，改它要重建表。记到“已知问题”。

B45 等用户说“部署”后带部署标记推送，用户真机测试。

## 接手时注意

- 装代码生成工具：见 AGENTS.md“常用命令”第一段。
- 后端需要 Go 1.26（`go.mod` 要求，本地旧版本会自动下载工具链）。
- 本地的 Chromium 和 playwright-core 版本不一致时，截图和端到端测试都加 `XC_SHOTS_BROWSER=/opt/pw-browsers/chromium`。
- 认证中间件只在 `/api/v1/mcp` 上看 `Authorization: Bearer`。代理连 `/agent/connect` 也用 Bearer，改中间件时别弄坏（B43 第一次写错过，全量测试里代理全连不上）。
- 截图要先起服务端：`XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1 XC_DATA_DIR=<临时目录> XC_ADDR=127.0.0.1:8090 XC_WEB_DIR=<仓库>/web/dist go run ./cmd/server`，再 `npm run shots -- --base http://127.0.0.1:8090`。
