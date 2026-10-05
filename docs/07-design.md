# 界面设计规范

写任何界面之前先读这一篇。颜色、间距、组件都有现成的，照着用，不要自己发明。
交活前用 `npm run shots` 出截图自查（见最后一节）。

> **2026-10-05 起界面在改版（B98 到 B102）。** 新的颜色、组件样子、左栏结构以 [B98 规格](specs/B98.md) 为准，设计稿在 `docs/design/clean/`。改版期间写新界面时，和本文冲突的地方按 B98 来。B98 做完后，它的第一到第三节会合并进本文，这段提示删掉。

## 一、原则

1. **信息先行**：用户打开一页，第一眼看到的是“现在怎么样”（概要卡片），然后才是列表和操作。
2. **一页一个主任务**：主要按钮只有一个（`xc-btn primary`），放在右上角。其他操作是普通按钮、图标按钮或“更多”菜单。
3. **密度适中**：列表行高 36 到 44px，卡片内边距 12 到 16px。不要大片空白，也不要挤在一起。
4. **每种状态都要有样子**：加载中、空、出错、还没上线、手机上。缺一个就是没做完。
5. **不写死颜色**：只用 `styles/tokens.css` 里的 `--xc-*` 变量，深浅两个主题都要能看。

## 二、页面结构

顶栏（B20）分三段，页面本身从概要卡片开始：

```
┌ 顶栏：[折叠] 小标题 · 概况灰字     页面按钮（主要按钮在最右） │ 全局按钮 ┐
├ StatStrip：3 到 5 张紧凑概要卡片 ─────────────────────────────┤
├ Toolbar：左边分类标签（xc-tabs），右边搜索框和视图切换 ──────────┤
├ 主体：列表、网格、看板、表单卡片 ───────────────────────────────┤
└ 空状态 / 出错 / 还没上线：用 States 里的组件 ─────────────────────┘
```

- **不显示大标题**。左上角已经有页面名。`PageHeading` 的 `title` 只给读屏软件；详情页传具体名称，左上角会显示成“项目 / XC 项目”。
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
| 设置标签页 | `features/settings/SecurityTab.tsx`、`features/drive/S3SettingsTab.tsx` |
| 全局浮层 | `features/assistant/AssistantPanel.tsx` |

## 三、组件清单

先找这里有没有，没有再自己写。

| 需要 | 用这个 | 说明 |
| --- | --- | --- |
| 页头 | `components/ui/PageHeading` | `subtitle` 写概况（显示在顶栏），`aside` 放按钮（显示在顶栏） |
| 顶栏里的页面按钮 | `components/layout/PageActions` | 不用 `PageHeading` 的页面直接包按钮 |
| 概要卡片 | `components/ui/Stat`：`StatStrip`、`StatCard` | 默认紧凑。卡片里可以放 `Segments`、`MiniBars`、`Ring` 小图 |
| 工具栏 | `components/ui/Toolbar`：`Toolbar`、`SearchBox`、`Segmented` | 窄屏自动变两行 |
| 分类标签 | `.xc-tabs` + `button.active` | 放进 `Toolbar` 的 `start` |
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
| 侧边栏二级菜单 | `registerNavChildren` + `components/layout/NavChildLinks` | 最多 5 条，多了给“全部 N” |
| 图标 | lucide-react，14 到 17 | 同一行里的图标大小一致 |

## 四、具体数值

| 项 | 值 |
| --- | --- |
| 页面左右内边距 | 24px（手机 16px） |
| 区块之间 | 12 到 20px |
| 卡片 | 内边距 14px 16px，圆角 `--xc-radius-lg`（12px），1px `--xc-border` 边框，不加阴影 |
| 输入框、按钮 | 高 30 到 32px，圆角 6 到 8px |
| 列表行 | 高 36 到 44px，行之间 1px `--xc-border` 分隔线，悬停 `--xc-surface-2` |
| 字号 | 正文 13 到 14px；次要信息 12 到 12.5px；标签和脚注 11px；卡片里的数字 18px（今日页 24px）。写成令牌 `var(--fs-13)`，不要写死像素，见下面“字号令牌” |
| 字重 | 标题和强调 500 到 600，正文 400 |
| 灰度 | 主文字 `--xc-text`，次要 `--xc-text-2`，说明 `--xc-muted`，最弱 `--xc-faint` |
| 强调色 | `--xc-accent` 只用在主要按钮、选中状态、关键数字，不要大面积铺 |
| 浮层 | `--xc-elevated` 底色 + `--xc-border-strong` 边框 + `--xc-shadow` 阴影，圆角 9 到 14px |
| 动画 | 0.12 到 0.2 秒，只用在展开、切换、出现 |

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
