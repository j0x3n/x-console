# B188 AI 操作范围清单

盘点日期：2026-10-13。下面记录修改前的注册状态。
Effect 使用 Read、Write、Dangerous。删除还可能带 Destructive 标记。
本批优先补齐九个没有业务操作的模块。已有入口的模块列出后续候选，不在本次扩大范围。

| 模块 | 原有操作（名称和 Effect） | 缺口或判断 |
| --- | --- | --- |
| ai | agents.operate_host（Dangerous）、memory.delete（Write）、memory.save（Write）、memory.update（Write） | 本次排除。由任务 A 补齐会话相关操作。 |
| aiagents | 无 | 缺 Agent 列表、详情和运行记录。此次补齐。 |
| aiconfig | aiconfig.status（Read） | 已有常用查询入口。此次不新增专用操作。 |
| automations | 无 | 缺规则列表、详情和运行状态。此次补齐。 |
| backup | 无 | 缺备份列表、任务状态和手动备份。此次补齐。 |
| brief | brief.preview（Read） | 已有常用查询入口。此次不新增专用操作。 |
| calendar | calendar.events（Read） | 本次排除。由任务 B 处理日历同步。 |
| cards | cards.create（Write）、cards.due_count（Read） | 已有新建和待复习数量。缺卡片详情和复习。 |
| checkins | checkins.answer（Write）、checkins.series（Read） | 已有常用查询入口。此次不新增专用操作。 |
| coding | coding.launch（Write）、coding.list_tasks（Read） | 已有启动和任务列表。缺任务详情和日志。 |
| codingroi | codingroi.weekly（Read） | 已有常用查询入口。此次不新增专用操作。 |
| contacts | contacts.list（Read）、contacts.touch（Write） | 已有列表和联系记录。缺联系人详情。 |
| credentials | credentials.list（Read） | 已有安全的元数据列表。秘密内容不应返回给 AI。 |
| dashboard | 无 | 不加。此模块只保存卡片布局，没有聚合查询。业务数据已有各模块的操作。 |
| dayplan | dayplan.generate（Write）、dayplan.today（Read） | 已有常用查询入口。此次不新增专用操作。 |
| documents | documents.list（Read） | 已有列表。缺文件详情和正文。 |
| drive | drive.copy（Write）、drive.create_download_link（Read）、drive.create_folder（Write）、drive.create_upload_link（Write）、drive.delete（Write）、drive.info（Read）、drive.list（Read）、drive.move（Write）、drive.read_text（Read）、drive.rename（Write）、drive.resolve_path（Read）、drive.search（Read）、drive.share_list（Read）、drive.share_revoke（Write，Destructive）、drive.task_status（Read）、drive.upload_base64（Write）、drive.upload_from_url（Write）、drive.write_text（Write） | 常用目录、文件详情、文本和分享已覆盖。 |
| experiments | experiments.list（Read）、experiments.result（Read） | 已有常用查询入口。此次不新增专用操作。 |
| files | 无 | 不加。此模块只处理公共附件上传、下载和归属。独立列表会脱离所属记录的权限检查。附件通过原模块读取。 |
| focus | focus.current（Read）、focus.start（Write）、focus.stop（Write） | 已有开始、停止和当前状态。缺历史记录。 |
| focusblock | 无 | 缺设置和阻止状态。此次补齐。修改设置会结束程序，暂保留在页面。 |
| forecasts | forecasts.add（Write）、forecasts.list（Read）、forecasts.resolve（Write） | 已有常用查询入口。此次不新增专用操作。 |
| github | github.create_pr（Write）、github.list_pulls（Read） | 已有 PR 列表和创建。缺 PR 详情。 |
| goals | goals.list（Read）、goals.progress（Read）、goals.update_progress（Write） | 已有常用查询入口。此次不新增专用操作。 |
| habits | habits.body_record（Write）、habits.body_trend（Read）、habits.checkin（Write）、habits.create（Write）、habits.delete（Write）、habits.library（Read）、habits.personal_activate（Write）、habits.personal_check（Write）、habits.personal_day（Read）、habits.personal_profile（Read）、habits.stats（Read）、habits.today（Read）、habits.update（Write）、workouts.log（Write） | 常用习惯、打卡和统计已覆盖。 |
| homeassistant | ha.call_dangerous_service（Dangerous）、ha.call_service（Write）、ha.get_state（Read）、ha.list_entities（Read） | 已有实体列表、状态和服务调用。 |
| hosts | hosts.clipboard_set（Write）、hosts.exec（Dangerous）、hosts.get_metrics（Read）、hosts.list（Read）、hosts.open（Write）、hosts.power（Dangerous）、hosts.service_action（Dangerous） | 本次排除。由任务 B 处理服务器相关修复。 |
| inbox | inbox.add（Write）、inbox.list（Read） | 已有常用查询入口。此次不新增专用操作。 |
| journal | journal.day（Read）、journal.search（Read） | 已有日期查询和搜索。 |
| knowledge | knowledge.search（Read） | 已有常用查询入口。此次不新增专用操作。 |
| linear | 无 | 缺同步状态、团队列表和本地映射。此次补齐。改映射和同步会修改远端数据，暂不新增。 |
| mail | 无 | 缺账号状态、邮件列表、搜索和详情。此次补齐。模块没有发信接口。 |
| maintenance | 无 | 缺资源概况、扫描结果和清理状态。此次补齐。清理和压缩暂保留在页面。 |
| mcp | 无 | 不加。此模块提供操作调用入口，本身不是业务数据模块。 |
| me | me.state（Read） | 已有常用查询入口。此次不新增专用操作。 |
| monitoring | monitors.list（Read）、scripts.list（Read）、scripts.run（Dangerous）、subscriptions.list（Read） | 已有常用查询入口。此次不新增专用操作。 |
| music | music.match（Write）、music.pending（Read）、music.playlist_add（Write）、music.playlist_create（Write）、music.scan（Write）、music.search（Read） | 已有搜索和歌单写操作。缺歌单读取。 |
| notes | notes.append（Write）、notes.create（Write）、notes.delete（Write）、notes.get（Read）、notes.list（Read）、notes.search（Read）、notes.update（Write） | 常用列表、搜索、详情和写操作已覆盖。 |
| projects | projects.get（Read）、projects.create（Write）、projects.update（Write）、projects.archive（Write）、issues.list（Read）、issues.get（Read）、issues.create（Write）、issues.update（Write）、issues.move（Write）、issues.comment（Write）、issues.delete（Write）、milestones.create（Write）、projects.add_checklist_item（Write）、projects.archive_card（Write）、projects.check_item（Write）、projects.comment（Write）、projects.create_card（Write）、projects.create_issue（Write）、projects.get_board（Read）、projects.get_issue（Read）、projects.list（Read）、projects.list_boards（Read）、projects.list_issues（Read）、projects.move_card（Write）、projects.update_issue（Write） | 常用项目、Issue 和看板操作已覆盖。 |
| quotas | quotas.list（Read） | 已有常用查询入口。此次不新增专用操作。 |
| readlater | readlater.add（Write）、readlater.search（Read） | 已有搜索和添加。缺文章详情。 |
| reminders | notify.send（Write）、reminders.complete（Write）、reminders.create（Write）、reminders.delete（Write）、reminders.done（Write）、reminders.list（Read）、reminders.update（Write） | 常用查询、新建、修改和完成已覆盖。 |
| router | 无 | 缺状态、客户端和流量查询。此次补齐。设备控制暂保留在页面。 |
| screentime | screentime.summary（Read） | 已有常用查询入口。此次不新增专用操作。 |
| skills | skills.list（Read）、skills.log（Write） | 已有常用查询入口。此次不新增专用操作。 |
| storage | 无 | 缺存储状态和网盘账号列表。此次补齐。搬迁、改连接和文件删除暂保留在页面。 |
| vault | 无 | 不加。隐藏空间的解锁和管理不应交给 AI。 |

## 实现约束

- 所有新操作移除隐藏空间授权。跨模块结果继续使用现有隐藏过滤。
- 列表默认 20 项。最多 50 项。正文最多返回 2000 个字符。
- 不返回密码、令牌、Webhook 地址和原始执行结果。
- 手动备份标为 Dangerous。复用原接口的提升权限检查和审计。
- 每个操作在模块外部测试包中调用。重点检查筛选、限量和隐藏过滤。
## 盘点补充

`issues.list/get/create/update` 是兼容别名。正常操作目录不重复展示。
`drive.create_upload_link` 和 `drive.create_download_link` 仅供 MCP 使用。
现有 `.delete` 操作由调用方识别为删除。`drive.share_revoke` 显式标为 Destructive。

## 文件范围

以下路径都在 `backend/internal/server/modules/` 下。
每个模块新增 `actions.go` 和外部测试包 `actions_test.go`。
每个模块的 `module.go` 接入注册调用。
Linear 和自动化原来直接返回实例，需要先保存实例再注册。
`aiagents/module.go` 本批只新增 `m.registerActions()` 一行。
该文件另有任务 A 的会话服务注册，不属于 B188。

| 模块 | 新文件 | 注册文件 |
| --- | --- | --- |
| mail | mail/actions.go、mail/actions_test.go | mail/module.go |
| router | router/actions.go、router/actions_test.go | router/module.go |
| maintenance | maintenance/actions.go、maintenance/actions_test.go | maintenance/module.go |
| focusblock | focusblock/actions.go、focusblock/actions_test.go | focusblock/module.go |
| linear | linear/actions.go、linear/actions_test.go | linear/module.go |
| automations | automations/actions.go、automations/actions_test.go | automations/module.go |
| aiagents | aiagents/actions.go、aiagents/actions_test.go | aiagents/module.go |
| backup | backup/actions.go、backup/actions_test.go | backup/module.go |
| storage | storage/actions.go、storage/actions_test.go | storage/module.go |

本批文档只有 `docs/qa/B188-ai-actions.md`。
没有改接口定义、生成文件或迁移。

## 验证记录

检查日期：2026-10-13。
`gofmt -l` 检查本批 27 个 Go 文件，没有输出。
`go vet` 检查以下九个模块的全部子包，全部通过。
`go test` 检查同一组包，全部通过。包括 `backup/repo` 的原有测试。

检查的包：`mail/...`、`router/...`、`maintenance/...`、`focusblock/...`、`linear/...`、`automations/...`、`aiagents/...`、`backup/...`、`storage/...`。
这些路径都位于 `backend/internal/server/modules/`。

提交号：无。尝试暂存文档时无法创建 `.git/index.lock`，沙箱拒绝写入。
没有暂存其他任务的文件，没有推送。
建议提交标题：`B188: 补齐九个模块的 AI 操作`。
提交信息最后一行：`Co-Authored-By: Codex <noreply@openai.com>`。

- 25 个新操作都有正常调用测试。全部放在模块外部测试包。
- 检查未知字段、空对象类型、无效编号和 limit 范围。
- 用已解锁的上下文调用操作，验证操作会移除隐藏授权。
- 邮件覆盖搜索、账号筛选、未读筛选和正文截断。
- 主路由覆盖客户端限量和最新流量采样。累计流量保留完整范围。
- 维护覆盖隐藏条目和已缓存的隐藏模块统计。
- 专注阻止覆盖隐藏场景和隐藏电脑。按服务器、本机分别过滤。
- Linear 覆盖隐藏项目映射和统计。
- 自动化隐藏引用隐藏模块的规则。详情省略 Webhook 密钥和执行参数。
- Agent 运行记录隐藏项目标题。状态省略指令和原始执行结果。
- 存储状态不返回容量明细。隐藏云盘时不返回网盘账号。
- 存储重建不会重复注册。操作读取重建后的实例。
- 本地备份覆盖提升权限检查、成功执行和审计记录。
- 未连接真实邮箱、主路由、Linear 或云存储。外部服务使用已有假服务。
- 按用户要求没有跑全量后端、前端构建或端到端测试。

## 本次新增操作

共 25 个。其中 24 个为 Read，1 个为 Dangerous。
列表默认 20 项，最多 50 项。长文本最多 2000 个字符。
列表不足以返回全部结果时带 `truncated` 或 `<字段>Truncated`。
邮件列表保留 `nextBefore`，可以继续翻页。

| 模块 | 操作 | Effect | 返回范围 |
| --- | --- | --- | --- |
| mail | mail.accounts | Read | 邮箱状态和未读数量。省略连接信息。 |
| mail | mail.list | Read | 邮件摘要。支持账号、未读、日期筛选。 |
| mail | mail.search | Read | 按主题、发件人搜索摘要。 |
| mail | mail.get | Read | 邮件纯文本和附件元数据。省略 HTML。 |
| router | router.status | Read | 主路由运行状态。 |
| router | router.clients | Read | 客户端列表。 |
| router | router.traffic | Read | 24 小时或 7 天流量。采样只保留最新几项。 |
| maintenance | maintenance.overview | Read | 资源和可见存储概况。省略数据目录。 |
| maintenance | maintenance.status | Read | 扫描或清理状态。只返回可见分组数量。 |
| focusblock | focusblock.status | Read | 可见电脑的阻止状态。 |
| focusblock | focusblock.settings | Read | 阻止设置。过滤隐藏场景和电脑。 |
| linear | linear.status | Read | 同步状态。错误只返回数量。 |
| linear | linear.teams | Read | Linear 团队摘要。 |
| linear | linear.mappings | Read | 可见项目的团队映射。 |
| automations | automations.list | Read | 可见规则摘要。 |
| automations | automations.get | Read | 触发条件和动作名称。省略动作参数。 |
| automations | automations.runs | Read | 状态和时间。省略输入与结果正文。 |
| aiagents | aiagents.list | Read | Agent 状态、任务数量和费用。 |
| aiagents | aiagents.get | Read | 状态与权限摘要。省略指令和仓库配置。 |
| aiagents | aiagents.runs | Read | 可见项目的运行摘要。省略执行结果。 |
| backup | backup.list | Read | 备份元数据。 |
| backup | backup.status | Read | 备份任务状态。省略错误和进度正文。 |
| backup | backup.export | Dangerous | 创建本地备份。需要提升权限。 |
| storage | storage.status | Read | 后端类型和搬迁状态。省略文件容量明细。 |
| storage | storage.remotes | Read | 可见网盘账号的就绪状态。省略连接配置。 |
