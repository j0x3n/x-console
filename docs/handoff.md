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

## 当前（2026-10-01）

**部署**：`92acccf` 那次已经成功。运行记录 https://github.com/j0x3n/x-console/actions/runs/36851153881 。前端、后端、镜像、两个代理、部署都过了。请用户在 iPhone 上测 B45（`?safari=A` 到 `F`、`all`、`off`）。

**部署提速三件事都做了，并且已经再部署过一次。** 运行记录 https://github.com/j0x3n/x-console/actions/runs/36856004369 。整次 4 分 31 秒（上次 `92acccf` 是 10 分 22 秒）。最慢的后端测试组 3 分 19 秒，另外两组 2 分 55 秒和 2 分 9 秒。检查任务 1 分钟。现在整次在等前端测试（3 分 55 秒），不再等后端。

1. `ci.yml` 的 `workflow_call` 加了输入 `race`（布尔，默认 true）。PR 和手动运行不传这个值，测试仍然带 `-race`。`deploy.yml` 的 `test` 任务传 `race: false`。
2. 后端拆成两个任务。`backend-check` 只跑一次：生成代码、gofmt、vet、shellcheck、本机代理脚本、Windows 编译。`backend-test` 三组同时跑。第一组是 drive、hosts、ai、github、monitoring、homeassistant、reminders。第二组是 app、backup、coding、notes、projects、store。第三组用 `go list` 取剩下的包，新包不会漏。分组按这次带 `-race` 的包用时摊开。
3. 快照复制已经在 `5450571`。Linux 容器里 `go test -race ./...` 通过，用时 3 分 33 秒，含第一次编译。改前本机全量带 `-race` 约 3 分 40 秒。`docs/tasks.md` 接口变更记录已写上 `store.Snapshot`、`store.OpenSnapshot`。直接 `store.Open(":memory:")` 的测试没改。
4. `AGENTS.md`“小修”一节的用时已改成这次的实测。

**之前约好的检查**：send_later 触发器 `trig_01DNcSTU2wAcFNEuTbAiwUE9` 会在 11:02 UTC 发消息回原会话。部署结果已经在上面，换了会话就忽略它。

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
