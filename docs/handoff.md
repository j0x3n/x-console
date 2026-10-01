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
| B47 | 完成 | 6 步都做完了，没做的部分记在 `docs/tasks.md`“已知问题” |

## 当前

B41 到 B48 这一批的代码都写完了，提交在 `develop`。剩下的：

- B45 第三步：用户说“部署”后带部署标记推送，用户在 iPhone 上用 `?safari=A` 到 `F` 测，按结果定稿。
- 整批还没部署过。部署前跑一遍完整检查（AGENTS.md“常用命令”）。

B47 的设计取舍（和规格不同的地方）：
- 构建产物存在 coding 模块自己的文件存储里（`artifacts/<任务 id>/`），在任务详情里下载，不进云盘的目录。云盘的目录有自己的数据库索引，要写进去得加一个云盘接口，这次没做。
- 构建只能在“等你决定”（review）状态跑，提交以后工作目录就删了。
- 任务开始前的 fetch 在派发任务时同步做（最多 3 分钟），这期间别的任务不会被派发。以后再改成异步。
- 内置 Agent 的模型存成 `providerId:modelId`，ai 模块通过 context 覆盖 B32 的模型选择。
- “只编译”任务（不经过 Agent 直接构建某个分支）这次不做，`coding_tasks.executor` 有 CHECK 约束，改它要重建表。记到“已知问题”。


## 接手时注意

- 装代码生成工具：见 AGENTS.md“常用命令”第一段。
- 后端需要 Go 1.26（`go.mod` 要求，本地旧版本会自动下载工具链）。
- 本地的 Chromium 和 playwright-core 版本不一致时，截图和端到端测试都加 `XC_SHOTS_BROWSER=/opt/pw-browsers/chromium`。
- 认证中间件只在 `/api/v1/mcp` 上看 `Authorization: Bearer`。代理连 `/agent/connect` 也用 Bearer，改中间件时别弄坏（B43 第一次写错过，全量测试里代理全连不上）。
- 截图要先起服务端：`XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1 XC_DATA_DIR=<临时目录> XC_ADDR=127.0.0.1:8090 XC_WEB_DIR=<仓库>/web/dist go run ./cmd/server`，再 `npm run shots -- --base http://127.0.0.1:8090`。
