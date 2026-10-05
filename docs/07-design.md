# 界面设计规范

写任何界面之前先读这一篇。颜色、间距、组件都有现成的，照着用，不要自己发明。
交活前用 `npm run shots` 出截图自查（见最后一节）。

> 2026-10-05 的界面改版（B98 到 B102，clean 风格、图标栏加二级菜单）已全部合并进本文。当时的规格和设计稿：[B98](specs/B98.md)、`docs/design/clean/`。

## 一、原则

1. **信息先行**：用户打开一页，第一眼看到的是“现在怎么样”（概要卡片），然后才是列表和操作。
2. **一页一个主任务**：主要按钮只有一个（`xc-btn primary`），放在右上角。其他操作是普通按钮、图标按钮或“更多”菜单。
3. **密度适中**：列表行高 36 到 44px，卡片内边距 12 到 16px。不要大片空白，也不要挤在一起。
4. **每种状态都要有样子**：加载中、空、出错、还没上线、手机上。缺一个就是没做完。
5. **不写死颜色**：只用 `styles/tokens.css` 里的 `--xc-*` 变量，深浅两个主题都要能看。
6. **Clean 风格**（B98）：白底、细边框、不加阴影、一个主题色。层次靠边框和留白，不靠底色和阴影。

## 二、页面结构

顶栏（B20）分三段，页面本身从概要卡片开始：

```
┌ 顶栏 58px：[展开二级菜单] 模块图标 页面名 · 概况灰字   页面按钮（主要在最右） │ 全局按钮 ┐
├ StatStrip：3 到 5 张紧凑概要卡片 ─────────────────────────────┤
├ Toolbar：左边分类标签（xc-tabs），右边搜索框和视图切换 ──────────┤
├ 主体：列表、网格、看板、表单卡片 ───────────────────────────────┤
└ 空状态 / 出错 / 还没上线：用 States 里的组件 ─────────────────────┘
```

- **左栏**（B99）：最左边 64px 的图标栏放全部模块，按“主要、个人、设备、集成”分组，组之间一条短横线。鼠标停上去显示模块名。当前模块登记过二级菜单时，右边出现 228px 宽的二级菜单（标题栏 + 菜单项），⌘B 收起或展开。收起按钮在二级菜单标题栏的最右边；收起后顶栏最左边出现展开按钮，没有二级菜单的模块不显示这个按钮（B103）。二级菜单显示时，模块名只在它的标题栏里出现，顶栏不再显示模块图标和模块名。页面里不要再放一份和二级菜单相同的分类、页签或列表（B104 把提醒、日程、习惯、云盘、Agent、仓库页里的都去掉了），手机上从抽屉切换。手机上左栏是抽屉，图标栏和二级菜单一起出来。新模块在 `app/nav.ts` 加一行就会出现在图标栏里。
- **顶栏**（B100）：高 58px，下边 1px 线。页面名 15px、600，前面是 28px 的模块图标（主题色浅底），手机上不显示图标。命令面板的入口在图标栏的搜索按钮，顶栏不再放 ⌘K 按钮。页面从顶栏下面 20px 开始（手机 16px）。
- **不显示大标题**。左上角已经有页面名。今日页例外：问候语 22px、600。今日页的卡片标题和内容在同一张卡片里。`PageHeading` 的 `title` 只给读屏软件；详情页传具体名称，左上角会显示成“项目 / XC 项目”。
- **概况**：`PageHeading` 的 `subtitle`（和 `meta`）显示在左上角标题后面，一行灰字，放不下时省略。宽度小于 1100px 时不显示。
- **页面按钮**：“做一件事”的按钮（新建、上传、调整布局、同步）用 `PageActions` 放进顶栏中段，`PageHeading` 的 `aside` 也会自动放进去。和右边的全局按钮之间有一条竖线。每个按钮都要有图标和 `title`，手机上只显示图标。最多 3 个，多了收进 `MoreMenu`。
- **页面里的工具栏不动**：搜索、筛选、视图切换、分组仍然放在页面的 `Toolbar` 里。空状态里的“新建”按钮也留在原处。
- **内容占满右侧**，`.xc-page` 没有最大宽度。表单卡片可以用 `settings-grid` 那样的自动排列，不要把整页限制成一窄条。
- **详情页**（一个项目、一台服务器、一条规则）：左边主体，右边 260 到 300px 的属性栏；窄屏时属性栏放到主体下面。
- **设置类页面**：卡片网格 `grid-template-columns: repeat(auto-fill, minmax(340px, 1fr))`，每张卡片一件事，底部右对齐按钮。

写新页面时，先复制最像的一页再改：

| 要做的 | 参考 |
| --- | --- |
| 有分类、搜索、列表和网格的数据页 | `features/drive/DrivePage.tsx` |
| 卡片列表 + 开关 + 编辑页 | `features/automations/` |
| 左中右三栏（分类、列表、编辑） | `features/notes/NotesPage.tsx` |
| 看板 | `features/projects/ProjectPage.tsx` |
| 二级菜单里有视图、搜索和分组（B101） | `features/projects/NavChildren.tsx` |
| 跨模块的筛选视图页（B101） | `features/projects/ViewPage.tsx` |
| 设置标签页 | `features/settings/SecurityTab.tsx`、`features/drive/S3SettingsTab.tsx` |
| 全局浮层 | `features/assistant/AssistantPanel.tsx` |

## 三、组件清单

先找这里有没有，没有再自己写。

| 需要 | 用这个 | 说明 |
| --- | --- | --- |
| 页头 | `components/ui/PageHeading` | `subtitle` 写概况（显示在顶栏），`aside` 放按钮（显示在顶栏） |
| 顶栏里的页面按钮 | `components/layout/PageActions` | 不用 `PageHeading` 的页面直接包按钮 |
| 概要卡片 | `components/ui/Stat`：`StatStrip`、`StatCard` | 一排合成一个整体，格子之间 1px 线。默认紧凑。卡片里可以放 `Segments`、`MiniBars`、`Ring` 小图 |
| 工具栏 | `components/ui/Toolbar`：`Toolbar`、`SearchBox`、`Segmented` | 窄屏自动变两行 |
| 分类标签 | `.xc-tabs` + `button.active` | 灰底容器，选中项白底加细边框。放进 `Toolbar` 的 `start` |
| 每行的操作 | `components/ui/MoreMenu` | 桌面下拉，手机从底部弹出 |
| 开关 | `components/ui/Switch` | 列表里开关一条规则 |
| 勾选框 | `<label className="xc-check">` | 下面补说明用 `<small className="xc-check-hint">` |
| 表单 | `.xc-field` 包 `.xc-input` / `.xc-select` / `.xc-textarea` | 说明写在 `<small>` 里 |
| 卡片 | `.xc-card` + `.xc-card-head` | 标题 14px、600 |
| 按钮 | `.xc-btn`，加 `primary` / `danger` / `ghost` / `small` | 链接做成按钮也用它，不会有下划线 |
| 状态标签 | `.xc-badge ok / warn / danger / info / accent` | 状态点 `.xc-dot ok / warn / danger` |
| 弹窗 | `components/ui/Dialog` | 底部按钮放 `.xc-dialog-actions`，取消在左、确认在右 |
| 二次确认 | `components/ui/ConfirmDialog` 的 `confirmAction` | 返回 Promise；`typeToConfirm` 要求输入文字才能确认 |
| 加载、空、出错 | `components/ui/States`：`Loading`、`EmptyState`、`ErrorState` | |
| 还没上线 | `components/ui/States` 的 `NotLive` | 接口回 404 或 501 时整页显示它，不要显示成“出错了” |
| 提示 | `toast("已保存")`、`toast({ message, tone: "error" })` | |
| Markdown 显示 | `components/markdown/Markdown` | |
| 长文字输入（描述、评论、备注） | `components/markdown/MarkdownEditor` | 和笔记一样的格式按钮和预览。不要用裸的 `textarea`；外层用 `div.xc-field`，不要用 `label` 包 |
| 左上角标题 | `PageHeading` 的 `title`、`parents`，或 `usePageCrumb` | 模块名能点回首页，不要再放“返回”按钮 |
| 设备在线状态 | `usePageStatus` | 只有服务器、本机这类有设备状态的页面用，显示成左上角标题后的小点 |
| 二级菜单的菜单项、分组、搜索框 | `components/layout/NavPanel`：`NavPanelStack`、`NavPanelGroup`、`NavPanelLink`、`NavPanelSearch` | 选中由模块传 `active`。参考 `features/projects/NavChildren.tsx`。页面里已经有搜索框时，二级菜单不再放搜索框（B104） |
| 左栏二级菜单 | `registerNavChildren` + `components/layout/NavChildLinks` | 显示在图标栏右边那一栏，最多 30 条，多了给“全部 N”。行高 34px，选中是主题色浅底 |
| 左栏图标上的标记 | `lib/navBadges`：`registerNavBadge`、`registerNavIcon`、`registerNavStatus` | 数量显示在图标右上角（10 以上只有点），状态是右上角的小点，文字进悬停提示 |
| 二级菜单标题栏的按钮 | `registerNavAction` | 比如笔记的“+” |
| 图标 | lucide-react，14 到 17 | 同一行里的图标大小一致 |

## 四、具体数值

### 颜色

只用变量。值在 `styles/tokens.css`，深浅两套：

| 变量 | 浅色 | 深色 | 用在哪 |
| --- | --- | --- | --- |
| `--xc-bg` | `#ffffff` | `#111113` | 内容区底色 |
| `--xc-panel` | `#fafafa` | `#0c0c0e` | 左栏底色 |
| `--xc-surface` | `#ffffff` | `#161618` | 卡片 |
| `--xc-surface-2` | `#fafafa` | `#1c1c1f` | 悬停、看板列、分段切换的底 |
| `--xc-elevated` | `#ffffff` | `#1f1f23` | 弹窗、下拉菜单、分段切换的选中项 |
| `--xc-border` | `#ececee` | `#26262b` | 卡片边框、分隔线 |
| `--xc-border-strong` | `#e4e4e7` | `#34343a` | 输入框、按钮边框 |
| `--xc-text` / `--xc-text-2` | `#18181b` / `#3f3f46` | `#ededf0` / `#c4c4cc` | 主文字 / 次要文字 |
| `--xc-muted` | `#71717a` | `#8b8b94` | 说明、数量 |
| `--xc-faint` | `#a1a1aa` | `#5c5c64` | 只用于图标和占位文字，对比度不够做正文 |
| `--xc-ok` `--xc-warn` `--xc-danger` `--xc-info` | 绿、琥珀、红、蓝 | 浅一档 | 状态。浅底用对应的 `*-soft` |

### 主题色

- 6 种：靛蓝 `indigo`（默认）、海蓝 `ocean`、青色 `teal`、紫罗兰 `violet`、玫瑰 `rose`、石墨 `graphite`。深浅主题都跟着用户选的走。
- 变量：`--xc-accent`（按钮底、进度条、选中图标）、`--xc-accent-soft`（选中项浅底）、`--xc-accent-text`（主题色文字）、`--xc-on-accent`（主题色底上的字，石墨深色时是黑字）。
- 只用在：主要按钮、选中状态、Logo、头像、进度条、关键数字、开关打开。不要大面积铺。
- 原来的褐色 `ember` 和薄荷绿 `mint` 已去掉。旧设置读出来时自动换成 `indigo` 和 `teal`。

### 字体

- `--xc-sans`：Geist，中文接系统字体（苹方、微软雅黑等）。`--xc-mono`：Geist Mono，用于编号、时间、快捷键。
- 字体文件跟着前端打包（`@fontsource-variable/geist`），不要引用 Google Fonts。

### 尺寸

| 项 | 值 |
| --- | --- |
| 页面左右内边距 | 24px（手机 16px） |
| 区块之间 | 16 到 20px |
| 卡片 | 内边距 12px 16px，圆角 `--xc-radius-lg`（10px），1px `--xc-border` 边框，不加阴影 |
| 概要条 | 外框 1px 边框、10px 圆角，格子内边距 12px 16px，格子之间 1px 线 |
| 按钮 | 高 30px（`small` 28px），圆角 7px，白底 1px `--xc-border-strong` 边框；`primary` 主题色底 |
| 输入框 | 高 30 到 32px，圆角 7 到 8px |
| 分段切换、分类标签 | 灰底容器 2px 内边距、9px 圆角；选中项 `--xc-elevated` 底加 1px 边框 |
| 标签、状态标签 | 圆角胶囊（999px），高 20px |
| 列表行 | 高 36 到 44px，行之间 1px `--xc-border` 分隔线，悬停 `--xc-surface-2` |
| 字号 | 正文 13 到 14px；次要信息 12 到 12.5px；标签和脚注 11px；概要卡片里的数字 24px、600、字距 -0.02em（手机 21px） |
| 字重 | 标题和强调 500 到 600，正文 400 |
| 浮层 | `--xc-elevated` 底色 + `--xc-border-strong` 边框 + `--xc-shadow` 阴影，圆角 9 到 14px。只有浮层用阴影 |
| 动画 | 0.12 到 0.2 秒，只用在展开、切换、出现 |

### 项目的状态和优先级图标

在 `features/projects/components/Icons.tsx`，其他模块要显示 Issue 时直接用。

- 状态：待规划灰色虚线圆，待办蓝色空心圆，进行中橙色半满，待审核绿色四分之三满，已完成灰色实心圆加勾，已取消浅灰实心圆加叉。
- 优先级：三格柱状（低 1 格、中 2 格、高 3 格），紧急是红色方块加“!”，无优先级是三个点。

## 五、细节规则

- **数字对齐**：数字用 `font-variant-numeric: tabular-nums`。大小写成 `formatBytes`，时间写成 `relativeTime`（“3 分钟前”），不要直接显示 ISO 时间。
- **长文字**：单行的名称用省略号（`overflow: hidden; text-overflow: ellipsis; white-space: nowrap`），外层要有 `min-width: 0`。
- **空状态要有下一步**：写清楚为什么空，给一个按钮（“新建”“去设置”）。
- **危险操作**：删除用 `danger` 样式。所有删除、停止、重启、结束进程、吊销、卸载、清空先调用 `confirmAction`（`components/ui/ConfirmDialog`），不要用浏览器自带的 `confirm`。标题写清对象，说明写后果，批量操作写数量。高危操作再包 `withElevation`（先确认，再要求提升权限）。
**字号令牌（B44）**：令牌名是桌面上的像素，手机（宽度 ≤ 720px）自动放大一档。定义在 `styles/tokens.css`。

| 令牌 | 桌面 | 手机 |
| --- | --- | --- |
| `--fs-9` | 9px | 11px |
| `--fs-10`、`--fs-10-5` | 10、10.5px | 12px |
| `--fs-11`、`--fs-11-5` | 11、11.5px | 13、13.5px |
| `--fs-12`、`--fs-12-5` | 12、12.5px | 15px |
| `--fs-13`、`--fs-13-5` | 13、13.5px | 16px |
| `--fs-14`、`--fs-15` | 14、15px | 17px |
| `--fs-16` 到 `--fs-19` | 16 到 19px | 19 到 22px |
| `--fs-input` | 16px | 16px（手机上所有输入框都用它，小了 Safari 会放大页面） |

- 小于 9px（图标里的小圆点）和 20px 以上（大标题）写像素，手机上不放大。
- CI 跑 `npm run lint:fonts`，样式里出现 9 到 19px 的写死字号就失败。
- `npm run shots` 在 390px 下检查：文字不小于 11px，输入框等于 16px。

- **手机（390px）**：
  - 整页不能横向滚动。表格和看板在自己的容器里滚动。
  - 列表变单列，次要信息（大小、时间）放到名称下面一行。
  - 行内操作收进 `MoreMenu`，菜单从底部弹出。
  - 主要按钮只留图标。
- **输入框**：直接在页面上写的输入框（笔记标题、正文）去掉全局聚焦框，只留光标；表单里的输入框保留聚焦边框。
- **不要做**：
  - 大标题、页面顶部大片空白。
  - 只有一张光秃秃的表格，没有概要、没有工具栏。
  - 按钮和链接有下划线。
  - 自己写颜色、阴影、圆角的数值。
  - 出错时只显示英文错误或空白。
  - 为了一个页面改公共组件的样子（要改就改组件本身，所有页面一起变）。

## 六、文案

- 中文，短句，一句一个意思，用日常词。不用比喻、拟人、网络流行语。
- 代码里写英文键，`t("Save")`，中文在模块的 `i18n.ts`。同一个英文键全局只能对应一个中文。
- 按钮写动作：“新建笔记”“上传”“保存”，不写“确定”“提交”。
- 状态写结果：“已保存”“已移到回收站”“同步失败”。

## 七、交活前自查

1. 跑 `npm run shots`（需要本地服务端和前端都起着，见脚本开头的说明）。它会：
   - 新建账号、造一些示例数据；
   - 在 1360px 和 390px 打开所有页面，截图存到 `web/screenshots/`；
   - 检查横向溢出和页面报错，有问题时退出码不为 0。
2. 自己看一遍截图，对照下面的清单：
   - [ ] 页面结构和第二节一致：概况 → 概要卡片 → 工具栏 → 主体。
   - [ ] 用的都是第三节的组件，没有自己写一样的东西。
   - [ ] 加载、空、出错、还没上线、手机，五种状态都看过。
   - [ ] 深色和浅色主题都看过（个人菜单里切换）。
   - [ ] 没有下划线、没有大标题、没有横向滚动。
3. PR 里附上新页面在两个宽度下的截图。审查者先看截图。
