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

## 当前（2026-10-01 额度用完时的状态）

**部署**：`92acccf` 带部署标记推送了，运行记录 https://github.com/j0x3n/x-console/actions/runs/36851153881 。前端测试和镜像已过，后端测试还在跑。接手后先看它成没成功；失败就看日志修好，再带部署标记推送到 `develop`。成功后让用户在 iPhone 上测 B45（`?safari=A` 到 `F`、`all`、`off`）。

**用户要求“都改”：让部署更快，三件事都做。**

1. 部署时后端测试不带 `-race`，PR 检查里保留。改法：`.github/workflows/ci.yml` 加 `workflow_call` 输入 `race`（布尔，默认 true），`go test` 那步按它决定加不加 `-race`；`deploy.yml` 的 `test` 任务传 `race: false`。还没做。
2. 后端测试拆成 2 到 3 个任务同时跑（matrix 按包分组）。生成代码检查、gofmt、vet、shellcheck、Windows 编译放进单独一个任务，只跑一次。还没做。参考数据（本地 4 核、不带 race）：drive 21 秒、hosts 13、ai 13、app 12、coding 11、notes 10、projects 9，其余都在 7 秒以下，全部加起来约 180 秒。
3. 测试提速：已经写了，但**还没提交、没跑完测试**。
   - 原因：带 `-race` 时每次新建测试环境要 3.6 秒，其中建表（跑迁移）占 2.8 到 3.2 秒；不带 race 时一共只要 0.46 秒。
   - 改法：`store` 加 `Snapshot`、`OpenSnapshot`（用 modernc sqlite 的 Serialize/Deserialize）。`testutil.openDB` 每个测试进程只迁移一次，之后的测试都从这份快照复制。`store_test.go` 加了 `TestSnapshotCopiesAMigratedDatabase`，已经通过。
   - 接手要做：跑 `go test -race ./...` 全量确认通过，比较改前改后的用时（改前本地全量带 race 约 3 分 40 秒）。然后提交，提交信息以“C”开头记成清理任务，并在 `docs/tasks.md`“接口变更记录”里记下 `store.Snapshot`、`store.OpenSnapshot`。
   - 别的包里还有直接调 `store.Open(":memory:")` 的测试（`app`、`settings`、`projects/due_test.go`），数量少，可以不改。
4. 三件都做完后，把 AGENTS.md“小修”一节里的测试用时更新成新的实测值。

**之前约好的检查**：send_later 触发器 `trig_01DNcSTU2wAcFNEuTbAiwUE9` 会在 11:02 UTC 发消息回原会话检查部署结果。换了会话的话忽略它。

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
