# 交接状态

额度用完或会话中断时，接手的 AI 先读这个文件，从“下一步”继续。

## 第三批（B70 到 B83，2026-10-01 开始）— 接手的 AI 先看这里

用户 2026-10-01 提了一批新需求。规格在 `docs/specs/B70.md` 到 `B83.md`（B77 写在 `docs/tasks.md`）。
Claude 写了规格和一部分前端，额度用完。现在 GPT 先写后端，前端等 Claude 回来接着做。

### 进度

| 任务 | 前端 | 后端 |
| --- | --- | --- |
| B77 看板卡片 | 完成（`e416f10`） | 不需要 |
| B76 左栏数量、二级菜单选中 | 完成（`dedcd0f`、`216386b`） | 不需要 |
| B70 仓库页 | 完成（`347a4ed`），用假数据截图看过 1360 和 390 | 待做，占位在 `modules/github/pending.go` |
| B71 仓库通知 | 完成（`347a4ed`） | 待做，同上 |
| B72 笔记 | 滚动修好（`6b3dc4d`）、左栏“+”（`a813007`）；外链分享、浮窗、双击编辑的代码写完了，**没测、没截图**（见下面“最后一个提交”） | 完成（`9ff2c08`）；分享、安全校验、附件与缩略图、访问统计已测试 |
| B73 便签 | 代码写完了，**没测、没截图** | 完成（Codex，本次提交）；kind、检索、标签和数量统计、动作与 MCP 已测试 |
| B74 编辑器 | 只做了背景色（`ColorPicker`、`noteColors.ts`、`notes.css` 末尾）。Markdown 的代码块折叠复制、视频音频、隐藏块、提示块、高亮、表格、工具条“插入”菜单都**没做** | 待做（`color` 字段） |
| B75 云盘分享页 | 没开始 | 待做 |
| B78 CI 提速 | 没开始 | 不需要 |
| B79 版本号和维护页 | 没开始 | 待做 |
| B80 保留位置 | 没开始 | 不需要 |
| B81 增量备份 | 没开始 | 待做 |
| B82 服务器信息 | 没开始 | 待做 |
| B83 健康提醒 | 没开始 | 待做 |

### 分工（用户 2026-10-01 定）

**GPT 只写后端，不改前端。** 前端（`web/src/` 下除了 `api/gen/` 生成文件以外的所有文件、`web/scripts/`）留给 Claude 下次接着做。下面“最后一个提交”一节是给 Claude 的，GPT 不用管。

GPT 的做法：

1. 顺序：B72、B73、B74（笔记，前端已经在等）→ B70、B71（仓库）→ B75 → B79 → B82 → B83 → B81。B76、B77、B78、B80 不用后端。
2. B70 到 B73 的接口定义已经写好，生成代码已提交，后端占位在 `modules/github/pending.go`、`modules/notes/pending.go`。实现一个接口就从 `pending.go` 删掉一个，全做完删文件。**不要改这几个 yaml 里已有的字段名**，前端按它写好了。确实要改时，在这个文件里写清楚。
3. B74 到 B83 的接口还没写进 yaml。照规格里“契约”一节写 yaml，跑 `go generate ./...`（后端）和 `cd web && npm run gen:api`（只更新 `web/src/api/gen/`，这是生成文件，可以提交）。前端还没做的接口不用写占位，直接实现。
4. 每个任务一个提交，提交信息以编号开头。做完一个在下面的进度表里把“后端”改成“完成”，写上提交号；和规格不同的地方写在规格开头。
5. 每个任务跑 AGENTS.md 里的后端检查（`go generate`、`gofmt`、`go vet`、`go test -race` 相关的包、Windows 交叉编译）。端到端测试 `web/scripts/e2e.mjs` 要补主流程时，只加接口调用（`api(...)`、`send(...)`）的检查，不加点界面的步骤。
6. 不要带部署标记。

### 最后一个提交（B72、B73 进行中）要先做的（给 Claude）

用户额度用完，最后一个提交是直接提交的，只跑过 `tsc --noEmit`（通过）。接手后：

1. **补中文词条**：`web/src/features/notes/i18n.ts` 还没加这次新的键，界面上会显示英文。缺的键：Shared by link、Share by link、Share note、Link created、Could not copy. Copy it by hand.、Stop sharing this note?、The link stops working right away.、Stop sharing、Sharing stopped、Note sharing、Password set、No password、Opened times、Last opened、Change how long it works、Require a password、Leave empty to keep the old password、4 to 32 characters、People need this password to open the link. Send it separately.、The link shows only this note: the title, the text and its images. Hidden notes cannot be shared.、Shared with X Console、This note needs a password、Open in a floating window、Open in notes、Minimize、Restore window、Memos、Memo、All memos、No memos with this tag、No memos yet、Quick notes from the top bar are saved here.、Others、Take a memo…、Delete this memo?、Saved to memos、Write it down. It is saved as a memo.、Turned into a memo、Turned into a note、Turn into a note、Turn into a memo、Background、Teal、Purple、Pink、Brown、Gray。另外 `ShareDialog.tsx` 里 `t("Opened")} {current.visits} {t("times")` 改成 `t("Opened times")} {current.visits}`，`FloatingWindows.tsx` 的 `t("Restore")` 改成 `t("Restore window")`（“Restore”已经是“恢复”）。中文词典全局唯一，加完跑 `npx vitest run src/lib/i18n.test.ts`。
2. 跑 `scripts/check-quick.sh` 和笔记的单元测试（`NotesPage.test.tsx` 可能要按新的左栏“便签”项改）。
3. 起服务端和前端，打开笔记页看：左栏“便签”、`/notes?view=memos` 瀑布流（后端没上线时显示“还没上线”）、编辑区右上角“在浮窗中打开”、更多菜单里“分享外链”“转成便签”“背景色”、阅读模式双击进入编辑、`/n/<token>` 分享页（后端没上线时显示“服务端还没上线”）。手机 390px 也看一遍。
4. 新文件：`features/notes/{MemoBoard,FloatingNotes,FloatingWindows,ColorPicker}.tsx`、`floating.ts`、`noteColors.ts`、`components/ShareDialog.tsx`、`share/NoteSharePage.tsx`；`app/App.tsx` 加了 `/n/<token>`，`app/GlobalPanels.tsx` 加了 `FloatingNotes`。这两处基础文件的改动还没记到 `docs/tasks.md` 的“接口变更记录”。
5. 已知的小问题：便签视图在 1180px 以下的布局只写了 CSS，没看过效果。

### 后端（GPT）

照各规格的“后端（待做）”做。接口定义已经改好、生成代码已提交的有：B70、B71（`github.yaml`）、B72、B73（`notes.yaml`）。B74 到 B83 的接口还没写进 yaml，按规格里“契约”一节先写 yaml，再 `go generate ./...` 和 `npm run gen:api`。

### 本地调试的方法

- 截图和交互用 Playwright：`/opt/pw-browsers/chromium-1194/chrome-linux/chrome`。没有 GitHub 账号时用 `page.route` 拦截 `/api/v1/github/**` 喂假数据看仓库页。
- 发现的后端小问题：`/mail/summary` 没有邮箱时 `accounts`、`latest` 返回 `null`（前端已兼容，`d5577e3`），后端应该返回空数组。

## 上一批的收尾（2026-10-01 更新）

B58、B53、B69 三个都做完了，都在 `develop` 上，还没部署。上线前要用户用真账号试一下：和风天气的 key、Gmail 应用专用密码、阿里企业邮箱。三个的“和规格不同的地方”都写在各自规格里。

**B69 提交时没跑完检查（用户额度用完，让直接提交）。接手的 AI 先做这几件：**

1. 跑端到端测试 `XC_SHOTS_BROWSER=/opt/pw-browsers/chromium-1194/chrome-linux/chrome XC_E2E_USE_BUILD=1 npm run e2e`。上一次跑时新加的“B69 在存储页加 WebDAV 账号”这一步已经通过；后面“B57 锁定后被隐藏的模块”那一步原来断言 `/remote-drives` 是空列表，因为 B69 前面加了账号所以失败。已经改成查 `/storage/remotes?drive=true` 只有“端到端网盘”，改完没再跑。
2. 跑一次后端全量 `go test -race ./...`。B69 只跑过 backup 和 storage 两个包，都通过；没跑全量。
3. 跑 `npm run shots`，看 设置 → 存储、设置 → 备份、云盘页在 1360px 和 390px 下的样子。没截过图。
4. 前端 typecheck、全部单元测试（65 个文件、403 个）、build、lint:fonts、prettier 都跑过，都通过。

| 任务 | 状态 | 要做什么 |
| --- | --- | --- |
| B58 天气 | 前后端都完成（后端 Claude，2026-10-01） | 和规格不同的地方写在 `docs/specs/B58.md` 的“后端”一节。开发环境连不上和风天气，上线后要用真 key 看一眼返回字段对不对 |
| B53 邮件 | 前后端都完成（后端 Claude，2026-10-01） | 和规格不同的地方写在 `docs/specs/B53.md` 的“后端”一节。只在内存 IMAP 服务器上测过，上线后要用户用 Gmail 应用专用密码和阿里企业邮箱真试一次 |
| B69 网盘账号挪到存储 | 前后端都完成（Claude，2026-10-01） | 和规格不同的地方写在 `docs/specs/B69.md` 开头。上线后看一眼：原来的备份照常、设置 → 存储里有迁过来的账号。下个版本删掉 `/remote-drives*`、`/backups/gdrive/auth` 和 `backup.webdav_password` 这几个旧设置键 |

截至 2026-10-01 20:46，`develop` 最新一次部署（`d487fff`）已经上线，包含 B63、B64、B68。之后另一个会话推了 B65（`7ca3aa3`），还没部署。

最近一轮 CI 上碰到过三个只在慢机器上出现的时序问题，都已修好：主机测试等代理连好再发指标（`7e01543`），WebDAV 上传失败后补删 `.part`（`49993b6`），编码任务构建结束先释放标记再写结果（`d487fff`）。以后 CI 偶尔红了，先看是不是这一类，找到原因再修，不要只重跑。

## 规则

- 分支：`develop`。
- 提交信息以任务编号开头。一个任务可能有多个提交，都以同一个编号开头。
- 每个任务的顺序：规格 → 接口定义（`api/modules/*.yaml`，跑生成）→ 后端占位（`pending.go` 回 `httpx.ErrNotLive`）→ 前端 → 测试。
- 前端遇到接口回 404 或 501 时，要能退回旧的样子或显示“还没上线”，不能报错。
- 小步骤只跑 `scripts/check-quick.sh`。没有用户说“部署”，提交信息里不带部署标记。

## 顺序和进度

| 任务 | 状态 | 说明 |
| --- | --- | --- |
| B54 | 前端完成（`b399ce6`） | 公共组件：菜单挂到最外层、Markdown 图片页内放大 |
| B49 | 前后端都完成 | 订阅。账号、默认提醒 7/3/1 天、汇率换算 |
| B50 | 前后端都完成 | 网站监控。只填域名补 https，抓站标 |
| B51 | 前后端都完成 | 域名。RDAP 没有就查 WHOIS，再没有就用手填日期 |
| B52 | 前后端都完成 | 推送同步。推送内容带发送时间，离线保留 3 天 |
| B55 | 前后端都完成 | 项目看板。锁定记在项目上，锁定时拒绝改看板和列表 |
| B56 | 前后端都完成（`aa76e8b`） | AI 润色公共接口 |
| B57 | 前后端都完成 | 隐藏内容。锁定时接口回 404，AI、MCP、事件、通知和早报不再露出 |
| B58 | 前后端都完成 | 和风天气和地震 |
| B53 | 前后端都完成 | 邮件 |
| B59 | 前端完成，不涉及后端 | 今日页 |

## 第二批（B60 到 B66）

2026-10-01 用户说额度不够，让 Claude 把剩下的需求全部写成规格，前后端都交给 GPT 分别做。规格在 `docs/specs/B60.md` 到 `B66.md`。

| 任务 | 内容 | 说明 |
| --- | --- | --- |
| B60 | AI 操作机器要经过 Agent，浮窗加权限、模型、思考程度 | 前后端都已完成（`22040b9`） |
| B61 | AI 记忆 | 前后端都已完成（`f66d695`） |
| B62 | Git 账号统一 | 前后端都已完成（`40a5a2a`） |
| B63 | 备份到 WebDAV 和 Google Drive | 前后端都已完成（Claude，`develop`），和规格不同的地方写在规格开头 |
| B68 | 云盘里浏览坚果云和 Google Drive，隐藏密码卡片，三级菜单收起 | 已完成（Claude，`develop`） |
| B64 | Unraid | 已完成（另一个会话，`e399314`） |
| B65 | OpenWrt 路由器集成 | 已完成（另一个会话，`7ca3aa3`） |
| B66 | 日历改名日程，早报挪到今日页 | 已完成（`fa31272`） |

这一轮 Claude 另外直接修了几个小问题（都在 `develop` 上）：Windows 代理隐藏黑窗口、事件日志编码、顶栏离线文字、AI 浮窗拖动卡死、夜间模式开关、提醒和习惯的计数、左栏二级和三级菜单、习惯的健身页（空页面）。

## 第一批（B49 到 B59）

前端都已提交。B56、B49、B52、B55、B50、B51、B57 的后端已做。B58 和 B53 的后端也已做完。

临时截图脚本 `web/scripts/.tmp-*.mjs` 没提交（已加到 `.git/info/exclude`），需要时自己写一个。

## 接手时注意

- 装代码生成工具：见 AGENTS.md“常用命令”第一段。
- 后端需要 Go 1.26（`go.mod` 要求，本地旧版本会自动下载工具链）。
- 本地的 Chromium 和 playwright-core 版本不一致时，截图和端到端测试都加 `XC_SHOTS_BROWSER=/opt/pw-browsers/chromium`。
- 认证中间件只在 `/api/v1/mcp` 上看 `Authorization: Bearer`。代理连 `/agent/connect` 也用 Bearer，改中间件时别弄坏（B43 第一次写错过，全量测试里代理全连不上）。
- 截图要先起服务端：`XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1 XC_DATA_DIR=<临时目录> XC_ADDR=127.0.0.1:8090 XC_WEB_DIR=<仓库>/web/dist go run ./cmd/server`，再 `npm run shots -- --base http://127.0.0.1:8090`。
