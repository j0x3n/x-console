# 交接状态

正在做的一批任务有交接状态时写在这里。批次做完后把内容挪到 `docs/archive/`，这里只留下面的环境提示。

## 2026-10-18 这批：X Console 看板 XC-56 到 XC-66

### 已经上线

- 30845e75 XC-58、6506fac0 XC-61/62/63/65、d88fb93f B247（邮件）已部署（8c8d5788 带部署标记，全部检查通过）。
- 看板上 XC-56、58、61、62、63、64、65、66 已挪到“待审核”，每张都写了评论。

### 已推送到 develop、没部署

- 3a2fb76d `修复: XC-63 添加卡片的输入框跟在最后一张卡片后面`。2026-10-19 在本地看过 1360 和 390 宽的看板，输入框跟在最后一张卡片后面。
- 7a6acef5 `B248`（看板 XC-59）：设置 → Git 只留账号和通知，“管理仓库”弹窗搬到仓库页，仓库页能运行工作流、重跑失败的任务。后端 Codex 写（运行记录 `~/.claude/codex-runs/20261018-234140/`），Claude 审查。检查记录见 [qa/B248-repo-actions.md](qa/B248-repo-actions.md)。
- 还没做：在 X Console 看板给 XC-63 补一条评论（上一条猜错了，说的是描述编辑框），XC-59 写评论并挪到“待审核”。要用户同意再发。
- 部署后先拿一个不会部署线上的工作流试运行，确认令牌权限不够时的提示。

### 等用户定

- B249（规格 [B249](specs/B249.md)，看板 XC-60）：用户同意先做“保存状态”，不做多标签页。终端切走不断开最多留几个、多久自动断开（建议 4 个、30 分钟），要用户定了再做。
- XC-57 左栏合并方案，下面是给用户的建议，还没和用户确认，也没写代码。用现有的 `navMerges`（`web/src/app/nav.ts`，B179）合并，地址都不变：
  - 工作：今日、项目、Agent（合并仓库）。
  - 笔记（笔记、稍后读、复习卡片、资讯）、问我的数据、邮件、日程（日程和专注、提醒事项、今日计划、收集箱）、音乐、云盘。
  - 设备（服务器、电脑、路由器、智能家居、屏幕时间）。
  - 个人（习惯和健身、目标、状态打卡、自我实验、技能、证件档案、联系人、菜谱、预测）。
  - 运维（监控、密钥、自动化）。
  - 要一起改 `navEntryLabels`、`defaultNavPins`，并确认用户已固定的旧入口（比如 `/router`）能映射到新入口。

### 这次发现的环境问题

- 本机 Git 开了 `autocrlf=true`，工作区文件是 CRLF。`check-quick.sh` 的 gofmt 和 prettier 会把没改的文件也报出来。查提交内容用 `git show HEAD:<文件> | gofmt -l`。
- 本机 PATH 里没有 node，用 `/c/Users/xcc19/AppData/Roaming/fnm/node-versions/v22.23.3/installation`。Node 25 有 localStorage 问题，用 22。
- 本机没装 B242 新加的依赖 `modern-screenshot`，`tsc` 会报这一条，和别的改动无关。跑前端检查前先 `npm ci`。
- 8080 和 5173 端口被别的程序占着（xs-server、X Studio 的 vite），本地前端用 5174。

## 接手时注意

- 装代码生成工具：见 AGENTS.md“常用命令”第一段。
- 后端需要 Go 1.26（`go.mod` 要求，本地旧版本会自动下载工具链）。
- 本地的 Chromium 和 playwright-core 版本不一致时，截图和端到端测试都加 `XC_SHOTS_BROWSER=/opt/pw-browsers/chromium`。
- 认证中间件只在 `/api/v1/mcp` 上看 `Authorization: Bearer`。代理连 `/agent/connect` 也用 Bearer，改中间件时别弄坏（B43 第一次写错过，全量测试里代理全连不上）。
- 截图要先起服务端：`XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1 XC_DATA_DIR=<临时目录> XC_ADDR=127.0.0.1:8090 XC_WEB_DIR=<仓库>/web/dist go run ./cmd/server`，再 `npm run shots -- --base http://127.0.0.1:8090`。
- 这个仓库有时是浅克隆，推送前先 `git fetch --unshallow origin develop`，看清远端用到了哪个任务编号，免得撞号（这次 B158 到 B160、B221 都撞过）。
