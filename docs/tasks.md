# 任务看板

这是项目进度的唯一来源。做什么、做到哪、下一步是什么，都只看这里。

- 开发者从“待做”里按顺序取第一个任务，开分支前把状态改成“进行中”，写上自己的名字。
- 任务在 PR 合并进 `develop` 后移到“已完成”，写上 PR 链接。
- 大任务的做法和验收标准在 `docs/specs/` 里。小任务直接写在这里。
- 用户提的新需求，由审查者加到“待做”末尾（编号顺延），需要时写规格文件。

## 当前状态（2026-09-27 更新）

- 前端已经做完四轮，线上是 `develop` 最新的部署（带部署标记的提交会部署，见 AGENTS.md）。
- 演示数据保留（用户要求），页面底部的“演示数据：开/关”可以切换。说明见下方“演示数据”。
- **下一步：Codex 在 `codex` 分支上做后端，从 B2 开始，按“待做”表的顺序往下做。** 做法见 AGENTS.md 的“流程”。
- `frontend-done` 分支是交给 Codex 之前的版本，需要时可以回到这里。

## 进行中

| 编号 | 任务 | 规格 | 负责 |
| --- | --- | --- | --- |
| （空） | | | |

## 待做

后端按这个顺序做。全部在 `codex` 分支上做，一个任务一个提交，提交信息以编号开头；一批做完由 Claude 整批验收，用户合并一次（2026-09-27 用户要求）。用户说“构建”或“部署”时才带部署标记。

| 编号 | 任务 | 规格 | 负责 |
| --- | --- | --- | --- |
| B12 | 两步验证可选的后端 | [specs/B12.md](specs/B12.md) | |
| B13 | 隐藏内容的后端：`/vault/*`、笔记的 `hidden`、隐藏空间自己的标签（`/notes/tags?hidden=true`） | [specs/B13.md](specs/B13.md) | |
| B8 | Playwright 端到端测试加进 CI。先覆盖最常用的流程，以后每个新功能补一条 | 见下方说明 | |
| B14 | 云盘后端：`modules/drive`、S3 同步、隐藏空间（记住原位置，还原放回） | [specs/B14.md](specs/B14.md) | |
| B3 | AI 助手和自动化的后端 | [specs/M12.md](specs/M12.md) | |
| C1 | 第一轮清理：不加新功能。去掉重复代码、补缺的测试、更新文档，并做 B9（prettier 检查加进 CI） | 见下方说明 | |
| B16 | 日历可写：本地日历、CalDAV（iCloud）写回。前端已做，后端回 501 | [specs/B16.md](specs/B16.md) | |
| B17 | 健身类习惯：记训练自动打卡。前端已做 | [specs/B17.md](specs/B17.md) | |
| B18 | 按需推送：WebSocket 订阅主题、代理按需上报详细数据 | [specs/B18.md](specs/B18.md) | |
| B1 | 部署面板的主机自动加入代理，可手动移除 | [specs/B1.md](specs/B1.md) | |
| B7 | 早报的“续费”部分接上运维监控 | 见下方说明 | |
| C2 | 第二轮清理，同 C1 | 见下方说明 | |
| B15 | B10 遗留：提醒页和项目页的概要卡片和规格不一致，等用户决定按哪个做 | 见下方说明 | |
| D1 | 去掉演示数据。**只有用户明确说要去掉时才做** | 见下方说明 | |

### 待做任务的说明

**演示数据（保留，D1 才删）**
- `web/src/demo` 在浏览器里拦请求，返回假数据。`main.tsx` 第一行引入。
- 常开的拦截（不管开关）：还没有后端的功能，`demo/vault.ts`（隐藏内容）、`demo/drive.ts`（云盘）、`demo/ai.ts`（AI 助手）、`demo/automations.ts`（自动化）。
- 演示开关：页面底部“演示数据：开/关”，存在浏览器的 `xc.demo.full` 里，默认开。开着时 `demo/full/*` 接管已有后端的模块（`demo/mode.ts` 的 `FULL_PREFIXES`），读写只动内存，刷新后恢复原样，不碰服务器上的真数据。关掉后回到真数据。`PASS_THROUGH` 里的接口（搜城市）始终走真实服务器。
- 今日页布局 B2 已接后端：`demo/dashboard.ts` 只在演示开关打开时拦截。
- **做后端时注意**：做完 B13、B14、B3 中任何一个，要把上面对应的“常开拦截”改成只在开关打开时生效（文件开头判断 `demoFull`，或者挪进 `demo/full`），否则关掉开关也看不到真数据。改完在这里更新。
- 做新的前端功能时，同时在 `demo/full` 里补假数据，保证演示模式下能用。
- D1 删除时：删 `web/src/demo`、`main.tsx` 第一行、`api/events.ts` 的 `emitDemoEvent`、`features/drive/api.ts` 的 `demoFileUrl`。

**B15 B10 遗留**
- `components/ui/Stat.tsx` 的 `Ring`：比例为 0 时不渲染 `.bar`。原因是 `stroke-linecap: round`，长度为 0 也会画一个点。
- 提醒页概要现在是“今天、下一个、即将到来、已完成”，规格写的是“今天、逾期、本周”。项目页“进行中”只在说明里，没单独成卡。等用户决定。

**B2 今日（首页）**
- 依赖 B10 的 `StatStrip`、`Section`。
- 第一轮前端：`web/src/features/overview` 做今日页，侧边栏改名“今日”。数据用各模块已有的接口，不用改后端。每个分区包错误边界。写好布局接口契约 `api/modules/dashboard.yaml`；接口没上线时布局先存在本地，并提示“还没上线”。
- 第二轮后端：新建 `modules/dashboard`，实现 `GET/PUT /dashboard/layout`。
- 做法、要调的接口和验收标准见规格。

**B3 AI 助手与自动化**
- 用官方 Go SDK `github.com/anthropics/anthropic-sdk-go`。默认模型 `claude-opus-5-5`，adaptive thinking，流式输出，开启服务端 refusal fallback（`fallbacks: "default"` 加 beta 头 `server-side-fallback-2026-07-01`）。写代码前查官方 SDK 文档确认用法，不要凭记忆。
- 工具来自 `d.Actions.List()`，各模块已经注册了动作（`grep -rn "Actions.Register" backend/internal/server/modules`）。`read` 直接执行；`write` 等用户确认；`dangerous` 要确认并要求提升权限。
- 自动化引擎执行动作时，actor 设成 `automation:<规则id>`（`audit.WithActor`）。M10 的 `scripts.run` 靠它区分自动化和 AI。
- HA 实体做触发器时要调用 `contracts.HomeAssistant.WatchEntity`。
- 早报的“AI 润色”：在注册表键 `ai.brief_polisher` 下注册实现 `brief` 包 Polisher 接口的对象，设置页的开关就会出现。
- 前端：`features/assistant`、`features/automations`。助手是全局浮窗，右下角按钮打开，放在 `app/GlobalPanels.tsx`，不在侧边栏出现。见规格。
- 所有非隐藏内容都要能读写删，缺的动作在各模块补上，清单见规格。

**B4 命令面板前缀输入**
- 现在 `web/src/components/command/CommandPalette.tsx` 不能把输入的文字传给命令。给 `Command` 加可选的前缀，比如 `>`，输入以它开头时把剩余文字传给 `run`。笔记模块注册 `>` 前缀，存成新笔记。

**B5 PWA**
- `web/public/manifest.webmanifest`、图标、`index.html` 引用、安装提示。
- `web/public/sw.js` 已有推送处理，缓存逻辑加在文件顶部标注的位置。

**B6 路由懒加载**
- 各模块的页面组件用 `React.lazy` 加载，构建时不再出现超过 500 kB 的警告。

**B7 早报续费**
- 早报在注册表键 `monitoring.renewals` 下找实现 `brief.RenewalSource` 的对象，运维监控模块还没注册。
- 做法：在 `contracts` 里加 `Renewals` 接口（`Upcoming(ctx, until) ([]RenewalRef, error)`），运维监控实现并注册，早报改用它。在下面的“接口变更记录”里记一笔。

**C1、C2 清理轮（2026-09-27 用户同意）**
- 每做完大约 5 个功能任务，留一轮只做清理，不加新功能：
  - 找出重复的代码和样式，合到公共组件里；
  - 每个模块至少有接口的集成测试，前端每个页面至少有一个渲染测试；
  - 对一遍规格和代码，规格过时的改规格；
  - 看一遍“已知问题”，能修的修掉；
  - `npm run shots` 全部页面过一遍，对照 `docs/07-design.md`。
- C1 同时做 B9：统一跑 prettier，并在 CI 里检查。

**B8 端到端测试**
- 用 Playwright 跑主流程：初始化账号、登录、建 Issue、写笔记、建提醒、看服务器、开终端、打卡。
- 在 CI 里起真实服务端和一个 Linux 代理。

**B9 代码格式**
- `cd web && npx prettier --write "src/**/*.{ts,tsx,css}"`，`src/api/gen/` 已被 `.prettierignore` 排除。
- CI 的 web 任务加 `npx prettier --check`。

## 已知问题（暂不排期）

没有在真实环境验证过：
- Windows 代理的运行时行为：ConPTY 终端、服务管理、剪贴板、锁屏关机、打开程序、编码任务的中断。只做过交叉编译和静态检查。
- 真实的 Claude Code 和 Codex CLI。Codex 的默认参数 `exec --json --full-auto -` 没实测，可以在代理配置里改。
- 真实的 Home Assistant、Telegram、Bark、Server酱、Web Push、GitHub、Linear。测试全部用假服务器。
- systemd 服务管理（开发环境没有 systemd）和真实的 SSH 主机。

功能限制：
- 同一个 TOTP 码在 30 秒窗口内可以重复使用。只支持一个用户。
- HA 的 `WatchEntity` 注册只存在内存里，使用方要在 `Start` 里调用。
- GitHub 每个仓库只拉第一页（100 个 PR）。Linear 不导入已完成或已取消的 Issue，本地新建的 Issue 不会自动建到 Linear。
- 看板拖动只支持桌面。习惯的提醒时段不能跨午夜。番茄钟不能暂停。
- 早报的习惯部分只显示今天，`contracts.Habits` 没有“昨天”的数据。
- SSH 主机的最后在线时间只存在内存里。Windows 上 `svc.logs` 返回“不支持”。
- 脚本运行记录不会自动清理。Windows 主机上跑 bash 脚本会直接失败。订阅支出汇总没有汇率换算。

可以改进：
- 手机上命令面板比屏幕高，底部的结果和前缀提示要滚动才能看到。
- 文件上传进度条。SSH 主机指纹变化后在界面上重新信任。
- `features/reminders` 和 `features/habits` 的 `api.ts` 修改后自己刷新数据，同时又用了 `invalidateOn`，有重复。
- Agent 任务（原“编码任务”）的运行设置在仓库页，没有单独的设置标签。
- 公共的 Markdown 编辑框（`components/markdown/MarkdownEditor`）还不能贴图片，只有笔记能。
- 搜城市先查 Open-Meteo，查不到再查 OpenStreetMap（Nominatim），开发环境连不上外网，没在真实网络下验证过。

## 已完成

| 批次 | 内容 |
| --- | --- |
| B19 | 部署前备份数据库和失败回退（待合并） |
| B11 后端 | 附件上传下载、缩略图、笔记删除时清理文件（待合并） |
| B2 后端 | 今日页布局保存与跨设备读取（待合并） |
| 0 | M0 基础（登录、TOTP、审计、加密设置、事件、调度、通知、代理配对与协议）、前端外壳、contracts、actions、文档 |
| 1 | M2/M3 服务器和本机、M5 项目、M6 备忘、M7 提醒与通知、M8 习惯、M9 Home Assistant |
| 2 | M4 编码任务、M10 运维监控、M11 日历早报番茄钟、M13 GitHub 和 Linear |
| 部署 | GitHub Actions 自动测试、构建镜像、部署到用户服务器，已上线 |
| B10 | 界面统一（[#12](https://github.com/j0x3n/x-console/pull/12)） |
| B11 前端 | 备忘改名笔记，新列表和编辑器，图片和附件（[#13](https://github.com/j0x3n/x-console/pull/13)） |
| B12 前端 | 两步验证可选：登录分两步、初始化可跳过、设置里的“安全”标签（[#16](https://github.com/j0x3n/x-console/pull/16)） |
| B13 前端 | 隐藏内容：点 Logo 3 次解锁、解锁后 Logo 旁一个小锁、15 分钟自动锁定、笔记的“隐藏”分类、安全标签里改隐藏密码。云盘部分跟 B14 一起做 |
| B14 前端 | 云盘：列表和网格、多选、拖拽上传和进度、预览、移动改名、回收站、隐藏分类、设置里的“云盘同步” |
| B3 界面 | AI 助手浮窗（右下角按钮、⌘J、可拖动和放大、手机全屏、历史对话、动作卡片和确认）、设置里的“AI 助手”、自动化的规则列表、编辑器和运行记录 |
| B4 | 命令面板前缀：`Command.prefix`，笔记注册 `>`，输入“> 内容”回车直接存成笔记 |
| B5 | PWA：manifest 和图标、SW 离线缓存（接口不缓存）、页头“安装应用”按钮、设置 → 通用里的安装说明、断网提示 |
| 设置页 | 按用户要求，设置页的切换从 B10 的左侧一列改回顶部一排标签，内容的卡片排法不变 |
| B6 | 路由懒加载：各模块页面用 `React.lazy`；第三方库拆成 react、vendor、icons 三个包；qrcode 按需加载。主包 132 kB，没有超过 500 kB 的警告 |
| B2 前端 | 今日页规格和前端（[#14](https://github.com/j0x3n/x-console/pull/14)、[#15](https://github.com/j0x3n/x-console/pull/15)） |
| 界面第二轮 | 去掉大标题、侧边栏二级菜单、概要卡片缩小、命令面板、设计规范 `docs/07-design.md` 和 `npm run shots`、只在带部署标记时部署 |
| 界面第三轮 | 全部页面的演示数据和开关；今日页按宽度分 1～4 列、天气置顶；“编码任务”改名“Agent 任务”；笔记标签颜色（后端已做）；健身并入习惯、健身类习惯、图标点选；云盘概况一行字；日历新建编辑界面；本机快捷按钮；后台标签页不处理高频推送 |
| 界面第四轮 | 左上角标题可点击跳转、去掉返回按钮、只有设备页有状态点；天气设置弹窗（搜城市、显示内容、降雨提醒，后端已做）；隐藏空间（独立文件夹和标签、原样还原）；本机操作二次确认；笔记列表选中样式 |
| 编辑框统一 | 项目描述、Issue 描述和评论、新建 Issue、日程备注、提醒备注、新建 Agent 任务都用和笔记一样的 Markdown 编辑框（`components/markdown/MarkdownEditor`） |

## 接口变更记录

跨模块的接口（`internal/server/contracts`、基础包、协议）有调整时记在这里。

| 日期 | 变更 | 原因 |
| --- | --- | --- |
| 2026-09-27 | 新增 `contracts.IssueSync`、`HomeAssistant.WatchEntity` | Linear 同步和 HA 联动需要 |
| 2026-09-27 | `app.New` 对重复的模块构造函数去重 | 测试里可以再传一次已注册的模块 |
| 2026-09-27 | `rpc` 写入不再使用可取消的 context；`shutdown` 修复 inflight 数据竞争 | 负载高时代理连接会被误断开 |
| 2026-09-27 | 数据库连接使用 `_txlock=immediate` | 文件数据库上并发的先读后写事务会报 SQLITE_BUSY |
| 2026-09-27 | 新增 `proxy`、`open` 两个代理能力 | M9 访问内网 HA；M3 打开程序和网址 |
| 2026-09-27 | `PageHeading` 加 `meta`，`title`、`subtitle` 可以传节点；新增 `components/ui/Stat.tsx`（`StatStrip`、`StatCard`、`Ring`、`Segments`、`MiniBars`、`Section`）；`.xc-page` 去掉最大宽度 | B10 界面统一 |
| 2026-09-27 | Markdown 渲染器从 `features/projects` 挪到 `components/markdown`，支持图片、可勾选的待办；原路径保留转发 | B11 笔记要显示图片，别的模块也要用 |
| 2026-09-27 | `app/nav.ts` 首页入口从 `Overview` 改成 `My day`（今日），图标换成 `Sun` | B2 今日页 |
| 2026-09-27 | `core.yaml`：登录的 `code` 改为可选，没带时回 401 `totp_required`（不计失败次数，已实现）；`/auth/elevate` 可传 `password`；新增 `/auth/setup/skip-totp`、`/auth/totp/*`、`/auth/password`（先回 501）；`AuthStatus` 加 `totpEnabled`。前端新增 `auth/TotpQr.tsx`，`ui.css` 加 `.xc-auth-actions` | B12 两步验证可选 |
| 2026-09-27 | 侧边栏 Logo 点击时发出 `xc:brand-tap` 事件；`app/GlobalPanels.tsx` 加 `VaultPanel`；新增 `api/modules/vault.yaml`，笔记接口加 `hidden` | B13 隐藏内容 |
| 2026-09-27 | `app/nav.ts` 在“个人”组加“云盘”（`/drive`）；`app/routes.tsx` 加 drive 路由；新增 `api/modules/drive.yaml` | B14 云盘 |
| 2026-09-27 | `app/nav.ts` 去掉“AI 助手”入口（改成全局浮窗）；`app/GlobalPanels.tsx` 加 `AssistantPanel`；新增 `api/modules/ai.yaml`、`automations.yaml`。比规格多了 `/ai/tools`、`/ai/conversations/{id}/stop` 和事件 `ai.message_saved`，已写进 M12 规格 | B3 界面 |
| 2026-09-27 | `lib/commands.ts` 的 `Command` 加可选的 `prefix`，`run` 的参数加可选的 `text`；新增 `matchPrefix`。命令面板支持前缀命令，底部显示可用前缀 | B4 |
| 2026-09-27 | `public/sw.js` 顶部加缓存逻辑；`index.html` 引用 manifest 和图标；`app/TopbarActions.tsx` 加 `InstallButton`；`auth/AuthGate.tsx` 断网时显示“网络断开了” | B5 PWA |
| 2026-09-27 | `app/Layout.tsx` 的 `Outlet` 外面包 `Suspense`；`web/vite.config.ts` 加 `manualChunks`；各模块 `routes.tsx` 的页面改成 `lazy` | B6 |
| 2026-09-27 | `PageHeading` 默认不显示大标题（加 `showTitle` 才显示），字符串标题写进新的 `stores/page-title.ts`，`Topbar` 在详情页显示“模块 / 名称”；`ui.css` 加 `.xc-sr-only` | 用户要求页面只用左上角小标题 |
| 2026-09-27 | 新增 `lib/navChildren.ts`（`registerNavChildren`）和 `components/layout/NavChildLinks.tsx`，`Sidebar` 支持二级菜单；设置从 `app/nav.ts` 移到个人菜单；`StatStrip` 加 `size`，默认紧凑；命令面板没输入时只列常用命令，搜索按标题排序；`Ring` 比例为 0 时不画 | 用户要求的界面调整 |
| 2026-09-27 | `ci.yml` 只改文档时不跑，同一分支连续推送时取消旧的检查 | 减少构建次数 |
| 2026-09-27 | 新增公共组件 `components/ui/Toolbar.tsx`（`Toolbar`、`SearchBox`、`Segmented`）、`MoreMenu.tsx`、`Switch.tsx`，`States` 加 `NotLive`，`ui.css` 加 `.xc-check`；云盘、自动化、AI 助手设置改用它们。新增 `docs/07-design.md` 和 `npm run shots`（`web/scripts/shots.mjs`，开发依赖 `playwright-core`） | 让别的开发者照着做出一样的界面 |
| 2026-09-27 | 平时推送 develop 不跑 CI；develop 上最后一个提交带 `[deploy]` 时构建并部署（见 AGENTS.md“构建和部署”） | 用户要求省构建额度 |
| 2026-09-27 | 中文词典冲突检查（`web/src/lib/i18n.test.ts`） | 不同模块用同一个英文键注册了不同中文，互相覆盖 |
| 2026-09-27 | `NavChildLinks` 的链接加可选的 `active`（带查询参数的链接自己判断选中）；`ui.css` 加 `.nav-child-dot`；侧边栏点一级菜单就展开二级菜单 | Agent 任务按状态、笔记按标签的二级菜单 |
| 2026-09-27 | `api/events.ts` 页面在后台时跳过 `host.metrics`、`ha.state_changed`，切回来刷新；`api/query.ts` 的 `staleTime` 从 15 秒改成 60 秒 | 性能优化（B18） |
| 2026-09-27 | `notes.yaml` 加 `PUT /notes/tag-colors`，`TagCount.color`（后端已实现，迁移 `m6_note_tag_colors`）；`habits.yaml` 加 `HabitKind`；`calendar.yaml` 加 `local` 类型、`writable`、事件的新建修改删除（后端回 501） | 标签颜色、健身类习惯、日历可写 |
| 2026-09-27 | “编码任务”界面上改名“Agent 任务”，导航图标换成 `Bot`；接口和代码里的名字不变 | 用户要求，Agent 不只写代码 |
| 2026-09-27 | `stores/page-title.ts` 加 `parents`、`status` 和 `usePageCrumb`、`usePageStatus`；`PageHeading` 加 `parents`；`Topbar` 的模块名和中间层可以点击跳转，去掉常驻的连接状态点（只在断线时显示黄点），设备页显示在线状态点；`Sidebar` 在“X Console”后面加 `#brand-slot`，再点一次已展开的一级菜单会收起 | 用户要求左上角兼做导航，详情页去掉“返回”按钮 |
| 2026-09-27 | `brief.yaml` 加 `GET /weather/places`、`GET/PUT /weather/alert`（后端已实现，每 30 分钟检查降雨）；`notes.yaml` 的 `/notes/tags` 加 `hidden`；`drive.yaml` 加 `restoreTo`，写清隐藏和还原的规则 | 天气设置弹窗、隐藏空间 |
| 2026-09-27 | 新增 `components/markdown/MarkdownEditor.tsx`（样式 `.xc-mde*` 在 `ui.css`），编辑用的纯函数从 `features/notes/logic.ts` 挪到 `components/markdown/edit.ts`（notes 里保留转发）；`demo/mode.ts` 加 `PASS_THROUGH` | 长文字输入统一用笔记的编辑框 |
