# 接口变更记录

- B254（2026-10-19）：仅修改基础包 `core/agentdist_test.go` 的测试时间，使用 UTC 构造过期配对码，以兼容 Windows 东八区环境。不改生产代码、API 或数据库。

- B250（2026-10-19）：XC-57 合并左栏入口，需要调整公共导航 `app/nav.ts`、`components/layout/` 和 `styles/nav.css`。增加入口名称和图标覆盖，默认固定 11 个入口。旧 `navPins` 按 `entryOf` 映射并去重，保留顺序和分隔线。只在用户调整固定项时保存新列表，不主动重写后端偏好。个人组超过 5 个可见功能时显示两列。合并组的角标、动态图标和状态组件按来源模块设置 key，避免延迟注册导致 Hook 数量变化。页面地址、API 字段和后端校验保持不变，不加迁移。

- B246（2026-10-17）：公共组件 `components/markdown/Markdown` 的 `snapshot` 加可选的 `tables: "cards" | "wide"`。为 `cards` 时，4 列及以上的表格画成卡片（`.xc-md-cards` > `dl.xc-md-card`，第一列是 `.xc-md-card-head`），`markdown.css` 加了卡片样式。不传时和原来一样。不改接口。

- B245（2026-10-17）：公共组件 `components/markdown/Markdown` 的表格外层 `.xc-md-table` 改由 `TableBlock` 渲染，右边还有没露出来的列时加 `more` 类（截图模式不加）。`markdown.css` 加：`.more` 右边缘淡出；640px 以下第一列 `position: sticky`，其余列 `min-width: 6em`。新 CSS 变量 `--xc-md-bg` 是固定列的底色，默认 `--xc-bg`，`.xc-card` 设成 `--xc-surface`，`.xc-dialog` 设成 `--xc-elevated`；Markdown 放在别的底色上时设这个变量。不改接口。

- B243（2026-10-17）：笔记分享链接可以允许编辑。`notes.yaml` 的 `NoteShareInput` 加可选的 `allowEdit`（不传不改），`NoteShare` 加 `allowEdit`，`PublicNote` 加 `canEdit`，新类型 `PublicNoteEdit`。加公开接口 `PUT /public/notes/{token}`（访客改标题和正文，没开编辑回 403 `note_edit_disabled`，`baseUpdatedAt` 对不上回 409 `note_changed`）。迁移 `20261017130000_m62_b243_note_share_edit.sql` 给 `note_shares` 加 `allow_edit`（只加不删）。事件 `note.updated` 在访客保存时带 `source: "share"`。审计 `note.share.edit`，`note.share.put` 传了 `allowEdit` 时详情里记一笔。访客保存的标题最多 200 个字。通知 kind `note.shared_edit`。

- B242（2026-10-17）：公共组件 `components/markdown/Markdown` 加可选参数 `snapshot: { showHidden }`，截图用：没有按钮，代码块不折叠也不懒加载图片，视频音频显示成一行字，待办显示成静态的勾选框，隐藏块按 `showHidden` 展开或整块不显示。不传时和原来一样。`markdown.css` 加了这几种的样式。`lib/i18n.ts` 加 `Video`、`Audio` 两个词条。`web/package.json` 加依赖 `modern-screenshot`（锁定 4.7.0）。不改接口，不加迁移。

- 修复（2026-10-16）：待审核的 Issue 不再算到期和过期。`contracts.Issues.ListDue` 不再返回 `in_review`（每日计划和晨报跟着变）；`GET /issues?due=overdue` 不含待审核；到期通知和提醒页的 Issue 到期事项也不含待审核。退回其他状态后照常算。不改字段，不加迁移。

- 修复（2026-10-16）：公共顶栏 `components/layout/Topbar.tsx` 在有具体名称（详情页）时给 `header` 加 `has-detail` 类；`styles/ui.css` 在 720px 以下按这个类隐藏模块名、上级和分隔的“/”，只留具体名称。电脑上不变。不改接口。

- B237（2026-10-16）：基础样式 `styles/ui.css` 改了两处。单选的 `select.xc-select` 去掉系统箭头（`appearance: none`），用两段渐变画一个 `--xc-muted` 色的小三角，右内边距 26px，圆角 7px，悬停底色 `--xc-surface-2`。多选下拉框不变。模块里给 `.xc-select` 写 `background` 简写会盖掉小三角，要写成 `background-color`。顶栏页面按钮里加了 `icon` 类的 `.xc-btn` 是 28px 正方形。不改接口，不加迁移。

- B214（2026-10-15）：`browser.Session` 加 `Captcha`（找字母或数字验证码的图片和输入框，返回 `browser.CaptchaParts`）和 `ElementShot`（只截一个元素的 PNG）；`page.js` 加 `captcha`、`box`。只加不改。网站自动化的 AI 工具加 `solve_captcha`。

- B212（2026-10-15）：`contracts` 加 `AIModel`、`WithAIModel(ctx, model, effort)`、`AIModelFrom`。ai 模块 `resolveLLM` 在它有值时不管用途都用这个模型和思考程度，模型不存在或格式不对返回 `llm.ErrNotConfigured`。只加不改，不带它的调用方行为不变。`watchers.yaml` 的 `WatchersSettings` 加可选的 `agentModel`、`agentEffort`、`judgeModel`、`judgeEffort`，新类型 `WatcherEffort`（空字符串是跟随）。保存时不带字段表示不改。

- B235（2026-10-15）：`browser.Session` 加 `Shape`（从第一条记录的字段算出列表的格式）和 `Rows`（按格式读出每条记录的字段），类型 `browser.Shape`；`page.js` 加 `shape`、`rows`。只加不改。网站自动化的 `recipe` 加可选的 `format`，AI 的 `finish` 工具加可选参数 `fields`、`template`。不改接口，不加迁移。

- 修复（2026-10-15）：网站自动化选结果区域。`browser.Session` 加 `Common`（取包住几个元素的最小元素），`page.js` 加对应的 `common`，只加不改。AI 的 `finish` 工具参数从 `ref` 改成 `refs`（旧的 `ref` 仍认），先把读到的文字回给 AI 确认，再调用一次同样的区域才算选定。不改接口，不加迁移。

- B233（2026-10-15）：代理自动更新。新协议能力 `update` 和流方法 `agent.update`（`AgentUpdateParams{version, size, sha256}`，`pkg/protocol/methods_update.go`，`docs/04-agent-protocol.md` 加一节）；新包 `internal/agent/selfupdate`；代理配置 `config.Config` 加 `disableUpdate`；`cmd/agent/main.go` 加两行（`selfupdate.Register`、启动时清 `.old`）。新模块 `agentupdate`，接口 `api/modules/agentupdate.yaml`：`GET /agent-updates`、`POST /agent-updates/{agentId}`（二次验证，回 202，不在线、不支持、已是最新、正在更新、同时已有两台在更新、面板没有这个系统的代理程序回 409）、`PUT /agent-updates/{agentId}/auto`（打开要二次验证，关掉不用）。设置键 `agentupdate.disabled`（关掉自动更新的机器 id 列表）。事件 `agentupdate.changed`（`{agentId}`，浏览器默认收得到）。审计 `agent_update.run`、`agent_update.auto`。不加迁移。基础代码只在 `app/modules.go` 加一行。模块直接读 `core.Version` 当面板带的代理版本（`maintenance`、`mcp` 也这样用）。

- B208（2026-10-15）：网站自动化可以经过一台已连接的机器上网。`watchers.yaml` 的 `Watcher` 加可选的 `egressAgentId`（没选时不给），`WatcherInput`、`WatcherPatch` 加 `egressAgentId`（空字符串是去掉）。它和 `proxy` 只能选一个，同时填回 400；填 `egressAgentId` 会清掉代理，填 `proxy` 会清掉出口机器；X 任务不能选。迁移 `20261015070000_m57_b208_watcher_egress.sql` 给 `watchers` 加 `egress_agent_id`（只加不删）。新协议方法 `net.dial`（流，`NetDialParams`）和能力 `egress`（`pkg/protocol/methods_egress.go`，`docs/04-agent-protocol.md` 加一节）；新包 `internal/agent/egress`；代理配置 `config.Config` 加 `disableEgress`；`cmd/agent/main.go` 加一行 `egress.Register`（注册方法并上报能力）。服务端新文件 `modules/watchers/egress.go`（HTTP 代理，监听 `XC_BROWSER_EGRESS_LISTEN`）。新环境变量 `XC_BROWSER_EGRESS_LISTEN`、`XC_BROWSER_EGRESS_URL`，`deploy/docker-compose.yml` 的 `x-console` 服务加这两个（默认 `:8091` 和 `http://x-console:8091`，端口不映射到宿主机）。 创建或修改时带 `egressAgentId` 要二次验证（和改代理一样）。`POST /watchers/{watcherId}/run` 在出口机器离线时回 409 `egress_offline`。

- B225（2026-10-15）：`mail.yaml` 的 `MailTriage` 加必有的 `code`（验证码、一次性密码或登录码。营销一律 false）。迁移 `20261015060000_m56_b225_mail_triage_code.sql`（`mail_triage` 加 `code`，只加不删）。选了 Jev 时，类别置信度低于或等于 0.85 的那封不采用，改走原来的整封分拣。用量在 Jev 已成功返回时照记。新邮件里验证码马上推送，营销不推送。没分拣出来的仍照旧推送。

- B224（2026-10-15）：`GET /mail/todo` 只返回今天收到的要处理邮件，不再包含最近 30 天。`PUT /mail/messages/{messageId}/done` 在标成已处理时，如果这封还是未读，会在邮箱服务器上标成已读。取消已处理不改已读。服务器没存上已读时回 502，本地也不标成已处理。

- B223（2026-10-15）：`ai.yaml` 的 `ApiStyle` 加 `jev`。`AiModelSettings` 和 `AiModelSettingsInput` 加可选的 `mailTriage`。保存时不传 `mailTriage` 表示不改，传 null 表示清空。Jev 不能当 Agent 或快速模型，邮件分拣只能选 Jev。迁移 `20261015040000_m55_b223_ai_jev.sql` 给 `ai_providers.api_style` 加上 `jev`（换表，数据照搬）。新契约 `contracts.MailSorter`（键 `ai.mail_sort`），ai 模块提供，邮件分拣优先用它判类别和要不要做决定，摘要仍走 `contracts.LLM`。设置键 `ai.mail_triage_model`。用量仍记在 `ai_usage`，purpose 用 `fast`（这张表只允许 fast 和 agent），source 为 `mail`，费用按每百万输入 token 0.042 美元估算，输出不计费。`web/scripts/e2e.mjs` 第一部分加一步。

- B220（2026-10-15）：`mail.yaml` 的 `MailTriage` 加必有的 `decision` 和可选的 `doneAt`。加 `GET /mail/todo`（返回 `MailPage`，只要处理、没标成已处理、30 天内、最多 20 封）和 `PUT /mail/messages/{messageId}/done`（body `MailDone`，只改本地，没分拣过回 404）。`GET /mail/summary` 的 `latest` 带上 triage。迁移 `20261015030000_m54_b220_mail_triage_decision.sql`（`mail_triage` 加 `decision`、`done_at`，只加不删）。重新分拣不改 `done_at`。标成已处理或取消后发 `mail.updated`。

- B222（2026-10-15）：`web/src/styles/ui.css` 末尾加全局规则，`.xc-page` 不再有四周内边距，最外层的 `xc-stats`、`xc-toolbar`、`xc-card` 贴边，其他最外层内容左右留 `--xc-page-x`。只加规则，没改组件。说明见 `07-design.md` 第二节。

- B164（2026-10-15）：`contracts.MeState` 加 `PhoneActive`、`Host`（只加不改）。`me.yaml` 的 `MeState` 加 `phoneActive`、`host`，加 `POST /me/seen`。`reminders.yaml`：`DigestMode` 加 `working`；`DigestSettings` 加可选 `outDirect`、`QuietHours` 加可选 `whenAsleep`，不传时都保持原值；`DigestStatus` 加 `outDirect`、`pendingWorking`；加 `POST /notify/digest/claim`（`HeldNotification`）。迁移 `20261015020000_m53_b164_digest_hold_for.sql`（`notify_digest_queue.hold_for`）。`notify.digest_queued` 事件的载荷加 `holdFor`。基础代码：`web/src/app/GlobalPanels.tsx` 加两行（`PhoneSeen`、`HeldNotices`）。`web/scripts/e2e.mjs` 第一部分加一步，用一个临时 MCP 令牌调 `notify_send`。

- B163（2026-10-15）：`brief.yaml` 的 `BriefSettings` 加可选 `sendMode`（`time`、`wake`），不传时保持原值，旧调用方不受影响。设置键 `brief.send_mode`。`brief` 订阅总线事件 `me.woke`。`web/scripts/e2e.mjs` 第一部分加一步。

- B215（2026-10-15）：`calendar.yaml` 加 Google 账号接口（`/calendar/accounts` 增删改查、`/calendar/accounts/{accountId}/auth`、`/calendar/accounts/{accountId}/remote-calendars` 列出和添加）和公开回调 `/calendar/google/callback`；`Calendar` 加可选的 `accountId`。迁移 `20261015010000_m52_b215_calendar_accounts.sql`（新表 `calendar_accounts`，`calendars` 加 `account_id`、`remote_id`、`read_only` 三列，只加不删）。设置键 `calendar.account.<id>.client_secret`、`calendar.account.<id>.refresh_token`（加密）。通知 kind `calendar.google_expired`。基础代码只在 `files` 包加了 `files/google.go`：`GoogleEndpoints.WithDefaults`、`GoogleAuthURLFor`（按传入的权限范围生成授权地址）、`GoogleRefresh`，Drive 原有的函数没动。`web/scripts/e2e.mjs` 第一部分加一步。

- B207（2026-10-14）：`watchers.yaml` 加 `WatcherKind`（web、x），`Watcher.kind`、`WatcherInput.kind`、`WatchersStatus.x`、`WatchersSettings.xQueryIds`。不加迁移（X 任务用已有的 `kind` 列，见过的推文编号存在 `recipe` 列）。查询 `CreateChange` 改成返回编号，加 `GetChange`。新契约 `contracts.XGraphQL`（`contracts/xgraphql.go`，键 `readlater.xgraphql`，错误 `contracts.ErrXAuth`），稍后读在 `modules/readlater/xgraphql.go` 提供，`module.go` 加一行。通知 kind `watcher.x`，按钮 `watcher.save:<变化编号>` 调动作 `readlater.add`。`web/scripts/e2e.mjs` 第一部分加一步。

- B206（2026-10-14）：新模块 `watchers`，接口 `api/modules/watchers.yaml`，迁移 `20261014030000_m48_b206_watchers.sql`（表 `watchers`、`watcher_runs`、`watcher_shots`、`watcher_changes`，只加表）。设置键 `watchers.options`。事件 `watcher.updated`。通知 kind `watcher.ask`、`watcher.changed`、`watcher.failed`，按钮 id 前缀 `watcher.`。动作 `watchers.list`、`watchers.check`。新契约 `contracts.ToolChat`（`contracts/toolchat.go`，键 `ai.toolchat`），ai 模块在 `modules/ai/toolchat.go` 提供，`module.go` 加一行。`contracts/hidden.go` 加两处映射（`watchers` 归到 `monitoring`）。`browser.Session` 加 `Fill`（填进去的值在页面内容和 `Text` 里都读不到），`page.js` 按位置找元素时要求文字也对得上。基础代码只改了 `app/modules.go`、`web/src/app/routes.tsx` 各加一行。监控模块的 `MonitoringPage.tsx` 和 `NavChildren.tsx` 各加一个入口。`web/scripts/shots.mjs` 加一页，`e2e.mjs` 第一部分加一步。

- B205（2026-10-14）：新公共包 `backend/internal/server/browser`（DevTools 协议客户端、浏览器会话、页面脚本 `page.js`）和测试辅助包 `browser/browsertest`。不加依赖。`deploy/docker-compose.yml` 加 `browser` 服务（profile `browser`，镜像 `chromedp/headless-shell`，可用 `XC_BROWSER_IMAGE` 换），面板容器加环境变量 `XC_BROWSER`（默认 `http://browser:9222`）。`.env.example` 加开启说明。不开 profile 时行为不变。

- B174（2026-10-13）：`POST /music/fill-albums` 扩大为补全歌手、专辑、专辑歌手和年份。路径、operationId、返回字段及 `music.album_progress` 事件名不变。批量处理未手动编辑且未跳过的歌曲，歌手、专辑、年份任意一项缺失即可。只补空字段，专辑歌手仅取来源明确给出的值。歌手为空时用完整标题和两秒内的时长差确认，确定歌手后再筛选歌词和封面。自动匹配与用户采用候选都补这些字段。开启写回时同步标签并读回校验。候选新增信息保存在现有设置存储的 `music.candidate_metadata.<trackId>` 中，清除候选或移除歌曲时删除。没有新增数据库迁移，也没有改前端代码。

- B172（2026-10-13）：`pkg/protocol` 增加能力和方法 `net.fetch`、共享域名白名单，以及有界 GET 参数和响应。`internal/agent/conn.New` 内置注册处理器并上报 hello 能力，保持原命令兼容。音乐来源直连失败时可通过在线 Windows 代理请求，歌词和封面共用入口。来源检查增加必填 `via`（`direct` / `agent`）和可选 `agentName`、`directError`，检查忽略 10 分钟路径缓存，保留直连失败原因。没有改前端页面。

- B171（2026-10-13）：音乐增加 `POST /music/fill-albums`（`fillMusicAlbums`），返回 `202 { total }`，与后台匹配互斥时回 `409`。新增事件 `music.album_progress`，数据为 `{ running, done, total, current? }`，结束时发 `music.library_changed`。匹配采用候选时只填未手动编辑的空专辑，开启写回时同步文件专辑标签及 sha。不改标题、歌手和专辑歌手。

- B157（2026-10-11）：音乐展示组件改名为 PlayerBar.tsx、LyricsView.tsx，避免与 player.ts、lyrics.ts 在 Windows 的 TypeScript 模块解析中重号。GlobalPanels 只调整播放器的导入路径，导出和界面行为保持兼容。

- B156（2026-10-11，原提交标题用 B152）：storage 增加外部网盘新建文件夹、改名、删除和文件内容写入接口。下载增加可选 inline 参数，支持同源预览和文本内容的 ETag 校验。隐藏账号复用原有权限检查。files 包只增加 WebDAV / Google Drive 的文件操作方法，新 Google 授权增加读写范围。既有账号和备份读写方式保持兼容。
- B154（2026-10-11）：PageActions 增加详情标题后的可选插槽。项目页用它展示看板切换列表。没有使用插槽的页面布局保持原样。

改了基础代码（`backend/internal/server/` 下除 `modules/` 以外的目录、`backend/pkg/`、`web/src/{api,auth,app,components,lib}`）、
共享合同（`contracts`）或模块接口的向后兼容性时，在最上面加一条：任务编号、日期、改了什么、为什么。
看板在 [tasks.md](tasks.md)。

- B139（2026-10-11）：新模块 `knowledge`，接口 `api/modules/knowledge.yaml`，迁移 `20261012130000_m42_b139_knowledge.sql`（表 `knowledge_docs`、`knowledge_chunks`，只加表）。设置键 `knowledge.options`。新契约 `contracts.Embedder`（键 `ai.embedder`，ai 模块提供，用“快速 AI”的供应商调 `/embeddings`）和 `contracts.KnowledgeSource`（键前缀 `knowledge.sources.`，notes、readlater、journal、cards 各提供一个，分页读，不含隐藏和已归档的内容）。基础代码只改了 `app/modules.go`、`web/src/app/{routes.tsx,nav.ts}` 各加一行。\n- B138（2026-10-11）：新模块 `skills`，接口 `api/modules/skills.yaml`，迁移 `20261012120000_m41_b138_skills.sql`（表 `skills`、`skill_sessions`，只加表）。`contracts.GoalMetricSkillMinutes`、`GoalRefSkill`；`goals.yaml` 的 `GoalSource` 加 `skill_minutes`，`GoalMetric.refKind` 加 `skill`，目标页多一个数据来源。基础代码只改了 `app/modules.go`、`web/src/app/{routes.tsx,nav.ts}` 各加一行。
- B138（2026-10-11）：新模块 `skills`，接口 `api/modules/skills.yaml`，迁移 `20261012120000_m41_b138_skills.sql`（表 `skills`、`skill_sessions`，只加表）。`contracts.GoalMetricSkillMinutes`、`GoalRefSkill`；`goals.yaml` 的 `GoalSource` 加 `skill_minutes`，`GoalMetric.refKind` 加 `skill`，目标页多一个数据来源。基础代码只改了 `app/modules.go`、`web/src/app/{routes.tsx,nav.ts}` 各加一行。
- B137（2026-10-11）：新模块 `forecasts`，接口 `api/modules/forecasts.yaml`，迁移 `20261012110000_m40_b137_forecasts.sql`（表 `forecasts`，只加表）。设置键 `forecasts.options`。动作 `forecasts.add`、`forecasts.list`、`forecasts.resolve`。通知类型 `forecast.due`，用 `Immediate` 不进摘要。基础代码只改了 `app/modules.go` 加一行、`web/src/app/{routes.tsx,nav.ts}` 各加一行。
- B136（2026-10-11）：`mail.yaml` 加 `MailSummary.triage`、`MailAccount.triage`、列表参数 `category`、`POST /mail/messages/{messageId}/triage`；迁移 `20261012100000_m39_b136_mail_triage.sql`（表 `mail_triage`，只加表）。开了分拣的邮箱编号存在设置键 `mail.triage`。邮件模块对新邮件不论是否推送都先走分拣，再按 `acc.Notify` 决定推送。读 `contracts.LLM`。
- B135（2026-10-11）：新模块 `dayplan`，接口 `api/modules/dayplan.yaml`，迁移 `20261012090000_m38_b135_dayplan.sql`（表 `dayplans`、`dayplan_items`）。新契约 `contracts.DayPlanSource`（`contracts/dayplan.go`，键 `dayplan.source`，早报读）。早报 `brief` 加段 `plan`（`brief.yaml` 的 sections 枚举，`allSections` 插在 calendar 后；已经保存过段落选择的用户要在设置里手动勾选这一段）。读 `contracts.Calendar`、`Issues.ListDue`、`Reminders.Upcoming`。设置键 `dayplan.options`。通知 kind `dayplan.draft`、`dayplan.start`、`dayplan.review`，按钮 id 前缀 `dayplan.`。今日页加卡片 `dayplan`。
- B134（2026-10-11）：新模块 `inbox`，接口 `api/modules/inbox.yaml`，迁移 `20261012080000_m37_b134_inbox.sql`（表 `inbox_items`）。基础代码：`contracts.TelegramMessage` 加 `Voice` 和 `Download`；新键 `contracts.TelegramInboxFallbackKey`，reminders 的 Telegram 处理先问稍后读（原有键），它不处理再问收集箱，向后兼容，但没有链接的普通文字现在由收集箱收下并回复。新契约 `contracts.Transcriber`（`contracts/transcribe.go`，键 `ai.transcriber`，ai 模块提供，用“快速”供应商的 `/audio/transcriptions`）。reminders 加 `telegramFile`（getFile 并下载，最大 20 MB）。设置键 `inbox.options`、`inbox.last_review`。通知 kind `inbox.suggest`、`inbox.review`，按钮 id 前缀 `inbox.`。
- B133（2026-10-11）：新模块 `experiments`，接口 `api/modules/experiments.yaml`，迁移 `20261012070000_m36_b133_experiments.sql`（表 `experiments`、`experiment_metrics`）。新契约 `contracts.BodySeries`（`contracts/experiments.go`，键 `habits.body_series`，习惯模块提供每日身体数据）。读 B132 的 `contracts.CheckinSeries` 和 B128 的 `contracts.GoalMetricSource`（逐日取数）。`checkins.yaml` 的平均分改成 double。
- B132（2026-10-11）：新模块 `checkins`，接口 `api/modules/checkins.yaml`，迁移 `20261012060000_m35_b132_checkins.sql`（表 `checkin_questions`、`checkin_answers`、`checkin_asks`，并写入两个默认问题）。新契约 `contracts.CheckinSeries`（`contracts/checkins.go`，键 `checkins.series`，B133 自我实验读它）。设置键 `checkins.options`。通知 kind `checkin.ask`，按钮 id 前缀 `checkin.`。
- B131（2026-10-11）：新模块 `focusblock`，接口 `api/modules/focusblock.yaml`，迁移 `20261012050000_m34_b131_focus_block.sql`。协议新增 `pkg/protocol/methods_focusblock.go`（`focusblock.apply`、`focusblock.clear`、`focusblock.status`，能力 `focusblock`），代理新包 `internal/agent/focusblock`，`cmd/agent/main.go` 的 register() 加一行。新契约 `contracts.FocusCurrent`（`FocusCurrentKey`，番茄钟模块提供）和 `contracts/screentime.go`（电脑时间模块提供“娱乐”类应用名单）。设置键 `focusblock.settings`。
- B130（2026-10-11）：新模块 `cards`，接口 `api/modules/cards.yaml`，迁移 `20261012040000_m33_b130_cards.sql`。新契约 `contracts.CardsDue`（`contracts/cards.go`），早报模块 `brief` 加“待复习”段和相应设置项（`api/modules/brief.yaml`）。依赖 `github.com/open-spaced-repetition/go-fsrs/v4`。笔记编辑器和稍后读详情页各加一个“做成卡片”入口（`SelectionCard`）。今日页加卡片 `cards`。
- B129（2026-10-11）：新模块 `codingroi`（只读汇总），接口 `api/modules/codingroi.yaml`，迁移 `20261012030000_m32_b129_codingroi.sql`（表 `codingroi_outcomes`）。`contracts/hidden.go` 加两处映射（`codingroi` 归到 `coding`）。读 `coding_*`、`ai_usage`、`ai_agents`、`screen_minutes`、`github_cache_v2` 表。
- B128（2026-10-11）：新模块 `goals`，迁移 `20261012020000_m31_b128_goals.sql`（表 `goals`、`goal_results`、`goal_result_logs`、`goal_reviews`）。新契约 `contracts.GoalMetricSource`（`contracts/goals.go`，键前缀 `goals.metrics.`，错误 `ErrGoalMetricMissing`），habits、focus、screentime、github 各加 `goal_metrics.go` 提供数据。设置键 `goals.options`、`goals.last_review_notice`。今日页加卡片 `goals`。
- B127（2026-10-11）：`notify/notify.go` 向后兼容地加 `Notification.Immediate`（默认 false）、`notify.Holder` 接口、`Service.SetHolder`、`Service.DeliverNow`；没装 Holder 时行为不变。reminders 加接口 `GET/PUT /notify/digest`、`POST /notify/digest/flush`，迁移 `20261012010000_m30_b127_notify_digest.sql`（表 `notify_digest_queue`），设置键 `notify.digest`、`notify.digest_state`，事件 `notify.digest_queued`、`notify.digest_sent`、`notify.digest_updated`。合并推送跳过被 B113 静音规则挡住的通知。
- B151（2026-10-11）：`drive.yaml` 加公开接口 `PUT /drive/upload-links/{token}`、`GET /drive/download-links/{token}`，任务种类加 `download`。迁移 `20261011000500_m29_b151_drive_links.sql`（表 `drive_upload_links`、`drive_download_links`）。动作注册表 `actions.Action` 加 `MCPOnly` 字段，`Registry.List` 不返回它，新增 `Registry.ListForMCP` 返回；`mcp` 模块的 `tools/list` 改用后者。设置键 `drive.mcp_max_upload`（默认 1 GB）。
- B150（2026-10-11）：新增契约 `contracts.FocusState`（番茄钟模块提供，音乐模块用它判断是否在专注期间）。`music.yaml` 加 `GET /music/stats`，设置加 `focus`。迁移 `20261011000400_m28_b150_music_plays.sql`（表 `music_plays`）。今日页 `overview/layout.ts` 加卡片 `music`，`TodayPage.tsx` 加一行显示条件（播放条里有歌才显示）。

- B149（2026-10-11）：`music.yaml` 加定时暂停（`/music/sleep`）、播放列表分享（`/music/playlists/{id}/shares`、`/music/shares/{id}`）和公开接口（`/public/music/{token}` 下）。音乐模块实现 `PublicPaths`（`/public/music/`），公开入口自己校验链接和密码。前端 `App.tsx` 加 `/m/<token>` 分享页（在登录检查之外渲染，和 `/s/`、`/n/` 一样）。迁移 `20261011000300_m27_b149_music_shares.sql`。

- B148（2026-10-11）：`contracts.DriveFiles` 加 `ReplaceContent`（换文件内容，不留旧版本，带原哈希防止盖掉新上传）和 `ErrDriveChanged`；云盘模块只在 `contract.go` 里加了实现，原有行为不变。`music.yaml` 加匹配、待确认、手动歌词和封面接口，设置加 `autoMatch`、`writeBack`、`providers`。迁移 `20261011000200_m26_b148_music_match.sql`（表 `music_candidates`）。\n\n- B146（2026-10-11）：新增 `contracts.DriveFiles`（云盘提供，音乐模块用来读云盘文件）。云盘模块只加了 `contract.go` 和 `module.go` 里的一行注册，原有行为不变。新增 `api/modules/music.yaml`、迁移 `20261011000100_m25_b146_music_library.sql`（表 `music_tracks`、`music_playlists`、`music_playlist_items` 和全文索引 `music_fts`）、依赖 `go.senan.xyz/taglib`。

- B145（2026-10-11）：新增纯展示组件 `components/ui/BrandMark.tsx`。登录页、左栏和云盘分享页共用 D 方案站标，页面内跟随现有主题色。保留左栏按钮的可读名称、导航和点击事件。浏览器与 PWA 图标加版本参数并更新离线缓存版本。只改展示和静态资源，不改接口。

- B113（2026-10-06）：`reminders.yaml` 加 `GET/POST /notify/mutes`、`PUT /notify/mutes/scope`、`DELETE /notify/mutes/{muteId}`。迁移 `20261006000200_m7_b113_notification_mutes.sql`（表 `notification_mutes`）。基础代码：`notify.Notification` 加可选字段 `Scope`（邮件新邮件填 `mail:<邮箱编号>`），向后兼容；`reminders` 的路由 `Route` 和 Web Push 发送按规则过滤。总线事件 `notify.scope_removed`（载荷 `{scope}`）：拥有这个范围的模块在范围被删除时发，`reminders` 收到后删掉规则。邮件模块删除邮箱时发这个事件。

- B112（2026-10-06）：`quotas.yaml` 加 `GET/PUT /quotas/notify`（设置键 `quotas.notify`，四个布尔值，默认都开）、账号的 `balanceLow`。迁移 `20261006000100_m14_b112_quota_notify.sql`：`quota_accounts.balance_low`、`quota_readings.fail_count`、表 `quota_notify_state`。通知 kind：`quota.low`、`quota.empty`、`quota.balance_low`、`quota.read_failed`，用现有的通知路由选渠道。

- B110（2026-10-06）：`pkg/protocol` 新增 `methods_quota.go`（能力 `quota`，方法 `quota.read`，错误码 `quota_signed_out`、`quota_unavailable`），只加不改。新增 `api/modules/quotas.yaml`、迁移 `20261006000000_m14_b110_quotas.sql`（`quota_accounts`、`quota_readings`）、模块 `quotas`（`app/modules.go`、`sqlc.yaml`、`cmd/agent/main.go` 各加一行）。代理新包 `internal/agent/quota`。服务端事件 `quota.updated`（载荷 `{id}`）供 B112 使用。

- B103（2026-10-05）：`components/layout/Sidebar.tsx` 新增导出 `useCurrentNavPanel()`，二级菜单标题栏加收起按钮（`.nav-panel-collapse`）。`Topbar.tsx` 在桌面上只有二级菜单收起时才显示展开按钮（手机上仍是打开抽屉）；二级菜单显示时顶栏加 `.panel-open`，模块图标和模块名（`.topbar-module`）不显示。样式在 `ui.css`。

- B100（2026-10-05）：`components/layout/Topbar.tsx` 页面名前加模块图标，去掉 ⌘K 按钮，`openPalette` 参数改成可选（不再使用）。`styles.css` 的顶栏改成 58px 加下边线，`.xc-page` 上边距从 4px 改成 20px（手机 16px）。

- B99（2026-10-05）：`components/layout/Sidebar.tsx` 重写成图标栏加二级菜单，外层仍是 `.sidebar`（脚本按它找左栏），菜单项是 `.nav-rail-item`，链接的可读名称是模块名（`aria-label`）。新增导出 `moduleForPath()`。`lib/navChildren.ts` 的 `NAV_CHILD_LIMIT` 从 5 改成 30。`stores/sidebar.ts` 的 `collapsed` 现在表示二级菜单收起。登记接口（`registerNavChildren`、`registerNavBadge`、`registerNavIcon`、`registerNavStatus`、`registerNavAction`）不变，显示位置变了。旧左栏的样式从 `styles.css`、`ui.css`、`theme.css`、`refinements.css`、`motion.css` 删掉，新样式在 `styles/nav.css`。

- B98（2026-10-05）：`core.yaml` 的主题色枚举改成 `indigo, ocean, teal, violet, rose, graphite`，默认 `indigo`。旧值 `ember`、`mint` 仍然接受，服务端读写时换成 `indigo`、`teal`。前端用 `normalizeAccent()` 做同样的转换。基础代码改了 `styles/tokens.css`（颜色值、新增 `--xc-on-accent`、`--xc-sans`）、`styles/ui.css`（按钮、卡片、概要条、分段切换、标签）、`styles/styles.css` 和 `theme.css`（内容区不再是浮起的圆角面板），字体改成跟前端打包的 Geist。

- B90（2026-10-03）：`brief.yaml` 的地点和早报位置增加可选 `id`。地点搜索支持坐标反查。天气查询需要和风配置，旧天气地址字段不再使用。

- B84 验收修复：`GitIssues` 加 `Issue`（只读一个 Issue，回调用），`GitIssue` 加 `PullRequest`。新增 `BoardGitUnbinder`：删除 Git 连接前先解除这个连接绑定的看板，卡片留在看板上变成普通卡片。看板同步改成先在锁外读 Git 服务，再加锁写库；回写 Issue 不再占锁。

- B86 验收修复：`CodingControl` 加 `CodingTasks` 和 `CodingTaskEvents`，新增 `AgentRuns.LinkRunTask`。`aiagents` 不再直接读 `coding_tasks`、`coding_task_events`，`coding` 不再直接写 `ai_agent_runs`。`finishRun` 只收尾一次。

- B91：公开 GET /notify/icons/{name} 用 HMAC 校验。Reminder 的输入、输出和补丁增加可选 icon。CreateReminder 增加可选 Icon，已有调用不受影响。

- B87：contracts 增加 ToolDecider、CodingQuestions 和 AssistantDecisions。CodingRunParams 增加可选 allowQuestions，CodingDone 增加可选 question。旧代理和非 Agent 编码流程照旧。

- B86：contracts 新增 CodingControl 和 ToolObserver。LaunchCoding 增加内部 runId 和自动提 PR 标志。用于复用现有编码流程并记录 Agent 执行，保留原调用方式。

- 2026-10-02（Claude，第四批前端）：
  - `lib/navBadges.ts` 加 `registerNavIcon`（换一级菜单图标，或者 `state: "working"` 让图标跳动）和 `registerNavStatus`（一级菜单右边的状态点和短文字）；`components/layout/Sidebar.tsx` 渲染它们；`styles/ui.css` 加 `.nav-icon`、`.nav-status`、`.nav-child-spin`（B86、B88、B93）。
  - `components/layout/NavChildLinks.tsx` 的链接加可选的 `busy`，名称右边转圈（B86）。
  - `styles/ui.css`：`.nav-child > .xc-flag` 不撑开（左栏服务器国旗和名称之间空很大）。

- 2026-10-02（Claude，部署后页面打不开）：`app/app.go` 的 `spa` 对 `/assets/` 下找不到的文件回 404，不再回 `index.html`；`public/sw.js` 不缓存 HTML 冒充的资源，缓存名改成 v2 清掉旧缓存；新增 `lib/chunkReload.ts`，`components/ui/ErrorBoundary.tsx` 和 `lib/errors.ts` 遇到按需加载的文件取不到时自动刷新一次（30 秒内只刷一次）。

- 2026-10-02（Claude，第三批前端）：
  - `app/App.tsx` 加 `/n/<token>` 笔记分享页，`app/GlobalPanels.tsx` 加 `FloatingNotes`（B72，提交在 develop 上的 `e68144a`）。
  - `components/layout/NavChildLinks.tsx` 加可选的 `onReorder`，传了就能拖动排序；`styles/ui.css` 加 `.nav-child.dragging`、`.drag-over`（B82）。
  - `stores/page-title.ts` 加 `mark` 和 `usePageMark`，`components/layout/Topbar.tsx` 在详情名称左边显示它（B82 国旗）。
  - `components/ui/MoreMenu.tsx` 加可选的 `icon`（B74 编辑器“插入”菜单用加号）。
  - `components/markdown/`：解析和渲染加表格、提示块、隐藏块、高亮、视频音频、链接卡片、代码块折叠和复制；新增 `InsertMenu.tsx`；`attachmentMarkdown` 对视频、音频也用图片的写法（B74）。`lib/i18n.ts` 加这些的全局词条。
  - 新增 `hooks/useKeepScroll.ts`；`app/Layout.tsx` 记每个模块最后的地址和主体滚动位置，去掉手机上换页时滚到顶部（改由恢复逻辑处理）；`components/layout/Sidebar.tsx` 一级菜单链接用 `lastPathFor`；`api/query.ts` 的 `gcTime` 改成 30 分钟（B80）。
  - `vite.config.ts` 用 `define` 注入 `__XC_VERSION__`、`__XC_BUILT_AT__`，新增 `types/build.d.ts`；`deploy/Dockerfile` 前端阶段加 `ARG VERSION` 传给 `XC_VERSION`（B79）。vitest 在 CI 上不限线程（B78）。
  - `.github/workflows/ci.yml`：前端拆成 `web` 和 `e2e` 两个任务，端到端用自带的 Chrome；PR 上按改动目录跳过前端或后端检查（B78）。
  - 后端 `httpx.ClientIP` 改成从右往左找第一个不可信地址（审查修复 `9619701`）；`notes.plainText` 去掉隐藏块和新语法的符号（B74）。

- B81：backup 增 mode、retention、快照、变化、检查和 job.warning/check/prune。cmd/server 在打开数据库与监听前同步执行现场备份、替换数据库和文件恢复；app 初始化同样拒绝未应用的恢复数据库。复用 storage.WithCleanup 阻止备份与存储迁移、清理并发。files.WebDAV 对无法读取的属性、非法路径和扫描中消失的子目录报错，防止不完整扫描导致备份漏文件或误清理。web/scripts/e2e.mjs 仅增 API 主流程。

- B83：新增 contracts.Presence、代理 presence.get/update 和 notify.show。hosts 提供内存状态及详情字段，habits 增作息表和提醒字段。app 对 habit.presence 同时检查习惯和所属电脑的隐藏状态，避免跨模块泄露。SQL 生成模型同步新增表。web/scripts/e2e.mjs 只增 API 主流程，并把请求助手移到首次使用之前。

- B82：新增 HostPairingInfo、host_info 加密资料与配对侧表。agenthub 将配对消费和资料保存放入同一事务，新增代理令牌鉴权的 whoami。config 新增 XC_TRUSTED_PROXIES，notes/drive 统一来源 IP。SystemInfo 地址字段保持旧代理兼容。sqlc 复制新增表到各模块生成模型。维护总览增加 DB-IP 来源。

- B79：新增 maintenance API、StorageReporter/Cleaner 和原始存储检查契约。app 注册维护模块及连接统计，ws/agenthub 新增只读连接数，core.BuiltAt 由 Dockerfile 服务端编译注入。公共上传和云盘定时清理复用安全重查，临时文件登记只保护仍在使用的文件。web/scripts/e2e.mjs 仅新增 API 主流程，未改前端功能。

跨模块的接口（`internal/server/contracts`、基础包、协议）有调整时记在这里。

| 日期 | 变更 | 原因 |
| --- | --- | --- |
| 2026-10-02 | 新增 `contracts.GitIssues`、`BoardGit` 和独立看板 Git 回调入口。复用 aiagents 的令牌客户端，projects 不读取或保存令牌 | B84 看板绑定仓库 |
| 2026-10-02 | `drive.yaml` 放宽 code 为 4 到 32 个字符，公开内容新增 preview，保留 inline；新增下载记录和公开缩略图接口，PublicShareItem 新增 thumbnail；事件 `drive_share.changed` 刷新分享列表 | B75 |
| 2026-10-02 | 新增 `contracts.GitWebhookReceiver`（键 `github.webhook`），aiagents 验证签名后同步调用 github；notify 新增 `SaveTx` 和 `Dispatch`，普通 `Send` 行为兼容，供通知与去重同事务提交 | B71 持久去重和回调失败重试 |
| 2026-10-01 | 新增 `lib/navBadges.ts`（`registerNavBadge`、`registerNavAction`）；`components/layout/Sidebar.tsx` 在一级菜单右边显示数量、行内按钮；`ui.css` 加 `.nav-badge`、`.nav-action`，二级菜单选中改成强调色，`.xc-list` 去掉 `ul` 默认缩进 | B76 左栏数量、B72 笔记“+” |
| 2026-10-01 | `app/nav.ts`：GitHub 挪到“主要”组 Agent 后面，改名 `Repositories`（仓库），图标 `FolderGit2`，地址不变；设置标签改名 `Git & repositories`；`github.yaml` 加 `watches`、`/github/repos`、`/github/commits`、`/github/runs/{runId}/jobs`、`/github/notify`，PR、运行、Issue 加 `connectionId`、`forge`（后端占位在 `github/pending.go`，`ListGitHubIssues` 多了一个不用的参数） | B70、B71 |
| 2026-10-01 | `contracts` 加 `remotes.go`：`RemoteDrives`（键 `storage.remotes`，storage 提供，backup 用）和 `RemoteUser`（键 `backup.remote_user`，backup 提供，storage 用来拦删除）。storage 模块加表 `storage_remotes`、`ServiceKey`、`UseGoogle`（测试用）。备份设置的 `target` 加 `remote` 和 `remoteId`，`webdav`、`gdrive` 两段只返回目录；`/remote-drives*`、`/backups/gdrive/auth` 标成过时，下个版本删。新加开发工具 `backend/cmd/fakedav`（端到端测试用的内存 WebDAV，不进发布） | B69 |
| 2026-10-01 | `contracts/hidden.go` 加 `router`（接口、事件 `router.`、通知链接 `/router`、来源 `router`），`vault.yaml` 的 `ModuleId` 加 `router`。前端 `app/nav.ts`、`app/routes.tsx`、`app/modules.ts` 各加一行；今日页 `overview/layout.ts` 和 `TodayPage.tsx` 加 `network` 卡片；`scripts/shots.mjs` 加两个页面 | B65 |
| 2026-10-01 | `contracts.AIAgents` 加 `ForHost`，`AIAgent` 加 `HostIDs`。ai 模块加 `withEffort`（按会话覆盖思考程度），和 B47 的 `withModel` 一样走 context | B60 |
| 2026-10-01 | `contracts` 加 `Memories`（`Prompt(ctx)`，键 `ai.memories`），ai 提供，aiagents 用。`actions.Action` 加 `PanelOnly`：只给面板 AI，`AllowedFor` 一律不给，自动化的目录和执行也跳过 | B61 |
| 2026-10-01 | `contracts` 加 `GitAccounts`（`Account`、`Credentials`、`ImportGitHub`，键 `aiagents.accounts`），aiagents 提供，github 用。github 设置键加 `github.connection_id`、`github.migrated_b62`；`useGithubModule` 和 `/github/config` 的 token、clearToken、apiUrl 标成 deprecated | B62 |
| 2026-10-01 | 基础代码 `files` 加两个 `Store` 实现：`WebDAV`（`NewWebDAV`、`ValidateWebDAV`、`Check`）和 `GDrive`（`NewGDrive`、`Folder`、`Account`、`Check`），以及 Google OAuth 的 `GoogleAuthURL`、`GoogleExchange`、`GoogleRevoke`、`ErrGDriveAuth`。只加新文件，不改已有的。测试用的假 Drive 在 `files/fakegdrive` | B63 |
| 2026-10-01 | `files.WebDAV` 加 `ReadDir`，`files.GDrive` 加 `ReadDir`、`Trail`、`OpenFile`，加 `DirEntry`。`GoogleExchange` 改成返回 `GoogleGrant`（带授权范围），授权地址多申请 `drive.readonly`。vault 的 `ModuleId` 加 `drive-webdav`、`drive-gdrive`。前端 `NavChildLinks` 的缩进子项默认收起，状态记在 localStorage 的 `xc.nav.open3` | B68 |
| 2026-10-01 | 前端 `moduleOfPath` 把 `/calendar/briefs` 算作不属于任何模块；左栏在早报页高亮“今日” | B66 |
| 2026-10-01 | `contracts` 加 `IgnoreHidden(ctx)`、`HidingIgnored(ctx)`。自动化执行步骤时带上这个标记，被隐藏模块的步骤照常执行。锁定时用到被隐藏模块的自动化规则在页面上按不存在处理 | B57 验收：自动化在后台没有会话，原来被隐藏模块的步骤一直失败 |
| 2026-10-01 | `contracts` 加 `HiddenModules`（`Hidden(ctx, module) bool`，键 `vault.hidden`）。vault 模块提供。锁定时被隐藏的左栏模块接口回 404，动作目录、事件、通知和早报按同一张对应表跳过 | B57 隐藏模块 |
| 2026-10-01 | 删掉 B45 的 `?safari=` 测试开关、`SafariProbe` 和 `browser.css` 里 C 到 F 的试验样式。页面样式保持测试前的样子 | 真机上 `all` 和 `off` 都可以，A 到 F 单独开不行 |
| 2026-10-01 | `store` 加 `Snapshot`、`OpenSnapshot`（把已经迁移好的内存库复制出来）。`testutil.openDB` 每个测试进程只迁移一次，后面的测试从这份快照复制。测试里直接调用 `store.Open(":memory:")` 的没有改 | 带 -race 时每次迁移要几秒 |
| 2026-09-30 | 项目改成多看板：新表 `project_boards`、`board_lists`、`issue_members`、`issue_activity`，`issues` 加 `board_id`、`list_id`、`archived_at`、`cover_file_id`；`POST /issues/{key}/move` 可以只传 `listId`（`status` 变成可选）；`Issue` 加 `boardId`、`listId`、`archivedAt`、`members`、`commentCount`；归档的卡片不出现在列表、到期提醒和提醒页里；B36 的分类界面去掉，接口保留；界面上 Issue 改叫“卡片” | B46 多看板 |
| 2026-10-01 | 认证中间件只在 `/api/v1/mcp` 上接受 `Authorization: Bearer xc_…` 的 API 令牌，别的路径照旧（代理的 `/agent/connect` 也用 Bearer，不受影响）；`auth.Session` 加 `Token *TokenInfo`，`auth.TokenFrom(ctx)` 取令牌；`actions` 加 `Module`、`Deletes`、`AllowedFor`（危险动作和别名永远不开放）；新模块 `mcp`：`/api-tokens` 增删查、`/api-tokens/tools`、`/api-tokens/calls`、`POST /mcp`；设置加“远程访问”页签 | B43 远程 AI 操作 |
| 2026-10-01 | 新模块 `aiagents`：表 `ai_agents`、`git_connections`；`coding_repos` 加 `connection_id`、`owner`、`repo`、`clone_url`、`build_config`，`coding_tasks` 加 `ai_agent_id`、`model`、`permission`、`build_status`、`build_attempts`、`artifacts`，`issue_comments` 加 `author`（这两个 id 列没加外键，删除时由代码置空）；新接口在 `contracts/aiagents.go`：`GitConnections`、`AIAgents`、`GitHubCredentials`（GitHub 模块提供） | B47 Agent 管理 |
| 2026-10-01 | 代理协议：能力 `coding.remote`，新方法 `coding.ensure_repo`，`CodingRunParams` 加 `model`、`permission`、`preferRemote`，`CodingPushParams` 加 `auth`（见 `docs/04-agent-protocol.md`）；`contracts.LaunchCoding` 加 `AIAgentID`、`AgentID`；coding 接口：`CreateRepo` 的 `path` 改成可选，加 `connectionId`、`remoteRepo`、`cloneUrl`，`CreateTask` 的 `executor` 改成可选，加 `aiAgentId`、`agentId` | B47 Agent 管理 |
| 2026-10-01 | 代理协议：新方法 `coding.build`，`CodingRunParams` 加 `continue`、`baseCommit`；`coding_tasks` 加 `build_error`；coding 接口：`PUT /coding/repos/{id}/build-config`（要提升权限）、`POST /coding/tasks/{id}/build`、`GET /coding/tasks/{id}/artifacts/{index}`，`Repo` 加 `buildConfig`，`Task` 加 `buildStatus`、`buildAttempts`、`buildError`、`artifacts`；事件 `coding_task.build` | B47 Agent 管理 |
| 2026-10-01 | `contracts/aiagents.go` 加 `IssueWork`（项目模块提供：卡片摘要、带作者的评论、加成员）和 `ToolRunner`（AI 模块提供：内置 Agent 按 B43 的权限过滤调用动作）；AI 模块 `resolveLLM` 支持用 context 覆盖模型；项目接口 `Comment` 加 `author`；`POST /ai-agents/{id}/assign`；公开入口 `POST /hooks/git/{connectionId}`（校验 GitHub、Forgejo、Gitea 的签名）；Git 连接的新建、换令牌、查看回调密钥改成“始终验证” | B47 Agent 管理 |
| 2026-10-01 | 左栏“Agent 任务”改成“Agent”（`app/nav.ts`，`lib/i18n.ts` 加 `Agents`）；`/coding` 改成 Agent 管理页，原来的任务列表移到 `/coding/tasks`，新页面 `/coding/connections`、`/coding/agents/:id`；今日页、卡片页里指向任务列表的链接跟着改；看板卡片的 Agent 成员显示头像，最近一次任务失败时有红点；前端新模块 `features/aiagents` | B47 Agent 管理 |
| 2026-09-30 | `auth.Session` 加 `ElevationMode`、`ViaToken`，`Elevated()` 按设置 `security.elevation_mode` 算；新增 `auth.RequireStrictElevated`（始终 5 分钟内验证过）；开启两步验证和从备份恢复改用它；`core.yaml` 加 `/auth/elevation-mode` | B48 二次验证可选 |
| 2026-09-30 | `contracts` 加 `WithAIUsage`、`AIUsageFrom`（给 AI 调用标来源）和 `AIUsageRecorder`（键 `ai.usage`，记 Agent 任务等外部用量）；`llm.Result` 加缓存和思考 token；代理的 Claude Code 解析把 `usage` 带给服务端 | B42 AI 用量 |
| 2026-09-30 | `styles/tokens.css` 加字号令牌 `--fs-9` 到 `--fs-19` 和 `--fs-input`，手机上放大；全部样式里 9 到 19px 的 `font-size` 换成令牌（`scripts/font-tokens.mjs`）；手机上输入框一律 16px；CI 加 `npm run lint:fonts`；`shots.mjs` 在 390px 下检查字号；`e2e.mjs` 支持 `XC_SHOTS_BROWSER` | B44 手机字号 |
| 2026-09-30 | `index.html` 首屏脚本读 `?safari=` 测试开关，存 `sessionStorage`，写到 `<html data-safari>`；`styles/browser.css` 末尾加 C 到 F 的开关样式；`usePreferenceEffects` 在开关 B 下不写 `theme-color`；`Layout` 顶部加 `SafariProbe`。定稿后删掉没用的开关 | B45 第一步，真机测试用 |
| 2026-09-30 | `httpx.Fail` 的报错 JSON 加 `requestId`，新增 `httpx.ExposeRequestID` 中间件写响应头 `X-Request-Id`；5xx 的业务错误也写日志。`api/common.yaml` 的 `Error` 加 `requestId`。前端 `ApiError` 加 `request`（方法、路径、响应、请求编号），`toast({tone:"error"})` 转到 `lib/errors.ts` 的报错列表，`api/query.ts` 加全局 `onError`，右下角提示改由 `app/NoticeStack.tsx` 渲染 | B41 报错统一显示 |
| 2026-09-30 | `actions.Action` 加 `AliasOf`：别名照样能 `Get`、`Run`，但 `List` 不返回。项目模块的 `issues.list/get/create/update` 标成 `projects.*` 的别名 | 审查修复：AI 工具重复 |
| 2026-09-30 | 新增 `useBrowserViewport` 和 `styles/browser.css`，统一动态高度及安全区；手机普通页面改为文档滚动，切换路由回到顶部；`usePreferenceEffects` 按 CSS 主题背景更新浏览器主题色 | B38 跨浏览器与主屏幕应用适配 |
| 2026-09-30 | `auth.Service` 加 `SessionActive(ctx)`：会话还在且没过期时为真。远端日志跟随和云盘日志跟随每 30 秒查一次，退出登录或改密码后断开；远端日志跟随最长 1 小时 | 审查修复：日志跟随在退出登录后还在推送 |
| 2026-09-30 | `protocol.FilesWriteParams` 加 `Private`（`omitempty`，新文件建成 0600）和能力 `files.private`；代理 `ServeWrite` 支持它，`cmd/agent/main.go` 加一行报这个能力；`hostagent.Runner` 加 `BackupBase`（测试用） | 审查修复：机器 Agent 的备份别人可读 |
| 2026-09-30 | `logfollow` 加 `UTF8Prefix`，远端日志和云盘日志都只发完整字符，云盘去掉自己的 `utf8Prefix` | 审查修复：远端日志跟随中文乱码 |
| 2026-09-30 | `contracts` 新增 `ExternalReminder`、`ReminderSource` 和注册表前缀 `reminders.sources.`；监控与项目提供到期事项，提醒模块按来源汇总 | B37 提醒页汇总 |
| 2026-09-30 | `contracts` 新增 `Files`、`FilesKey`（`files.files`）；公共上传模块提供 Markdown 图片认领和按归属清理，项目、日历、提醒、Agent 任务接入 | B36 公共图片上传 |
| 2026-09-30 | `contracts` 新增 `LLM`、`LLMKey`（`ai.llm`）。AI 模块提供统一调用层，自动化和笔记按用途调用；浮窗、早报也使用同一调用层 | B32 OpenAI 兼容接口 |
| 2026-09-30 | 云盘公开入口 `/public/shares` 已启用并自行校验令牌、提取码和范围；发 `drive_share.changed`、`drive_task.updated`、`drive_item.batch` 事件；云盘与远端文件日志共用 `logfollow` 帧 | B31 云盘后端 |
| 2026-09-29 | 新增基础包 `internal/server/files`（`Store`、`Local`、`Scoped`、`Manager`、`OpenSeeker`、`MigrateLegacyLayout`）；`module.Deps` 加 `Files *files.Manager`；`config.Config` 加 `FilesDir()`、`TmpDir()`；`app.New` 启动时把旧目录 `data/drive`、`data/notes/attachments` 搬到 `data/files/`，并清空 `data/tmp/`；云盘和笔记改用 `d.Files.For(...)` | B24 统一文件目录 |
| 2026-09-29 | `core.Handlers` 加 `Settings`、`Bus`（偏好设置用）；`app.New` 传入 | B22 偏好设置后端 |
| 2026-09-29 | 新增模块 `modules/storage`（`app/modules.go` 加一行）；`config.Config` 加 `FilesCacheDir()`；启动时把云盘旧的 S3 设置复制到 `storage.s3`；`files.NewCached` 的上限 0 表示不缓存、负数表示不限 | B24 存储位置设置和搬迁 |
| 2026-09-29 | 新增模块 `modules/backup`（`app/modules.go` 加一行）；`config.Config` 加 `BackupsDir()`、`RestoreDir()`；`cmd/server/main.go` 在打开数据库之前调用 `backup.ApplyPending`（换上待恢复的数据库）；`storage` 导出 `S3Config` 给备份用，搬迁和用量统计跳过 `backups/`；`files.S3.Put` 遇到未知大小时用 16 MiB 分片 | B25 备份和恢复 |
| 2026-09-29 | `protocol.MetricsDetailParams` 加 `IntervalMs`（`omitempty`，旧代理忽略）；浏览器 WebSocket 加控制消息 `interval {hostId, ms}`（`ws/events.go`），`ws.Handler` 按各连接要求的最小值调用代理，并在进程内发事件 `host.metrics_interval {hostId, ms}`（不转发给浏览器）；`hosts` 模块订阅它，1 秒模式下每个样本都推，环形缓冲只在这个模式下每 4.5 秒合并成一格，保持一小时的历史；代理加 `Collector.SampleFast` | B26 刷新周期 |
| 2026-09-29 | `protocol.MetricsSample` 和 `MetricsSummary` 加 `netRxTotal`、`netTxTotal`（`omitempty`，累计字节数，只算 `protocol.CountedInterface` 认可的网卡；旧代理不传）；新增迁移 `20260929000300` 三张表 `host_traffic_hourly`、`host_traffic_daily`、`host_traffic_plans`；`hosts` 模块每次样本进内存累加器，每分钟落库，进程内新事件 `host.traffic_plan_changed {hostId}`；`hosts.cleanup` 删 7 天前的小时表 | B27 月流量 |
| 2026-09-29 | `protocol.DockerLogsParams` 加 `Lines`，新增 `DockerLogLine`、`DockerImageRemoveParams`、`DockerImagePruneResult`、方法 `docker.image_remove`、`docker.image_prune` 和能力 `docker.lines`（代理 `capabilities()` 在有 Docker 时一起报）；`monitoring/pending.go` 删除，`RemoveImage`、`PruneImages` 在 `monitoring/images.go` | B28 容器日志来源、镜像清理 |
| 2026-09-29 | 新增协议 `pkg/protocol/methods_syslog.go`（`syslog.query`、`syslog.units`、`syslog.follow`、错误码 `syslog_permission`）和能力 `syslog`；新增代理包 `internal/agent/syslog`，`cmd/agent/main.go` 各加一行；`hosts` 模块的 `agentErr` 加一个错误码映射，`GetSyslog`、`ListSyslogUnits`、`FollowSyslog` 从 `pending.go` 挪到 `hosts/syslog.go` | B29 系统日志 |
| 2026-09-29 | `config.Config` 加 `AgentsDir`（`XC_AGENTS_DIR`）；`core.Handlers` 加 `PublicURL`、`AgentsDir`；`agenthub.Hub` 加 `PairingCodeValid`（只查不用掉）；新增 `core/agentinstall`（脚本模板和 `AppendSetup`）；`app.go` 的 `corePublic` 加 5 个路径；`core/pending.go` 删除；`testutil` 加 `NewWithConfig`；`deploy/Dockerfile` 打包四个平台的代理；CI 加 shellcheck；代理新增 `internal/agent/setup`（Windows 安装模式），`cmd/agent/main.go` 把 `pair` 的主体拆成 `doPair` | B30 一条命令添加服务器 |
| 2026-09-30 | `files.read` 增加 `offset`、`length`，代理新增 `files.stat` 和 `files.range` 能力；服务器新增 `logfollow` 公共帧编码，供远端日志与云盘实时日志复用 | B33 远端日志 |
| 2026-09-30 | 云盘新增内存后台任务，事件 `drive_task.updated` 和 `drive_item.batch`；任务最多同时运行两个，完成 10 分钟后清理 | B31 后台任务 |
| 2026-09-29 | `components/markdown/MarkdownEditor.tsx` 加可选的 `uploadScope`（粘贴、拖入、选择图片）；新增 `components/markdown/upload.ts`（上传、占位、插入文字，笔记的 `logic.ts` 改成转发）；`Markdown.tsx` 显示 `/api/v1/files/<id>` 的图片时取缩略图并链接原图；`NavChildLinks` 加可选的 `limit` 和链接的 `nested`（`.nav-child.nested` 在 `ui.css`）；`lib/i18n.ts` 加编辑框贴图的文案；新增契约 `api/modules/files.yaml`（公共上传，后端模块待做） | B36 贴图、侧边栏显示项目分类 |
| 2026-09-29 | `app/App.tsx`：路径是 `/s/<token>` 时渲染云盘分享页，不经过登录检查；`drive.yaml` 新增公开入口 `/public/shares/*`（`security: []`），云盘模块的 `PublicPaths` 先放在 `drive/pending.go` | B31 外链分享 |
| 2026-09-29 | `drive/viewer/LogView.tsx` 拆出 `RangeLogView`（按段读和实时模式，不绑定云盘），服务器文件标签也用它；`assistant/components/Timeline.tsx` 的动作卡片显示命令和原因、长结果折叠；`hosts.yaml` 新增 `files/range`、`files/follow`（后端待做，代理要加 `files.range` 能力） | B33 Agent 标签、远端日志 |
| 2026-09-28 | `core.yaml` 加代理一键安装的 5 个公开接口（后端占位在 `core/pending.go`）；服务器详情标签支持 `preview`（旧代理也显示新标签）；导航“本机”改名“电脑”（英文键 `Computer`） | B29、B30 |
| 2026-09-28 | 新增公共组件 `components/log/LogViewer.tsx`（虚拟列表、级别、输出、关键字过滤）和 `components/log/levels.ts`；`monitoring.yaml` 容器日志加 `format=json`、镜像删除和清理 | B28 容器日志、B29 系统日志、B31 日志文件共用 |
| 2026-09-28 | `api/events.ts` 加 `useMetricsInterval`，WebSocket 控制消息加 `{"type":"interval","hostId","ms"}`（旧服务端忽略） | B26 刷新周期 |
| 2026-09-28 | 新增契约 `storage.yaml`、`backup.yaml`（后端模块还没建，请求回 404）；设置的“云盘同步”标签换成“存储”，新增“备份”；新增 `features/storage/S3Fields.tsx` 公共 S3 输入框 | B24、B25 前端 |
| 2026-09-28 | `brief.yaml` 的 `/weather` 加 `refresh`；`monitoring.yaml` 加订阅分类接口和 `cycleCount`、`cycleUnit`、`categoryId`（后端占位在 `monitoring/pending.go`）；契约里有、后端没做的接口统一放在各模块的 `pending.go`，回 `httpx.ErrNotLive` | B23 天气刷新、订阅表单 |
| 2026-09-28 | `core.yaml` 加 `GET/PUT /me/preferences`（后端先回 501，占位在 `core/pending.go`）；`httpx` 加 `ErrNotLive`（501）；`api/client.ts` 加公共的 `isNotLive`；偏好加主题色 `accent`，`html` 上写 `data-accent`；`tokens.css` 加五套白天主题色；设置去掉“通用”标签，安装应用挪到个人菜单（`features/pwa/InstallMenu.tsx`），顶栏去掉安装按钮 | B22 主题色、夜间开关、设置整理 |
| 2026-09-28 | 新增 `components/ui/ConfirmDialog.tsx`（`confirmAction`、`useConfirm`、`ConfirmHost`），`Layout` 挂载 `ConfirmHost`；`ui.css` 加 `.xc-btn.danger.solid` | B21 统一二次确认 |
| 2026-09-28 | 新增 `components/layout/PageActions.tsx`（顶栏页面按钮）、`stores/sidebar.ts`（侧边栏折叠，`⌘B`）；`stores/page-title.ts` 加 `subtitle`；`PageHeading` 的 `subtitle`、`meta` 显示到顶栏，`aside` 转成 `PageActions`；`Topbar` 加折叠按钮和页面按钮位置 | B20 页面按钮移到顶栏 |
| 2026-09-28 | 新增前端公共 `api/useInvalidate.ts`，日历、提醒、习惯、监控共用查询刷新 Hook；命令面板在窄屏使用动态视口高度；截图脚本支持 `--theme dark` | C2 去重并修复手机命令面板溢出，补深色主题自查 |
| 2026-09-28 | 新增 `contracts.Renewals`、`RenewalRef` 和 `RenewalsKey`，早报通过统一契约读取运维监控订阅 | B7 接通续费部分 |
| 2026-09-28 | 服务端新增 `pairing-code` 子命令，代理连接增加 `ErrRevoked` 与退出码 3；部署镜像和脚本管理本机代理 | B1 自动配对面板所在服务器，并在吊销后保持停用 |
| 2026-09-28 | `/events` 增加浏览器订阅、暂停和恢复控制；代理协议增加 `metrics.detail`；`GET /hosts` 返回 `HostListItem` 概要 | B18 减少今日页与后台标签的数据传输，并按详情订阅切换采样频率 |
| 2026-09-28 | `web/src/demo/router.ts` 增加 `routeFull`，统一演示开关的路由拦截；`vite.config.ts` 限制测试 worker 并延长慢速环境超时 | C1 去重并稳定全量测试 |
| 2026-09-28 | `actions.Action` 增加可选的 `Destructive` 标记；`app/modules.go` 注册 AI 助手和自动化模块 | B3 确认删除类动作并接入服务 |
| 2026-09-28 | `auth` 增加 `VaultUnlocked`、`WithoutVault`；会话增加 `vault_until` | B13 隐藏内容 |
| 2026-09-28 | `auth` 用 `setup_completed` 区分初始化和启用两步验证；按账号设置改用密码或验证码提升权限，新增安全设置操作 | B12 两步验证可选 |
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
| 2026-09-29 | 删掉没有引用的 `components/ui/LineChart`、`MetricCard`、`Progress`，`hooks/usePresence.ts`，`lib/exportCsv.ts`；`05-frontend.md` 的组件表去掉“小图表”一行 | 清理没用的代码 |
| 2026-10-09 | `router.yaml` 加 `POST /router/report`（公开，令牌校验）、`POST /router/push/token`；`RouterMode` 加 `push`，`RouterConfig` 加 `lastReportAt`、`reportUrl`，`RouterStatus` 加必填的 `source`；`router` 模块实现 `PublicPaths`（B114） | 路由器主动上报 |
| 2026-10-10 | `contracts/hidden.go` 加 `documents`（接口、事件 `document.`、通知链接 `/documents`、来源 `documents`、动作 `documents.`），`vault.yaml` 的 `ModuleId` 加 `documents`，`core/handlers.go` 的 `hidesAny` 加 `documents`；前端 `app/nav.ts`、`app/routes.tsx`、`app/modules.ts` 各加一行；`scripts/shots.mjs`、`scripts/e2e.mjs` 加证件档案 | B115 证件档案 |
| 2026-10-10 | `contracts/hidden.go` 加 `screentime`（接口、事件 `screentime.`、通知链接 `/screentime`、来源 `screentime`、动作 `screentime.`），`vault.yaml` 的 `ModuleId` 加 `screentime`，`core/handlers.go` 的 `hidesAny` 加 `screentime`；`pkg/protocol` 加 `methods_screentime.go`；`cmd/agent/main.go` 加一行注册和一行能力；前端 `app/nav.ts`、`app/routes.tsx`、`app/modules.ts`、`overview/layout.ts`、`overview/TodayPage.tsx` 各加一项；`scripts/shots.mjs`、`scripts/e2e.mjs` 加时间去向 | B116 电脑时间去向 |
| 2026-10-10 | `contracts` 新增 `telegram.go`（`TelegramInbox`，注册表键 `telegram.inbox`）；`reminders/telegram.go` 注册 webhook 时 `allowed_updates` 加 `message`，收到普通消息交给 `TelegramInbox`，已有 webhook 要重新注册；`contracts/hidden.go` 加 `readlater`（接口、事件 `readlater.`、通知链接 `/readlater`、来源 `readlater`、动作 `readlater.`），`vault.yaml` 的 `ModuleId` 加 `readlater`，`core/handlers.go` 的 `hidesAny` 加 `readlater`；前端 `app/nav.ts`、`app/routes.tsx`、`app/modules.ts` 各加一项，`public/manifest.webmanifest` 加 `share_target`；`scripts/shots.mjs`、`scripts/e2e.mjs` 加稍后读 | B117 稍后读 |
| 2026-10-10 | `contracts` 新增 `journal.go`（`ActivitySource`，注册表键前缀 `journal.sources.`）；`projects`、`coding`、`focus`、`habits`、`notes`、`github`、`screentime`、`readlater` 各加一个 `activity.go`，并在自己的 `module.go` 的 `New` 里加一行注册；`contracts/hidden.go` 加 `journal`（接口、事件 `journal.`、通知链接、来源、动作），`vault.yaml` 的 `ModuleId` 加 `journal`，`vault/modules.go` 和 `core/handlers.go` 的 `hidesAny` 同步；前端 `app/nav.ts`、`app/routes.tsx`、`app/modules.ts` 各加一项；`scripts/shots.mjs`、`scripts/e2e.mjs` 加每日时间线 | B118 每日时间线 |
| 2026-10-10 | `habits` 模块：`PersonalDay`、`PersonalDayInput` 加 `restingHr`（旧记录读出来是空字符串，不加迁移），新增 `/habits/body/push`、`/habits/body/push/token`、`/habits/body/report`（公开入口，用 Bearer 令牌校验，同 B114），`habits` 模块实现 `PublicPaths`；`web/scripts/shots.mjs`、`web/scripts/e2e.mjs` 加身体数据 | B119 身体数据 |
| 2026-10-10 | `contracts/hidden.go`（`credentials` 加进后端映射、动作、事件前缀、链接和来源）、`core/handlers.go`（`hidesAny` 加 `credentials`）、`api/modules/vault.yaml`（`ModuleId` 加 `credentials`）、`vault/modules.go` 和 `vault/modules_test.go`（计数 22/22/20）、`app/modules.go`、`backend/sqlc.yaml` 各加一行；`web/src/app/{nav,modules,routes}` 加密钥台账入口；`web/scripts/shots.mjs`、`web/scripts/e2e.mjs` 加密钥台账 | B120 密钥台账 |
| 2026-10-10 | 新增代理方法 `aiconfig.sync` 和能力 `aiconfig`（`pkg/protocol/methods_aiconfig.go`，`docs/04-agent-protocol.md` 加一节）；`cmd/agent/main.go` 加一行注册、一行能力、一行 `SetStateDir`；`contracts/hidden.go` 把 `aiconfig` 归到“Agent”（后端映射、动作、事件前缀各加一处）；`app/modules.go` 加一行；`web/src/app/routes.tsx` 加一行；`web/src/features/coding/NavChildren.tsx` 加“配置下发”一项；`web/scripts/shots.mjs`、`web/scripts/e2e.mjs` 加配置下发，e2e 的代理进程加 `CLAUDE_CONFIG_DIR`、`CODEX_HOME` 指向临时目录 | B121 配置下发 |
| 2026-10-10 | `contracts/hidden.go`（`contacts` 加进后端映射、动作、事件前缀、链接和来源）、`core/handlers.go`（`hidesAny` 加 `contacts`）、`api/modules/vault.yaml`（`ModuleId` 加 `contacts`）、`vault/modules.go` 和 `modules_test.go`（模块数和事件前缀数 +1）、`app/modules.go`、`backend/sqlc.yaml` 各加一处；`web/src/app/nav.ts`、`modules.ts`、`routes.tsx` 各加一行；`web/scripts/shots.mjs`、`web/scripts/e2e.mjs` 加联系人 | B122 联系人 |
| 2026-10-10 | `router.yaml`：`POST /router/report` 回复改成 200 纯文本（`interval=`、`cmd=`）；加 `PUT /router/push/interval`；`RouterConfig` 加 `pushInterval`；重启接口、重启路由器在 push 模式下回 202；`InsertTraffic` 冲突时累加，流量表按分钟一行（B114） | 上报间隔可调、push 模式下重启 |
| 2026-10-10 | `credentials.yaml`：`Credential` 加必填的 `hasSecret`，`CredentialInput`、`CredentialPatch`、`CredentialRotate` 加 `secret`（加密存储，列表和详情不返回），新增 `GET /credentials/{id}/secret`（要提升权限，记审计）；迁移 `20261010000600_m22_credentials_secret.sql` 给 `credentials` 加 `secret_enc` 列。`documents.yaml`：`DocumentKind` 从枚举改成字符串（内置类型之外，用户自己加的类型写成 `c:名称`），新增 `GET/POST/DELETE /documents/kinds`（自定义类型存在设置键 `documents.custom_kinds`，没有迁移）。前端 `app/nav.ts` 把稍后阅读挪到笔记下面、密钥挪到监控下面，`app/modules.ts` 的命令分组 `密钥台账` 改 `密钥`，稍后读统一改名稍后阅读 | 用户 2026-10-10 的几条修改 |
| 2026-10-11 | `contacts.yaml`：`Contact` 加必填的 `phones`、`emails`、`source`，`ContactInput`、`ContactPatch` 加 `phones`、`emails`；新增 `POST /contacts/import`、`GET/PUT/DELETE /contacts/sync`、`POST /contacts/sync/run`；迁移 `20261010000700_m23_contacts_import.sql` 给 `contacts` 加 `phones`、`emails`、`source`、`external_id` 四列；`backend/go.mod` 加 `github.com/emersion/go-vcard`；联系人模块多了 `contacts.sync` 定时任务。代理加命令 `install`，`pair` 加 `--no-start`，`install.ps1` 改用 `install`（不是接口，是代理命令行） | B140 联系人导入和同步、B142 代理后台常驻 |
| 2026-10-11 | `readlater.yaml`：`ReadItem` 加必填的 `hasHtml`、`kind`（`page`、`tweet`）、`meta`，详情加 `contentHtml`；新增 `GET /readlater/{id}/assets/{hash}`（存档图片）、`GET /readlater/{id}/export`（单文件 HTML）、`GET/PUT/DELETE /readlater/x-auth`（X 登录 Cookie，保存和清除要提权，不返回内容）、`POST /readlater/x-auth/test`；迁移 `20261010000800_m24_b143_readlater_archive.sql`（`read_items` 加三列，新表 `read_assets`）；设置项 `readlater.x.cookie`（加密）、`readlater.x.config`；推文网址统一存成 `https://x.com/用户名/status/编号`；`web` 新增依赖 `dompurify`；`settings/tabs.tsx` 加一行。 |
| 2026-10-11 | `music.yaml`：`MusicMatchRequest` 加 `choose`（为 true 时只找候选放进待确认，不改歌）。前端：全局音乐播放器从底部挪到页头（`app/TopbarActions.tsx` 加一行 `TopbarPlayer`） | B158、B159 |
| 2026-10-11 | `readlater.yaml`：`ReadItemInput` 和 `ReadItem` 加 `downloadMedia`，`POST /readlater/{itemId}/refetch` 加 `media` 查询参数；迁移 `20261012140000_m43_b160_readlater_media.sql` 给 `read_items` 加 `download_media`；`GET /readlater/{itemId}/assets/{hash}` 改成支持 Range（视频）；存档里允许 `<video>`（只有勾选下载时才保留） | B160 |
| 2026-10-11 | 新增 `api/modules/me.yaml` 和 `me` 模块：`GET /me`、`PUT/DELETE /me/manual`、`GET /me/history`、`GET/PUT /me/settings`、`GET/POST/DELETE /me/report`、`POST /me/report/token`（`POST /me/report` 是公开入口，用 Bearer 令牌校验；同一前缀下的其他路由由模块自己要求登录和 CSRF 头）；迁移 `20261012150000_m44_b162_me.sql`（`me_states`、`me_reports`）；`contracts` 新增 `me.go`（`Me`，键 `me.state`，事件 `me.state`、`me.woke`）和 `schedule.go`（`Schedule`，键 `habits.schedule`，习惯模块提供）；`habits` 模块订阅 `me.woke` 填睡眠时长，`module.go` 加两行；`app/modules.go`、`backend/sqlc.yaml`、`settings/tabs.tsx` 各加一行；`overview/layout.ts`、`TodayPage.tsx` 加“我的状态”卡片；动作 `me.state`；`web/scripts/e2e.mjs` 加我的状态 | B162 |
| 2026-10-13 | `core.yaml` 的 `Preferences.accent` 枚举加 `cyber`，`core/preferences.go` 的 `accents` 同步；`styles/tokens.css` 加辅色 `--xc-accent-2*`、站标 `--xc-logo-*` 两组变量和赛博主题两段（连底色一起换）；`components/ui/BrandMark.tsx` 换成像素站标；`styles/styles.css` 加全局 `accent-color`、链接颜色、选中文字底色；`styles.css`、`theme.css`、`refinements.css`、`interaction-polish.css` 里提示条、头像菜单、命令面板、图标按钮写死的颜色改成变量；`public/` 站标和图标换掉，版本号 `connect-1` 改 `led-1` | B175 |
| 2026-10-13 | `github.yaml` 的 `GitHubActionsUsage` 加必填 `publicMinutes`，`usedMinutes`、`byOS` 只算私有仓库（B177）。`core.yaml` 的 `Preferences` 加可选 `navPins`（B179）；`app/nav.ts` 加 `navMerges`、`navEntryLabels`、`defaultNavPins`、`entryOf`、`launcherGroupLabels`；`components/layout/Sidebar.tsx` 图标栏改成固定入口加启动台，`useCurrentNavPanel` 多返回 `panelLabel`，合并组的二级菜单由 `MergedNavPanel` 包一层；新增 `components/layout/Launcher.tsx`、`navEntries.ts`；`stores/preferences-store.ts` 加 `navPins`；`hooks/usePreferencesSync.ts` 同步 `navPins` | B177、B179 |
| 2026-10-13 | `ai.yaml`：新增 `GET /ai/sessions`（`AiSession`：面板对话、服务器 Agent、Agent 执行记录合在一起）和事件 `ai.message_started`；`createHostAgentConversation` 请求体加可选 `mode`。迁移 `20261013010000_m45_b184_host_permissions.sql` 给 `ai_conversations` 加 `host_permission`、`host_permission_until`（服务器 Agent 权限存库）。`contracts` 新增 `ai_sessions.go`（`AISessionRuns`，aiagents 提供）。AI 请求带 `prompt_cache_key`，供应商不支持时去掉重试。前端：AI 入口从右下角挪到顶栏（`app/TopbarActions.tsx` 加一行 `AssistantButton`）；`app/Layout.tsx` 顶栏挪到滚动区外（B190）；`navPins` 里 `/-/<编号>` 表示左栏分隔线（B189）；`deploy/docker-compose.yml` 给 `XC_TRUSTED_PROXIES` 加默认值（B186） | B182 到 B193 |
| 2026-10-13 | `contracts/knowledge.go` 的 `Embedder.Available`、`Embedder.Embed` 加 `providerID` 参数（0 是快速 AI 的供应商）；`knowledge.yaml` 的 `KnowledgeSettings` 加可选 `providerId`；`ai` 模块一次嵌入请求最多 100 段 | B197 资料问答的嵌入可以选供应商 |
| 2026-10-13 | `knowledge.yaml`：`KnowledgeSettings` 加 `driveFolders`（`KnowledgeDriveFolder`，保存只看 id，读取时带路径），`KnowledgeStatus` 加 `unreadable`，来源多了 `drive`。`backend/go.mod` 加 `github.com/ledongthuc/pdf`（读 PDF 文字）。前端 `music/FolderPicker.tsx` 加可选的 `title`、`description`，知识库设置借用它。`web/scripts/e2e.mjs` 知识库一段加选云盘文件夹 | B199 云盘文件进知识库 |
| 2026-10-14 | `habits.yaml`：新增 `HabitTargetMode`（`fixed`、`prompts`），`Habit`（必填）、`HabitInput`、`HabitPatch` 加 `targetMode`；`HabitToday` 加必填 `target`，`HabitDay` 加必填 `target`。迁移 `20261014010000_m46_b202_habit_prompts.sql`（`habits.target_mode`、新表 `habit_prompts`）。`contracts.HabitProgress.Target` 改成今天的目标。前端 `overview/components/SideCards.tsx` 改用 `target` | B202 |
| 2026-10-14 | `habits.yaml`：新增 `HabitAutoCheck`，`Habit`、`HabitInput`、`HabitPatch` 加 `autoCheck`。生成的 `RemindMode` 常量改名为 `RemindModeInterval` 这类带前缀的（新枚举里也有 `none`）。迁移 `20261014020000_m47_b201_habit_auto_check.sql`（`habits.auto_check`、`auto_before`）。打卡来源多了 `me` | B201 |
| 2026-10-14 | `projects.yaml`：`CreateProject` 加可选 `template`（`indie-game`） | B204 |
| 2026-10-14 | `contracts` 新增 `websearch.go`：`WebSearch`（键 `ai.web_search`，ai 模块提供）和 `PageReader`（键 `readlater.page_reader`，稍后读提供）。`modules/ai/llm` 的 `Request` 加 `WebSearch`（Responses 风格往 `tools` 里加 `{"type":"web_search"}`，chat 风格回 `ErrWebSearchStyle`），`Result` 加 `Sources`（从 `url_citation` 标注收集）；带联网搜索又带 Schema 时，供应商回 400 就去掉 Schema 重试一次。`ai.yaml` 加 `GET/PUT /ai/web-search`（PUT 要提升权限）、`POST /ai/web-search/test`，新增 `AiWebSearchMode` 枚举，生成的 `ReasoningEffort` 常量因此改名为 `ReasoningEffortOff` 这类带前缀的。`recipes.yaml` 加 `POST /recipes/discover`、`GET /recipe-candidates`、`PATCH /recipe-candidates/{id}`；迁移 `20261014060000_m51_b211_recipe_candidates.sql`（`recipe_candidates`、`recipe_discover_runs`）；设置项 `ai.web_search_mode`、`ai.brave_key`（加密） | B211 |
| 2026-10-16 | `watchers.yaml` 的 `Watcher`、`WatcherInput`、`WatcherPatch` 加 `activeFrom`、`activeTo`、`activeDays`（检查时段，迁移 `m59_b236_watcher_window` 给 `watchers` 加三列）；`web/src/features/watchers` 的新建修改弹窗和详情页加检查时段 | B236 网站自动化的检查时段 |
| 2026-10-17 | `monitoring.yaml` 的 `Subscription` 加必有的 `nextRenewalTime`，`SubscriptionInput`、`SubscriptionPatch` 加可选的 `nextRenewalTime`（HH:MM，空字符串表示只按天）；迁移 `m60_b240_renewal_time` 给 `subscriptions` 加 `renewal_time` 列。订阅扫描间隔改成每分钟 | B240 |
| 2026-10-19 | `core.yaml` 的 `Preferences` 加可选 `terminalLeave`（B249），`core/preferences.go` 同步校验；前端 `stores/preferences-store.ts` 加 `terminalLeave`，`hooks/usePreferencesSync.ts` 同步它。新增 `hooks/useDraft.ts`（`useDraft`、`useKeptState`）和 `hooks/sessionMap.ts`（`useKeepScroll` 的 sessionStorage 存取挪到这里共用）。`app/TopbarActions.tsx` 加一行 `BackgroundButton`，`app/GlobalPanels.tsx` 加一行 `TerminalDock`，`settings/tabs.tsx` 加“终端”一页 | B249 终端切走时的处理方式、写了一半的输入不丢 |
