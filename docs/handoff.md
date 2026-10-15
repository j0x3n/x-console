# 交接状态

正在做的一批任务有交接状态时写在这里。批次做完后把内容挪到 `docs/archive/`，这里只留下面的环境提示。

## 路由器批次 B227 到 B232（2026-10-15 开始）

规格：[B227-B232](specs/B227-B232.md)。用户定的分工：前端（`web/` 下）Claude 写，其他 Codex 写。

做法：

- 顺序是 B227、B228、B232、B229、B230、B231。每个任务先做后端，推到 `develop` 后再做前端。B231 只有前端。
- Codex 每个任务先改 `api/modules/router.yaml` 并生成代码，这是前后端的约定。接口和规格不一样时，先在规格里改，写清楚原因。
- 迁移只开一个：`m57`，B228、B229、B230、B232 的表都放进去。B228 的后端提交建表，后面的任务要加列或加表时再开新迁移，不改已推送的迁移。
- 上报脚本 `modules/router/report.sh` 每个任务都会改。新段面板认不出时要忽略，老脚本要能照常上报。
- 后端提交信息写“B228: 后端……”，前端写“B228: 前端……”。
- Codex 写的后端推送后，Claude 写前端之前先按 AGENTS.md“验收”一节审查一遍，有问题记在下面，交回 Codex 改。

进度：

| 任务 | 后端（Codex） | 前端（Claude） |
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
