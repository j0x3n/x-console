# 交接状态

正在做的一批任务有交接状态时写在这里。批次做完后把内容挪到 `docs/archive/`，这里只留下面的环境提示。

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

- XC-57 左栏合并：2026-10-19 用户已确认方案和样式，还没写代码，下一个会话直接开工。纯前端，Claude Opus 5.5 写。先在 `docs/tasks.md` 加一条任务（编号顺延，推送前 `git fetch` 看远端用到哪个号），再写 `docs/specs/` 的规格。
  - 分组（用现有的 `navMerges`，B179，地址都不变）：
    - 工作：今日、项目、Agent（`/coding`，合并仓库 `/github`）。
    - 笔记（`/notes`：笔记、稍后读、复习卡片、资讯 `/feeds`）、问我的数据、邮件、日程（`/calendar`：日程和专注、提醒事项、今日计划、收集箱）、音乐、云盘。
    - 设备（`/servers`：服务器、电脑、路由器、智能家居、屏幕时间）。现在的“家”（`/home` + `/router`）拆掉，`navEntryLabels` 里的 `Home and network` 去掉。
    - 个人（`/habits`：习惯和健身、目标、状态打卡、自我实验、技能、证件档案、联系人、菜谱、预测）。
    - 运维（`/monitoring`：监控、密钥、自动化）。
  - 已固定的旧入口：换成它所在的新入口（`entryOf`），保留用户原来的顺序，重复的去掉。比如 `/reminders` 换成 `/calendar`，`/router` 换成 `/servers`。在 `navEntries.ts` 的 `railItems`、`railEntries`、`togglePin`、`isPinned` 里统一处理，不改后端存的值。
  - `defaultNavPins` 改成全部 11 个入口，用分隔线（`/-/1`、`/-/2`）分成三段：项目、Agent | 笔记、问我的数据、邮件、日程、音乐、云盘 | 设备、个人、运维。
  - 样式（用户选了方案 B）：
    - 设备、个人、运维是新入口，入口名写进 `navEntryLabels`（Devices、Personal、Ops，中文 设备、个人、运维，注意 `Personal` 已是启动台分组的键，中文同样是“个人”，不冲突）。各配一个新图标，需要在 `nav.ts` 加一个 `navEntryIcons`，`buildEntries` 用它。
    - 二级菜单标题栏写入口名。顶部列出组里的功能，横线下面是当前功能自己的菜单（现有 `MergedNavPanel`，`Sidebar.tsx` 约 340 行）。
    - 组里超过 5 项时，顶部的功能排成两列，行高 28px，字号 12px（只有个人组会这样）。不超过 5 项的照旧竖排，行高 34px。样式加在 `web/src/styles/nav.css` 的 `.nav-merge` 附近。
    - 组里功能的顺序跟 `navItems` 一样，`MergedNavPanel` 按 `navItems` 过滤。要按上面的顺序排就调 `navItems` 的顺序（比如资讯挪到复习卡片后面，云盘不再跟在服务器下面）。
    - 启动台（`Launcher.tsx`）分成工作、个人、设备三组，`integrations` 组没有入口了就不显示（现在空组本来就会被过滤，确认一下）。各入口的 `group` 取入口自己那一行的 `group`，运维和云盘要改成 `machines`，设备入口是 `machines`。在启动台搜组里的功能名（比如“菜谱”）也要能找到它所在的入口，要看 `Launcher.tsx` 现在是不是只搜入口名。
    - 手机上抽屉内容一样，截图看 390px 宽时两列放不放得下。
  - 要改的测试：`web/src/components/layout/navEntries.test.ts` 第一条写死了“合并三组”和 `Home and network`，要按新分组重写，再补旧固定入口换成新入口的用例。`Launcher.test.tsx` 也看一下。
  - 文档：`docs/07-design.md` 第 29 行左栏那段写的是“笔记、习惯、家”三组和“云盘在设备组排在服务器下面”，要一起改。
  - 截图自查：`npm run shots -- --only /habits`、`/servers`、`/calendar`，看 1360 和 390。

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
