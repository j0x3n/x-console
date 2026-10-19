# 交接状态

正在做的一批任务有交接状态时写在这里。批次做完后把内容挪到 `docs/archive/`，这里只留下面的环境提示。

## 2026-10-19 部署交接：B250 到 B254

- 用户已要求推送并部署 B251～B254，B250 随本批上线。部署由 B254 提交的部署标记触发，最终结果以 GitHub Actions 为准。
- B251：`70fa2b6`；B252：`3a94d5a`；B253：`41d5c19`；B254 为本记录所在提交。
- 用户明确负责验证，后续部署不再额外做本地测试、截图或编码 AI 审查。约定已写入 AGENTS.md，现有 CI 保留。
- 快速检查已在新要求到达前通过：后端 build、core 的 vet/test、前端类型和格式检查、相关 67 条测试。浅色云盘和 Git 设置截图脚本完成，不作为用户验收结论。
- Codex 只读审查已按用户要求停止。运行记录 `~/.claude/codex-runs/20261019-220539/`，会话 `01a1547b-75ec-7312-a64c-7634c946dc41`，不需要继续。
- 本次临时启动的 8092 后端与 5175 前端已停止。B254 修复本机字号路径和配对码测试 UTC 时间，旧环境提示见下文历史记录。
- B216 按用户要求跳过登录保持验证，仍待开发。接下来先读 `docs/tasks.md`、`docs/specs/B216.md`，再读相关开发约定。不要把 B216 记成已经实现。
- 下文 B248 未部署的描述属于旧记录，B248 已随 B249 的成功部署上线。

## 2026-10-18 这批：X Console 看板 XC-56 到 XC-66

### 已经上线

- 30845e75 XC-58、6506fac0 XC-61/62/63/65、d88fb93f B247（邮件）已部署（8c8d5788 带部署标记，全部检查通过）。
- 看板上 XC-56、58、61、62、63、64、65、66 已挪到“待审核”，每张都写了评论。

### 已推送到 develop、没部署

- 3a2fb76d `修复: XC-63 添加卡片的输入框跟在最后一张卡片后面`。2026-10-19 在本地看过 1360 和 390 宽的看板，输入框跟在最后一张卡片后面。
- 7a6acef5 `B248`（看板 XC-59）：设置 → Git 只留账号和通知，“管理仓库”弹窗搬到仓库页，仓库页能运行工作流、重跑失败的任务。后端 Codex 写（运行记录 `~/.claude/codex-runs/20261018-234140/`），Claude 审查。检查记录见 [qa/B248-repo-actions.md](qa/B248-repo-actions.md)。
- 2026-10-19 已在 X Console 看板给 XC-63 补了更正评论，XC-59 写了评论并挪到“待审核”。各多发了一条重复评论（XC-63 的 16 号、XC-59 的 17 号），MCP 删不了，请用户在页面上删。
- 往看板写评论：用 `~/.claude.json` 里 xconsole 的 MCP 地址和请求头，在同一个 Python 进程里调 `projects_comment`。不要把中文放进命令行参数再开子进程，Windows 上读回来会看着像乱码。
- 部署后先拿一个不会部署线上的工作流试运行，确认令牌权限不够时的提示。

### 2026-10-19 已部署

- ccd01c8e B249（看板 XC-60，带部署标记，全部检查和端到端通过）切换模块时保留正在做的事：终端切走时问要不要留，顶栏“后台”按钮，`useDraft`、`useKeptState`。后端偏好字段 Codex 写（运行记录 `~/.claude/codex-runs/20261019-010638/`），Claude 审查；前端 Claude 写。检查记录见 [qa/B249-keep-state.md](qa/B249-keep-state.md)。
  - 还没做：看板 XC-60 写评论、挪到“待审核”；真实机器上试终端留在后台（检查记录“没验证的”一节）。
  - 关键文件：`web/src/features/servers/terminal/`（`store.ts` 后台列表，`sessions.ts` xterm 和连接，`TerminalDock.tsx` 隐藏容器），`features/servers/BackgroundButton.tsx`，`components/TerminalTab.tsx`，`web/src/hooks/useDraft.ts`。

### 等用户定 / 下一步

- XC-57 左栏合并已完成，任务 B250。对应本次 B250 提交，推送到 develop，不部署。
  - 11 个入口和两条默认分隔线。设备、个人、运维使用独立入口名和图标。个人顶部功能两列，其余单列。
  - 旧固定项映射、去重，保留原顺序。不主动改后端保存值。启动台的旧固定项排序和成员名搜索已验证。
  - 修复合并入口延迟注册角标时的 Hook 数量错误。快速检查及前端全量 1126 条测试通过。
  - 已检查习惯、设备、日程在 1360px 和 390px 的深浅主题，以及手机抽屉和启动台。记录见 [qa/B250-nav-merge.md](qa/B250-nav-merge.md)。完整端到端留给 CI。
  - 用户指定 GPT-6.1-sol high 全面替代 Claude Opus 5.5，开发和审查按新规则执行。已更新 AGENTS.md。

### 这次发现的环境问题

- 本机 Git 开了 `autocrlf=true`，工作区文件是 CRLF。`check-quick.sh` 的 gofmt 和 prettier 会把没改的文件也报出来。查提交内容用 `git show HEAD:<文件> | gofmt -l`。
- 本机 PATH 里没有 node，用 `/c/Users/xcc19/AppData/Roaming/fnm/node-versions/v22.23.3/installation`。Node 25 有 localStorage 问题，用 22。
- 本机没装 B242 新加的依赖 `modern-screenshot`，`tsc` 会报这一条，和别的改动无关。跑前端检查前先 `npm ci`。
- 8080 和 5173 端口被别的程序占着（xs-server、X Studio 的 vite），本地前端用 5174。
- `backend/internal/server/core` 的 `TestInstallRejectsBadCodes` 在本机必失败，改动前就这样：测试往 SQLite 写的是本地时间（东八区），过期的配对码被当成没过期。CI 在 UTC 下能过。`check-quick.sh` 会停在这一步，前端那几步要手动跑。
- Git Bash 里跑 `npm run shots -- --only /servers` 要加 `MSYS_NO_PATHCONV=1`，不然路径会被改写。
- 浏览器面板的启动配置里 `bash` 指向 WSL，要写成 `C:/Program Files/Git/bin/bash.exe`。

## 接手时注意

- 装代码生成工具：见 AGENTS.md“常用命令”第一段。
- 后端需要 Go 1.26（`go.mod` 要求，本地旧版本会自动下载工具链）。
- 本地的 Chromium 和 playwright-core 版本不一致时，截图和端到端测试都加 `XC_SHOTS_BROWSER=/opt/pw-browsers/chromium`。
- 认证中间件只在 `/api/v1/mcp` 上看 `Authorization: Bearer`。代理连 `/agent/connect` 也用 Bearer，改中间件时别弄坏（B43 第一次写错过，全量测试里代理全连不上）。
- 截图要先起服务端：`XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1 XC_DATA_DIR=<临时目录> XC_ADDR=127.0.0.1:8090 XC_WEB_DIR=<仓库>/web/dist go run ./cmd/server`，再 `npm run shots -- --base http://127.0.0.1:8090`。
- 这个仓库有时是浅克隆，推送前先 `git fetch --unshallow origin develop`，看清远端用到了哪个任务编号，免得撞号（这次 B158 到 B160、B221 都撞过）。
