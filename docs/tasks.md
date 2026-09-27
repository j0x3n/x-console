# 任务看板

这是项目进度的唯一来源。做什么、做到哪、下一步是什么，都只看这里。

- 开发者从“待做”里按顺序取第一个任务，开分支前把状态改成“进行中”，写上自己的名字。
- 任务在 PR 合并进 `develop` 后移到“已完成”，写上 PR 链接。
- 大任务的做法和验收标准在 `docs/specs/` 里。小任务直接写在这里。
- 用户提的新需求，由审查者加到“待做”末尾（编号顺延），需要时写规格文件。

## 进行中

| 编号 | 任务 | 规格 | 负责 |
| --- | --- | --- | --- |
| B11 | 统一页头并重做笔记页面 | [specs/B11.md](specs/B11.md) | GPT |

## 待做

| 编号 | 任务 | 规格 | 负责 |
| --- | --- | --- | --- |
| B1 | 部署面板的主机自动加入代理，可手动移除 | [specs/B1.md](specs/B1.md) | |
| B2 | 概览首页 | [specs/M1.md](specs/M1.md) | |
| B3 | AI 助手与自动化 | [specs/M12.md](specs/M12.md) | |
| B4 | 命令面板支持前缀输入：`> 内容` 直接存成笔记 | 见下方说明 | |
| B5 | PWA：manifest、图标、安装提示 | 见下方说明 | |
| B6 | 路由懒加载，消除主包超过 500 kB 的构建警告 | 见下方说明 | |
| B7 | 早报的“续费”部分接上运维监控 | 见下方说明 | |
| B8 | Playwright 端到端测试加进 CI | 见下方说明 | |
| B9 | 前端统一跑一遍 prettier，并在 CI 里检查 | 见下方说明 | |

### 待做任务的说明

**B2 概览首页**
- `web/src/features/overview`：卡片网格，数据来自各模块已有的 hooks。每张卡片包错误边界，一张出错不影响别的。卡片布局存服务端。
- 要显示的卡片和布局接口见规格。

**B3 AI 助手与自动化**
- 用官方 Go SDK `github.com/anthropics/anthropic-sdk-go`。默认模型 `claude-opus-5`，adaptive thinking，流式输出，开启服务端 refusal fallback（`fallbacks: "default"` 加 beta 头 `server-side-fallback-2026-07-01`）。写代码前查官方 SDK 文档确认用法，不要凭记忆。
- 工具来自 `d.Actions.List()`，各模块已经注册了动作（`grep -rn "Actions.Register" backend/internal/server/modules`）。`read` 直接执行；`write` 等用户确认；`dangerous` 要确认并要求提升权限。
- 自动化引擎执行动作时，actor 设成 `automation:<规则id>`（`audit.WithActor`）。M10 的 `scripts.run` 靠它区分自动化和 AI。
- HA 实体做触发器时要调用 `contracts.HomeAssistant.WatchEntity`。
- 早报的“AI 润色”：在注册表键 `ai.brief_polisher` 下注册实现 `brief` 包 Polisher 接口的对象，设置页的开关就会出现。
- 前端：`features/assistant`、`features/automations`；在 `app/GlobalPanels.tsx` 加侧边面板，在 `app/TopbarActions.tsx` 加按钮。

**B4 命令面板前缀输入**
- 现在 `web/src/components/command/CommandPalette.tsx` 不能把输入的文字传给命令。给 `Command` 加可选的前缀，比如 `>`，输入以它开头时把剩余文字传给 `run`。备忘模块注册 `>` 前缀，存成新笔记。

**B5 PWA**
- `web/public/manifest.webmanifest`、图标、`index.html` 引用、安装提示。
- `web/public/sw.js` 已有推送处理，缓存逻辑加在文件顶部标注的位置。

**B6 路由懒加载**
- 各模块的页面组件用 `React.lazy` 加载，构建时不再出现超过 500 kB 的警告。

**B7 早报续费**
- 早报在注册表键 `monitoring.renewals` 下找实现 `brief.RenewalSource` 的对象，运维监控模块还没注册。
- 做法：在 `contracts` 里加 `Renewals` 接口（`Upcoming(ctx, until) ([]RenewalRef, error)`），运维监控实现并注册，早报改用它。在下面的“接口变更记录”里记一笔。

**B8 端到端测试**
- 用 Playwright 跑主流程：初始化账号、登录、建 Issue、写备忘、建提醒、看服务器、开终端、打卡。
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

## 接口变更记录

跨模块的接口（`internal/server/contracts`、基础包、协议）有调整时记在这里。

| 日期 | 变更 | 原因 |
| --- | --- | --- |
| 2026-09-27 | 新增 `contracts.IssueSync`、`HomeAssistant.WatchEntity` | Linear 同步和 HA 联动需要 |
| 2026-09-27 | `app.New` 对重复的模块构造函数去重 | 测试里可以再传一次已注册的模块 |
| 2026-09-27 | `rpc` 写入不再使用可取消的 context；`shutdown` 修复 inflight 数据竞争 | 负载高时代理连接会被误断开 |
| 2026-09-27 | 数据库连接使用 `_txlock=immediate` | 文件数据库上并发的先读后写事务会报 SQLITE_BUSY |
| 2026-09-27 | 新增 `proxy`、`open` 两个代理能力 | M9 访问内网 HA；M3 打开程序和网址 |
| 2026-09-27 | 中文词典冲突检查（`web/src/lib/i18n.test.ts`） | 不同模块用同一个英文键注册了不同中文，互相覆盖 |
