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
| B43 | 后端完成，前端进行中 | 后端：迁移 `m0_b43_api_tokens`、`auth/tokens.go`、`actions.AllowedFor`、新模块 `modules/mcp`（令牌接口和 `POST /api/v1/mcp`）。前端要做：设置里的“远程访问”页签 |
| B47 | 未开始 | |

## 当前

B43：后端已提交。注意：认证中间件只在 `/api/v1/mcp` 上看 `Authorization: Bearer`，别的路径不能看，代理连 `/agent/connect` 也用 Bearer（第一次写错过，全量测试里代理全连不上）。
下一步：前端设置页“远程访问”：令牌列表、新建（权限、模块、有效期，建好后只显示一次）、吊销、接入说明（Claude Code、Codex、Cursor）、最近调用。
B45 等用户说“部署”后带部署标记推送，用户真机测试。

## 接手时注意

- 装代码生成工具：见 AGENTS.md“常用命令”第一段。
- 后端需要 Go 1.26（`go.mod` 要求，本地旧版本会自动下载工具链）。
- 本地的 Chromium 和 playwright-core 版本不一致时，截图和端到端测试都加 `XC_SHOTS_BROWSER=/opt/pw-browsers/chromium`。
- 截图要先起服务端：`XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1 XC_DATA_DIR=<临时目录> XC_ADDR=127.0.0.1:8090 XC_WEB_DIR=<仓库>/web/dist go run ./cmd/server`，再 `npm run shots -- --base http://127.0.0.1:8090`。
