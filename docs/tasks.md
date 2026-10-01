# 任务看板

这是项目进度的唯一来源。做什么、做到哪、下一步是什么，都只看这里。

- 开发者从“待做”里按顺序取第一个任务，开分支前把状态改成“进行中”，写上自己的名字。
- 任务在 PR 合并进 `develop` 后移到“已完成”，写上 PR 链接。
- 大任务的做法和验收标准在 `docs/specs/` 里。小任务直接写在这里。
- 用户提的新需求，由审查者加到“待做”末尾（编号顺延），需要时写规格文件。

## 当前状态（2026-09-28 更新）

- 前端已经做完四轮，线上是 `develop` 最新的部署（带部署标记的提交会部署，见 AGENTS.md）。
- 演示数据已备份到 `demo-data-backup-20260929` 分支（提交 `f74744d`）。
- D1 已合并。正式前端直接使用真实接口。
- B22 到 B37 的后端已合并到 `develop`。B35 的关闭 PR 补齐和 C5 清理在 `codex`。
- 后端完成记录见 [backend-todo.md](backend-todo.md)。
- `frontend-done` 分支是交给 Codex 之前的版本，需要时可以回到这里。

## 进行中

| 编号 | 任务 | 规格 | 负责 |
| --- | --- | --- | --- |


## 待做

后端按这个顺序做。全部在 `codex` 分支上做，一个任务一个提交，提交信息以编号开头；一批做完由 Claude 整批验收，用户合并一次（2026-09-27 用户要求）。用户说“构建”或“部署”时才带部署标记。

| 编号 | 任务 | 规格 | 负责 |
| --- | --- | --- | --- |
| B70 | “GitHub”改名“仓库”，挪到 Agent 下面；多仓库、提交、CI 步骤、Forgejo 也能关注；设置页整理 | [B70](specs/B70.md) | 前端 Claude，后端 GPT |
| B71 | 仓库事件通知：CI 开始、成功、失败，新提交，PR，Issue，发布 | [B71](specs/B71.md) | 前端 Claude，后端 GPT |
| B72 | 笔记：编辑时能滚动、双击进入编辑、外链分享（可加密码）、左栏“+”快速新建、浮窗 | [B72](specs/B72.md) | 前端 Claude，后端 GPT |
| B73 | 便签：快速记录存成便签，瀑布流，和笔记共用标签 | [B73](specs/B73.md) | 前端 Claude，后端 GPT |
| B74 | 编辑器：多媒体、折叠代码块和复制、隐藏块、背景色、提示块、表格等 | [B74](specs/B74.md) | 前端 Claude，后端 GPT（只有一个字段） |
| B75 | 云盘分享页：浏览器能看的都直接预览，下载按钮，密码，下载次数 | [B75](specs/B75.md) | 前端 Claude，后端 GPT |
| B76 | 左栏一级菜单显示未读和待处理数量，二级菜单选中更明显 | [B76](specs/B76.md) | 前端 Claude |
| B77 | 看板卡片不显示编号，紧急度加标题 | 见下 | 前端 Claude |
| B78 | CI 提速：前端检查从六七分钟降下来 | [B78](specs/B78.md) | Claude |
| B79 | 设置弹窗显示版本号；设置 → 维护：资源占用、清理没用的文件 | [B79](specs/B79.md) | 前端 Claude，后端 GPT |
| B80 | 切换页面后回来，保留列表和内容的位置 | [B80](specs/B80.md) | 前端 Claude |
| B81 | 增量备份：只传变化的部分，删掉的也同步删，能回到任意一次 | [B81](specs/B81.md) | 前端 Claude，后端 GPT |
| B82 | 服务器：备注、账号密码、IP 地址、国家，左栏顺序和拖动排序 | [B82](specs/B82.md) | 前端 Claude，后端 GPT |
| B83 | 健康提醒（喝水、护眼、起身）按电脑是否在用来提醒；工作日和作息 | [B83](specs/B83.md) | 前端 Claude，后端 GPT |

### 任务说明

**2026-10-01 第三批（B70 到 B83）的说明**
- 用户 2026-10-01 定的分工：Claude 只做前端，写好规格、接口定义（`api/modules/*.yaml`）和后端占位（`pending.go` 回 501）。后端交给 GPT，照各规格里“后端（待做）”一节做。
- 前端遇到 404 或 501 时退回旧的样子，或者显示“还没上线”，不能报错。
- Claude 替用户定的两件事（用户问“帮我想想”）：GitHub 页改名“仓库”，挪到“Agent”下面，地址还是 `/github`；便签和笔记的“新建”按钮怎么放，见 B73。用户不同意再改。
- B77 看板卡片：不显示卡片编号（编号还在卡片详情和鼠标悬停提示里），紧急度图标和标题放一行，标题用正文颜色。只改前端。

**2026-10-01 第二批（B60 到 B66）的说明**
- 用户额度不够，Claude 只写了规格，前后端都交给 GPT 分别做。每个规格里都有“前端（待做）”和“后端（待做）”，接口以“契约”一节为准，先改契约再分头写。
- B64、B65、B66 是 Claude 的建议方案，用户还没确认，开工前先问用户。
- 几个任务会改同一批共享文件（`app/modules.go`、`features/settings/tabs.tsx`、`app/nav.ts`、`app/modules.ts`），并行做时按 AGENTS.md 的规则只加行，合并时注意冲突。

**2026-10-01 这一批（B49 到 B59）的说明**
- 前端已经全部做完并提交到 `develop`（Claude）。后端按各规格的“后端（待做）”一节做，B56 已经前后端都做完，B52、B49、B55、B50、B51、B57 的后端已做。
- 用户 2026-10-01 定的分工：Claude 在 `develop` 上写前端、接口定义和规格，新接口在各模块的 `pending.go` 先回 501。后端由别的开发者照规格里的“后端（待做）”做。
- 表里的顺序就是前端的开发顺序。后端可以按编号做，互相不依赖。B59 的邮件卡片依赖 B53。
- 用户已确认：新订阅默认提前 7、3、1 天提醒；过期和 3 天内标红，7 天内标黄；服务器在国外，能直接连 Gmail；邮件第一版只收不发；推送保留 3 天；拖动不加新的库；和风天气的 key 由用户在设置里填。
- 隐藏模块按 B57 规格里写的方式做：锁定时完全看不出有隐藏内容，接口和 MCP 都按不存在处理。

**2026-09-30 这一批（B41 到 B47）的说明**
- 表里的顺序就是建议的开发顺序，不按编号。B41 先做，后面的任务出错时能直接看到报错。B45 的第一步（开关）做完就带部署标记推送，让用户在真机上测，不要等整批做完。B48 在 B47 之前做，B43 和 B47 里“始终要验证”的操作要用它的 `RequireStrictElevated`。
- B46 做完再做 B43 和 B47：B43 要暴露看板的动作，B47 要把 Agent 挂到卡片成员上。
- B47 定了运行位置：在代理所在的机器上跑，不在服务端进程里跑，理由见规格。
- B36 的“二级分类”停做。分类的表和列保留不删，界面和接口不再用，下个版本删。
- 开工前和用户确认：B46 界面上把 “Issue” 改叫“卡片”；B47 左栏“Agent 任务”改成“Agent”。

**2026-09-28 这一批（B20 到 B37）的说明**
- B20 改所有页面的顶部结构，排在最前面。后面的界面任务都按新结构写：页面按钮用 `PageActions` 放顶栏。
- B21 做完以后，新写的删除、停止、重启、结束进程、吊销这类操作都要用 `useConfirm`。
- B24 定了文件目录 `data/files/<模块>/`。以后新模块存文件都走 `files.Store`，不要自己拼路径。
- B32 替换 B3 里“用 Anthropic SDK”的做法。B33 依赖 B32。
- 有几处是审查者按自己的理解写的，开工前和用户确认：B22 的“设置弹窗”（按个人菜单写的）。B36 的“二级分类”（Issue 分两级）和 B31 的“外链分析”（外链分享）用户 2026-09-29 已确认。

**B21 高危操作统一二次确认**
- 新增 `components/ui/ConfirmDialog.tsx` 和 `useConfirm()`：返回 Promise，确认为 true。参数：标题、说明、确认按钮文字、是否危险（危险时确认按钮用 `danger`）、可选的“要输入的文字”（比如 B25 的“恢复”）。样式用现有 `Dialog`，取消在左、确认在右，默认焦点在取消上。
- 替换现在全部 27 处原生 `confirm()`（`grep -rn "confirm(" web/src`），并补上还没有确认的操作。范围：所有删除（包括移到回收站、删除标签和分类）、停止、重启、结束进程、关机、锁屏、注销、吊销代理、卸载、清空、解绑、恢复备份、关闭两步验证。
- 确认框里写清楚对象和后果，比如“删除容器 nginx？容器里没有挂载出去的数据会丢失。”不写“确定吗？”。
- 批量操作写数量：“删除选中的 12 条笔记？”
- 已经有提升权限（`withElevation`）的操作，先确认再要求提升权限。
- `docs/07-design.md` 第五节“危险操作”一条改成用 `useConfirm`。
- 验收：`grep -rn "confirm(" web/src` 只剩 `useConfirm` 相关的代码；逐页点一遍删除和停止类按钮，都先弹确认框；按 `Esc` 等于取消。

**B22 主题色、夜间开关、设置整理**
- 主题色：在现在的配色之外再加 5 套，名字和颜色由开发者定，要求好看、现代、对比度够（正文和背景对比度不低于 4.5:1）。参考方向：紫色、薄荷绿、日落橙、玫瑰粉、石墨黑白。每套只改 `tokens.css` 里的强调色和少量底色，用 `html[data-accent="..."]` 切换，不改组件。
- 夜间模式改成单独的开关：开、关、自动（跟随系统）。夜间只有一套配色，不跟主题色变化，强调色用夜间配色自己的。原来的“主题：跟随系统 / 深色 / 浅色”去掉，旧的设置自动转换（深色 → 开，浅色 → 关，跟随系统 → 自动）。
- 主题色和夜间开关放在个人菜单（头像弹出的菜单）里：夜间用三段 `Segmented`，主题色用一排色块，点了立即生效。存在服务端的用户设置里（跨设备），本地也存一份用来首屏不闪。
- 设置里去掉“通用”标签。语言也挪到个人菜单。“安装应用”从顶栏挪到个人菜单，已经安装（`display-mode: standalone`，或者收到过 `appinstalled`）时不显示。
- “AI 助手”改名“AI”：设置标签、浮窗标题、命令面板、快捷键说明。英文键换一个新的，避免和别处冲突。
- 截图脚本 `npm run shots` 加参数，能对每套主题色各出一张今日页截图。
- 验收：每套主题色在白天和夜间各截一张今日页和服务器页，文字清楚；夜间选“自动”时跟着系统切换；换一台浏览器登录，主题色还在；设置里没有“通用”标签。

**B23 零碎修改**
- 今日页天气：
  - 天气内容最后加一个刷新按钮：`RefreshCw` 图标 13px，颜色 `--xc-faint`，没有边框和底色，鼠标移到天气模块上时才变成 `--xc-muted`。
  - 点击后图标旋转（`motion.css` 里加一个旋转动画），请求带 `refresh=1`，后端跳过 10 分钟缓存重新拉取（`brief.yaml` 的 `/weather` 加这个参数，1 分钟内最多真刷新一次）。拿到结果后停止旋转，失败时 `toast` 提示。
  - 鼠标移到整个天气模块上时显示提示“更新于 10:32（3 分钟前）”，数据来自 `fetchedAt`。
- 笔记：
  - 预览模式下不显示格式工具栏和插入图片、附件按钮，只显示渲染后的内容。标题照常可以改。
  - 列表支持单个删除：每行的“更多”菜单加“删除”，手机上左滑也能删。
  - 列表支持批量删除：列表上方加“选择”按钮，进入选择模式后每行出现勾选框，有“全选”，顶栏显示“删除选中的 N 条”。删除要二次确认（B21）。
  - 删除走已有的删除接口。批量删除接口没有的话加 `POST /notes/batch-delete`。
- 监控订阅的新建和编辑：
  - 分类可以自己加：分类下拉最后一项“管理分类…”，弹窗里新建、改名、删除（删除时这个分类下的订阅改成“其他”）。默认保留现在的四个。后端加 `subscription_categories` 表，订阅加 `category_id`，旧的枚举值迁移过去，旧列先保留（迁移只加不删）。
  - 币种改成和其他字段一样的下拉框（`xc-select`），选项 CNY、USD、EUR、HKD、JPY、GBP、SGD，最后一项“其他…”，选了以后出现输入框手动填 3 位代码。现在是输入框加候选列表，已填了 CNY 所以候选只剩 CNY，这就是“只有 CNY”的原因。
  - 周期改成一行：固定文字“每”，数字输入框（默认 1），单位下拉（秒、分钟、小时、天、周、月、年）。后端加 `cycle_count`、`cycle_unit` 两列，旧的 `monthly`、`yearly`、`custom_days` 迁移成对应的值。月和年按日历算（1 月 31 日加 1 个月是 2 月最后一天）。
  - “提前几天提醒”改成两个勾选项：提前 1 天、提前 7 天，可以都选，默认都选。存法沿用现在的列表字段。
- 验收：天气刷新按钮平时不显眼，点了会转，悬停有更新时间；笔记预览时看不到格式按钮；批量删除 3 条笔记后列表正确；订阅用“每 3 个月”保存后，下次续费日期按 3 个月算；自定义分类能用。

**C3、C4、C5 清理轮**
- 和 C1、C2 一样的内容（见下面“C1、C2 清理轮”）。
- C3 另外检查：B20 之后所有页面顶栏的按钮位置一致；B21 之后没有遗漏的确认框。
- C4 另外检查：服务器相关的新标签（日志、Agent）和新卡片在电脑页也正常。
- C5 另外检查：旧 Anthropic 设置界面和接口已删除。旧配置提示与对话格式兼容代码保留；所有持久文件都走 `files.Store`。

**B34 浏览器推送自检和后台通知**
- 用户反馈：开了浏览器通知，点测试只看到站内提示，系统通知没弹。
- 已查过的情况：测试接口返回成功，说明服务端已经把消息交给了浏览器厂商的推送服务。之后没弹，常见原因有两个：系统设置里关了这个浏览器的通知；Chrome 的推送走谷歌的服务器（`fcm.googleapis.com`），国内网络下浏览器收不到。Edge（微软）、Firefox（Mozilla）、Safari（苹果）走各自的服务器。
- 要做的：
  - 设置 → 通知的“浏览器推送”卡片里列出已订阅的浏览器：设备名（从 User-Agent 解析）、推送服务（按 `endpoint` 的域名判断：谷歌、微软、Mozilla、苹果）、上次成功时间、上次错误。谷歌推送服务旁边写一句“国内网络可能收不到，可以换 Edge 或 Safari”。可以单个删除。
  - 数据库给订阅加 `last_ok_at`、`last_error` 两列，每次推送后更新。
  - “发送测试”分两步显示结果：① 本机检查：页面直接调用 `registration.showNotification` 弹一条本地通知，弹不出来就提示“系统没有允许这个浏览器显示通知”，并按系统给出打开方法（Windows、macOS、Android、iOS 各一句）；② 服务端推送：显示推送服务的返回状态。
  - `sw.js` 的通知图标从 `favicon.svg` 改成 PNG（用 B5 已有的 192px 图标）。部分平台不显示 SVG 图标。
  - 面板关掉后也能收到：Web Push 本来就支持，只要浏览器还在后台运行。在卡片里写清楚：电脑上浏览器要在后台运行；iPhone 要先把面板“添加到主屏幕”，从主屏幕打开后再开启推送（iOS 16.4 以上）；安卓 Chrome 需要谷歌服务。想要更稳的后台通知，推荐同时开 Bark 或 Telegram。
- 验收：在 Edge 上开启推送，关掉面板标签页，建一个 1 分钟后的提醒，到点弹出系统通知；订阅列表显示推送服务和上次成功时间；在系统里关掉浏览器通知后点测试，页面给出明确提示。

**B35 GitHub 仓库下拉多选，同步加快**
- 设置 → GitHub：个人访问令牌的输入框已经有。填好令牌后，“关注的仓库”从手动输入改成下拉多选：列出令牌能访问的全部仓库（`GET /user/repos`，分页取全，按最近更新排序），可以搜索，显示私有标记。已选的显示成标签，可以单个去掉。
- 后端加 `GET /github/available-repos`，结果缓存 10 分钟，有“刷新”按钮。
- 同步间隔从 5 分钟改成 1 分钟。每个请求带 `If-None-Match`（ETag），GitHub 回 304 时不算进限额。界面显示剩余限额。剩余不足 10% 时自动退回 5 分钟。
- 页面上加“立即同步”按钮，同步中显示转圈。
- 每个仓库的 PR 只拉第一页（100 个）的限制一起去掉，改成分页拉取，只拉打开的和最近 7 天关闭的。“已知问题”里对应那条删掉。
- 验收：用假的 GitHub 服务器测 ETag 和 304；下拉能搜到仓库并保存；1 分钟内能看到新开的 PR。

**B37 提醒页汇总其他模块的提醒**
- 用户问：监控里的各种提醒也会显示在提醒页吧？现在不会。订阅续费、证书和域名到期这些只发通知，提醒页看不到。
- 在 `contracts` 里加 `ReminderSource` 接口：`Upcoming(ctx, from, until) ([]ExternalReminder, error)`，`ExternalReminder` 有来源模块、标题、时间、链接、是否已完成。注册表键 `reminders.sources`，可以有多个。记到“接口变更记录”。
- 实现方：监控（订阅续费、证书到期、域名到期）、项目（B36 的 Issue 截止时间）。以后别的模块照着加。
- 提醒页“即将到来”和“今天”列表里混排显示这些条目，前面有来源标签（比如“订阅”“证书”“Issue”），只读，点击跳到来源页面。工具栏加一个开关“显示其他模块”，默认打开。
- 概要卡片的数字包含这些条目。
- 验收：建一个 3 天后续费的订阅，提醒页“即将到来”里能看到，点击跳到监控的订阅；关掉开关后不显示。

**D1 演示数据移除**
- 用户已明确要求删除，删除前已把完整版本备份到远端 `demo-data-backup-20260929` 分支。
- 删除 `web/src/demo` 和入口导入、演示事件发送、云盘演示文件地址、截图脚本的演示模式设置。
- 验收：旧浏览器即使存有 `xc.demo.full=on`，页面也直接请求真实接口。页面底部不再出现演示开关。云盘文件和缩略图使用正式接口。端到端主流程、全量检查和桌面手机截图通过。

**B15 B10 遗留**
- `components/ui/Stat.tsx` 的 `Ring`：比例为 0 时不渲染 `.bar`。原因是 `stroke-linecap: round`，长度为 0 也会画一个点。
- 用户决定提醒页保留“今天、下一个、即将到来、已完成”四张卡。项目页在现有四张卡之外增加“进行中”，单独显示进行中的 Issue 数。
- 验收：项目页有五张概要卡，“进行中”数字与进行中 Issue 数一致。提醒页仍有原来的四张卡。桌面和手机均无横向溢出。

**B2 今日（首页）**
- 依赖 B10 的 `StatStrip`、`Section`。
- 第一轮前端：`web/src/features/overview` 做今日页，侧边栏改名“今日”。数据用各模块已有的接口，不用改后端。每个分区包错误边界。写好布局接口契约 `api/modules/dashboard.yaml`；接口没上线时布局先存在本地，并提示“还没上线”。
- 第二轮后端：新建 `modules/dashboard`，实现 `GET/PUT /dashboard/layout`。
- 做法、要调的接口和验收标准见规格。

**B3 AI 助手与自动化**
- 用官方 Go SDK `github.com/anthropics/anthropic-sdk-go`。默认模型 `claude-opus-5-5`，adaptive thinking，流式输出，开启服务端 refusal fallback（`fallbacks: "default"` 加 beta 头 `server-side-fallback-2026-07-01`）。写代码前查官方 SDK 文档确认用法，不要凭记忆。
- 工具来自 `d.Actions.List()`。`read` 直接执行；普通 `write` 直接执行；删除和标记为破坏性的动作要确认；`dangerous` 要确认并要求提升权限。“所有写操作都先确认”开关可以覆盖普通写动作。
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
- B1 本机代理的自动安装、吊销后保持停用和卸载尚未在真实服务器验证。
- 真实的 Home Assistant、Telegram、Bark、Server酱、Web Push、GitHub、Linear。测试全部用假服务器。
- systemd 服务管理（开发环境没有 systemd）和真实的 SSH 主机。
- B29 系统日志：`journalctl` 只用假输出测过，没在真实 systemd 机器上跑；Windows 事件日志（`wevtutil`）只测了 XML 解析和查询拼装，没在 Windows 上跑。没有 journal 的 Linux 读 syslog 文件时没有级别，按级别过滤会得到空列表。

- B30 的 Windows 安装（`install.ps1`、`setup.exe` 的安装模式、任务计划程序）没在真实 Windows 上跑过，Linux 的 `install.sh` 在没有 systemd 的容器里跑过下载、校验、配对，没跑过 systemd 那一段；Docker 镜像四个平台的编译没在本机构建过。
- 测试 `backup` 的 `TestAutomaticBackupToS3KeepsTheNewest` 在全量 `go test -race ./...` 下偶尔失败（2026-09-30 遇到一次：第 3 天的备份不在列表里，第 2 天的没被删）。单独跑这个包 6 次都通过。失败时日志里有 “database is closed”，像是后台备份任务在测试结束后还在跑。还没查原因。
- B30 没做：托盘图标、代理自动更新。现在升级要再执行一次安装命令。`setup.exe` 没有代码签名，Windows SmartScreen 会提示。

功能限制：
- 同一个 TOTP 码在 30 秒窗口内可以重复使用。只支持一个用户。
- HA 的 `WatchEntity` 注册只存在内存里，使用方要在 `Start` 里调用。
- Linear 不导入已完成或已取消的 Issue，本地新建的 Issue 不会自动建到 Linear。
- 看板拖动只支持桌面（B46 后手机上用卡片详情里的“看板和列表”下拉移动）。习惯的提醒时段不能跨午夜。番茄钟不能暂停。
- 早报的习惯部分只显示今天，`contracts.Habits` 没有“昨天”的数据。
- SSH 主机的最后在线时间只存在内存里。Windows 上 `svc.logs` 返回“不支持”。
- 脚本运行记录不会自动清理。Windows 主机上跑 bash 脚本会直接失败。订阅支出汇总没有汇率换算。

可以改进：
- B43 只支持带令牌的本地客户端（Claude Code、Codex、Cursor）。网页版 claude.ai、ChatGPT 要 OAuth 登录，没做。MCP 只开放工具，没有资源和提示词。
- B47 没做的部分：“只编译”任务（不经过 Agent 直接构建某个分支，`coding_tasks.executor` 有 CHECK 约束，要重建表）；PR 合并后编码任务的状态不变（状态也有 CHECK 约束，只把卡片移到“已完成”并写评论）；构建产物不进云盘目录，只能在任务详情里下载；Windows 上的 clone 和构建没在 CI 里跑过，要用户在自己电脑上验证；任务开始前的 fetch 是同步的，最多 3 分钟，这期间不派发别的任务。
- B46 没做完的部分：手机上长按拖动卡片（手机上用卡片详情里的“看板和列表”下拉移动）、卡片封面图和附件区、看板页签拖动排序（接口已支持 `afterId`）、卡片详情做成弹窗（现在还是单独一页）、跨项目移动卡片的界面入口（接口已支持）。
- B44 手机上的点击区域（按钮、图标按钮、列表行）还没统一到 44 × 44px，只放大了字号。
- 文件上传进度条。SSH 主机指纹变化后在界面上重新信任。
- `features/reminders` 和 `features/habits` 的 `api.ts` 修改后自己刷新数据，同时又用了 `invalidateOn`，有重复。
- Agent 任务（原“编码任务”）的运行设置在仓库页，没有单独的设置标签。
- 搜城市先查 Open-Meteo，查不到再查 OpenStreetMap（Nominatim），开发环境连不上外网，没在真实网络下验证过。

## 已完成


2026-10-01 核对提交记录：原来留在“进行中”和“待做”里的 B20 到 B37、B49 到 B57、B59 其实都已经做完合并，表里没及时挪过来，现在从那两张表里删掉了。

| 批次 | 内容 |
| --- | --- |
| B69 | 网盘账号挪到 设置 → 存储：WebDAV 和 Google Drive 都能加多个，能测试、授权、撤销；备份设置从账号里选一个，只填目录；云盘页按账号出标签。启动时把 B63 的设置自动变成账号，Google 旧回调地址继续能用。和规格不同的地方写在规格开头。规格见 [B69](specs/B69.md) |
| B53 | 邮件后端：IMAP 收 Gmail、阿里企业邮箱和其他邮箱，保存前先试登录；每个账号一条常连的连接，用 IDLE 几秒内收到新邮件并推送，不支持 IDLE 的每分钟查一次；断线按 10 秒到 5 分钟重连；已读和星标双向同步；正文第一次打开时取，附件存文件存储；GBK 编码能正确显示。和规格不同的地方写在规格的“后端”一节。规格见 [B53](specs/B53.md) |
| B58 | 和风天气和地震：设置里填 Host 和 key，保存前试调一次；天气详情有预警、两小时降水、空气、日出日落月相、生活指数、和昨天比、附近地震（中国地震台网，取不到用美国地质调查局）；快下雨、天气预警、附近地震三种推送。和规格不同的地方写在规格的“后端”一节。规格见 [B58](specs/B58.md) |
| B65 | OpenWrt 主路由：新模块 router，通过路由器的 ubus 读系统信息、接口、在线设备、WAN 口速率，每分钟存流量，WAN 掉线或连不上超过 2 分钟推送；可以直连或通过家里的代理转发；能重启接口和路由器；左栏“集成”加路由器，设置加路由器标签，今日页加网络卡片。规格见 [B65](specs/B65.md) |
| B64 | 服务器支持 Unraid：安装脚本认出 Unraid 后把代理装在 U 盘上，开机由 /boot/config/go 启动，退出后自动重启；卸载脚本同样认 Unraid；添加服务器时多给一条不带 sudo 的命令。阵列状态没做。规格见 [B64](specs/B64.md) |
| B60 | 面板 AI 不再直接操作机器，要通过绑定了这台机器的 Agent（agents.operate_host）；Agent 可以勾选能操作的机器；浮窗加权限档（手动、写入、全部允许）、模型和思考程度；设置 → AI 加新对话的默认权限。规格见 [B60](specs/B60.md) |
| B61 | AI 记忆：面板 AI 能记、改、删，编码 Agent 和机器会话只读，远程 AI 看不到；设置 → AI 有记忆卡片，浮窗里显示“已记住”。规格见 [B61](specs/B61.md) |
| B62 | Git 账号统一：设置里一个“Git 与 GitHub”，GitHub 页面选一个 Git 账号同步，Agent 页不再单独管连接；老的 GitHub 令牌启动时自动变成一个 Git 账号。规格见 [B62](specs/B62.md) |
| B63 | 自动备份可以选 WebDAV（坚果云、Nextcloud、alist 等）和 Google Drive；设置页能测试连接，Google Drive 在面板里授权和撤销，授权过期时自动备份失败并推送通知。规格见 [B63](specs/B63.md) |
| B68 | 云盘页给备份绑定的坚果云（WebDAV）和 Google Drive 各加一个标签，能浏览和下载，可以在隐藏的模块里单独隐藏，不影响备份；Google 授权多申请只读权限；隐藏密码卡片只在解锁时显示；左栏三级菜单默认收起。规格见 [B68](specs/B68.md) |
| B66 | “日历”改名“日程”，二级菜单去掉早报；今日页右上角加“早报”按钮，早报页左上角是“今日 / 早报”，不跟着日程隐藏。规格见 [B66](specs/B66.md) |
| B67 | 快速记录保存后马上用 AI 生成标题和标签，标签直接加上，正文短也生成。规格见 [B67](specs/B67.md) |
| B45 | Safari 工具栏：真机上 `all` 和 `off` 都可以，A 到 F 单独开不行。页面保持原样，测试开关已删。规格见 [B45](specs/B45.md) |
| B41 | 报错统一显示：控制台打印、界面常驻、可复制、请求编号、前端报错写进服务器日志、设置里看最近的报错（`e8aa94b`、`0822494`、`2a4c5db`、`6575840`） |
| B44 | 手机字号放大，字号令牌和检查脚本，输入框 16px（`690dbd4`）。点击区域 44px 没做，见“已知问题” |
| B42 | AI 用量：缓存命中率、按天和模型统计、历史明细、导出，Claude Code 和 Codex 的用量（`b64abb7`、`c87ce7a`） |
| B48 | 二次验证改成设置里可选，关键操作始终验证（`e10f1ed`） |
| B46 | 项目多看板：看板、列表、卡片拖动、成员、归档、复制、活动、标星（`757e92b`、`dc0a8e9`）。没做完的见“已知问题” |
| B43 | API 令牌和 MCP 接口，设置 → 远程访问（`b87b132`、`f6f7ee3`） |
| B47 | Agent 管理：Agent、Git 连接（GitHub、Forgejo）、按远端登记仓库、构建和产物、失败重试、卡片分配和评论、Git 回调、内置 Agent、前端（`0ed675e`、`fc40cd4`、`1453f75`、`3a1c4c3`、`900fb61`）。没做的见“已知问题” |
| B39 | AI 供应商可选 Chat Completions 或 Responses 接口；助手能发图片和文本文件；快速模型单独设思考强度（`c44cc80`、`9dac6fe`） |
| B40 | 笔记阅读模式、AI 润色、手动和保存后生成标题标签、三栏拖动调宽并记住（`51175be`、`0ead369`）。截图 1360px 和 390px 已自查 |
| B38 | Safari 标签页和主屏幕应用、跨浏览器动态高度、安全区、横屏、浮层及主题色适配；Chromium、Firefox、WebKit 交互检查通过，真机 iOS 浏览器栏待复核（`46fec3d`） |
| C4 | 核对电脑与服务器共用的详情页，补电脑代理的日志和 Agent 标签测试；页面截图已检查（本次核对） |
| C3 | 核对页面顶栏动作、二次确认、测试覆盖和页面截图。功能页面无原生确认调用（本次核对） |
| C5 | 移除旧 AI 设置界面和接口，保留旧配置迁移提示；核对文件存储、模块测试及页面截图（本次提交） |
| B35 补全 | GitHub PR 加入最近 7 天关闭和合并的记录，并显示状态（本次提交） |
| B32 后端 | OpenAI 兼容供应商、调用层、用量、笔记自动标题和标签已完成；进度见 [backend-todo.md](backend-todo.md)（`bfb7569`） |
| B33 后端 | 服务器 Agent 的会话、权限、10 个代理工具、风险判断、确认、审计和停止已完成；浏览器主流程通过。提交包括 `bb7e20e`、`e2a040b`、`0551786` 和本次提交 |
| D1 | 移除前端演示数据及开关，旧浏览器的演示设置不再影响真实接口；原版备份在 `demo-data-backup-20260929`（`f74744d`）；变更待合并 |
| B15 | 提醒页保留四张概要卡片，项目页增加独立的“进行中”卡片并统计全部分页数据；完成端到端与深浅主题截图检查（待合并） |
| C2 | 修复 Windows 网络错误归类与手机命令面板高度，合并四处查询刷新 Hook，核对模块接口测试和页面渲染覆盖，完成深浅主题截图及规格清理（待合并） |
| B7 | 运维监控向早报提供未归档续费，覆盖逾期、七日内和跨时区日期（`8f9f761`，待合并） |
| B1 | 部署面板时自动安装本机代理，界面吊销后停止，支持配置卸载（`3396a25`，待合并） |
| B18 | 浏览器按主题订阅事件、代理按需切换概要和详情采样、主机列表接口瘦身（`6237021`，待合并） |
| C1 | 统一前端格式、CI 格式检查、稳定异步测试、修复笔记开发模式自动保存、扩展逐页测试和截图（待合并） |
| B17 | 健身类习惯、训练自动打卡与删除回退、真实界面和端到端验证（`710bc8b`，待合并） |
| B16 | 本地日历事件增删改、CalDAV 条件写回和冲突处理、迁移与端到端流程（`ab7cc44`，待合并） |
| B3 | AI 助手和自动化后端、跨模块动作、演示拦截开关与端到端主流程（`c83a2da`，待合并） |
| B14 | 云盘后端、S3 同步和隐藏空间；390px 上传预览删除（`b914a73`，待合并） |
| B8 | Playwright 端到端主流程和 CI（待合并） |
| B19 | 部署前备份数据库和失败回退（待合并） |
| B11 后端 | 附件上传下载、缩略图、笔记删除时清理文件（待合并） |
| B13 后端 | 隐藏内容：隐藏密码、会话解锁、隐藏笔记不出现在列表搜索和动作里（待合并） |
| B12 后端 | 两步验证可选：跳过绑定、启用或关闭、密码修改和重新验证（待合并） |
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
