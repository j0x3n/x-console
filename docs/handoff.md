# 交接状态

正在做的一批任务有交接状态时写在这里。批次做完后把内容挪到 `docs/archive/`，这里只留下面的环境提示。

## 路由器批次 B227 到 B232（2026-10-15 开始）

规格：[B227-B232](specs/B227-B232.md)。分工：前端（`web/` 下）Opus 5.5 写，其他由 Sonnet 5.5 子代理写（用户 2026-10-15 让在云端直接做，按 AGENTS.md 分派）。

做法：

- 顺序是 B227、B228、B232、B229、B230、B231。每个任务先做后端，推到 `develop` 后再做前端。B231 只有前端。
- 后端每个任务先改 `api/modules/router.yaml` 并生成代码，这是前后端的约定。接口和规格不一样时，先在规格里改，写清楚原因。
- 迁移只开一个：`m58`，B228、B229、B230、B232 的表都放进去。B228 的后端提交一起建好 B229、B230 要用的表。
- 上报脚本 `modules/router/report.sh` 每个任务都会改。新段面板认不出时要忽略，老脚本要能照常上报。
- 后端提交信息写“B228: 后端……”，前端写“B228: 前端……”。
- 后端写完后，前端开始之前 Opus 5.5 先按 AGENTS.md“验收”一节审查一遍，有问题记在下面，交回后端改。

进度：

| 任务 | 后端（Sonnet 5.5） | 前端（Opus 5.5） |
| --- | --- | --- |
| B227 | 没开始 | 没开始 |
| B228 | 没开始 | 没开始 |
| B232 | 没开始 | 没开始 |
| B229 | 没开始 | 没开始 |
| B230 | 没开始 | 没开始 |
| B231 | 不用 | 没开始 |

审查记录：还没有。

## 接手时注意

- 装代码生成工具：见 AGENTS.md“常用命令”第一段。
- 后端需要 Go 1.26（`go.mod` 要求，本地旧版本会自动下载工具链）。
- 本地的 Chromium 和 playwright-core 版本不一致时，截图和端到端测试都加 `XC_SHOTS_BROWSER=/opt/pw-browsers/chromium`。
- 认证中间件只在 `/api/v1/mcp` 上看 `Authorization: Bearer`。代理连 `/agent/connect` 也用 Bearer，改中间件时别弄坏（B43 第一次写错过，全量测试里代理全连不上）。
- 截图要先起服务端：`XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1 XC_DATA_DIR=<临时目录> XC_ADDR=127.0.0.1:8090 XC_WEB_DIR=<仓库>/web/dist go run ./cmd/server`，再 `npm run shots -- --base http://127.0.0.1:8090`。
- 这个仓库有时是浅克隆，推送前先 `git fetch --unshallow origin develop`，看清远端用到了哪个任务编号，免得撞号（这次 B158 到 B160、B221 都撞过）。
