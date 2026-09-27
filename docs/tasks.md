# 任务看板

这是项目进度的唯一来源。做什么、做到哪、下一步是什么，都只看这里。

- 开发者从“待做”里按顺序取第一个任务，开分支前把状态改成“进行中”，写上自己的名字。
- 任务在 PR 合并进 `develop` 后移到“已完成”，写上 PR 链接。
- 大任务的做法和验收标准在 `docs/specs/` 里。小任务直接写在这里。
- 用户提的新需求，由审查者加到“待做”末尾（编号顺延），需要时写规格文件。

## 进行中

| 编号 | 任务 | 规格 | 负责 |
| --- | --- | --- | --- |
| B4 | 命令面板支持前缀输入：`> 内容` 直接存成笔记 | 见下方说明 | Claude |

## 待做

| 编号 | 任务 | 规格 | 负责 |
| --- | --- | --- | --- |
| B5 | PWA：manifest、图标、安装提示 | 见下方说明 | |
| B6 | 路由懒加载，消除主包超过 500 kB 的构建警告 | 见下方说明 | |
| B11 | 笔记后端：接口现在返回 501。做完后验收“刷新后图片还在”“删笔记时删附件” | [specs/B11.md](specs/B11.md) | |
| B2 | 今日页后端：`GET/PUT /dashboard/layout` | [specs/M1.md](specs/M1.md) | |
| B12 | 两步验证可选的后端 | [specs/B12.md](specs/B12.md) | |
| B13 | 隐藏内容的后端：`/vault/*`、笔记的 `hidden` | [specs/B13.md](specs/B13.md) | |
| B14 | 云盘后端：`modules/drive`、S3 同步 | [specs/B14.md](specs/B14.md) | |
| B3 | AI 助手和自动化的后端 | [specs/M12.md](specs/M12.md) | |
| B15 | B10 遗留：0% 进度环多一个点；提醒页和项目页的概要卡片和规格不一致，等用户决定按哪个做 | 见下方说明 | |
| B1 | 部署面板的主机自动加入代理，可手动移除 | [specs/B1.md](specs/B1.md) | |
| B7 | 早报的“续费”部分接上运维监控 | 见下方说明 | |
| B8 | Playwright 端到端测试加进 CI | 见下方说明 | |
| B9 | 前端统一跑一遍 prettier，并在 CI 里检查 | 见下方说明 | |

顺序说明（2026-09-27 用户要求）：先把前端做完整，再一步步做后端。

- 第一轮只做前端：B10、B11、B2、B12、B13、B14、B3 的界面部分，以及 B4、B5、B6。接口契约（`api/modules/*.yaml`）和前端一起写好，后端没做的接口先返回 501（错误码 `not_ready`）或 404，界面上显示“功能还没上线”。
- 第二轮按表里的顺序补后端：B11、B2、B12、B13、B14、B3，然后 B1、B7。
- 最后做 B8、B9。
- 这一轮由 Claude 一个人按顺序做，不开子代理。每个任务做完、全部检查通过后直接提交到 `develop`，不开 PR（2026-09-27 用户要求）。`main` 仍然要用户同意才动。

### 待做任务的说明

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
- 文件上传进度条。SSH 主机指纹变化后在界面上重新信任。
- `features/reminders` 和 `features/habits` 的 `api.ts` 修改后自己刷新数据，同时又用了 `invalidateOn`，有重复。
- `features/projects` 的 Markdown 渲染器可以挪到 `components/ui` 给其他模块用。
- 编码任务的运行设置在仓库页，没有单独的设置标签。

## 已完成

| 批次 | 内容 |
| --- | --- |
| 0 | M0 基础（登录、TOTP、审计、加密设置、事件、调度、通知、代理配对与协议）、前端外壳、contracts、actions、文档 |
| 1 | M2/M3 服务器和本机、M5 项目、M6 备忘、M7 提醒与通知、M8 习惯、M9 Home Assistant |
| 2 | M4 编码任务、M10 运维监控、M11 日历早报番茄钟、M13 GitHub 和 Linear |
| 部署 | GitHub Actions 自动测试、构建镜像、部署到用户服务器，已上线 |
| B10 | 界面统一（[#12](https://github.com/j0x3n/x-console/pull/12)） |
| B11 前端 | 备忘改名笔记，新列表和编辑器，图片和附件（[#13](https://github.com/j0x3n/x-console/pull/13)） |
| B12 前端 | 两步验证可选：登录分两步、初始化可跳过、设置里的“安全”标签（[#16](https://github.com/j0x3n/x-console/pull/16)） |
| B13 前端 | 隐藏内容：点 Logo 5 次解锁、顶部提示栏、15 分钟自动锁定、笔记的“隐藏”分类、安全标签里改隐藏密码。云盘部分跟 B14 一起做 |
| B14 前端 | 云盘：列表和网格、多选、拖拽上传和进度、预览、移动改名、回收站、隐藏分类、设置里的“云盘同步” |
| B3 界面 | AI 助手浮窗（右下角按钮、⌘J、可拖动和放大、手机全屏、历史对话、动作卡片和确认）、设置里的“AI 助手”、自动化的规则列表、编辑器和运行记录 |
| B2 前端 | 今日页规格和前端（[#14](https://github.com/j0x3n/x-console/pull/14)、[#15](https://github.com/j0x3n/x-console/pull/15)） |

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
| 2026-09-27 | 中文词典冲突检查（`web/src/lib/i18n.test.ts`） | 不同模块用同一个英文键注册了不同中文，互相覆盖 |
