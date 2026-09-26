# xcc 工作台

基于 React 19、TypeScript、Vite、Zustand 和 lucide-react 的工作台界面。项目按业务功能组织，页面、展示组件、跨页面状态、示例数据和工具函数分别维护。

## 开发与构建

```bash
npm install
npm run dev
npm run typecheck
npm test
npm run build
```

## 目录结构

```text
src/
├── main.tsx                    # 挂载应用、加载全局样式
├── app/
│   ├── App.tsx                 # 组合布局、页面、弹窗与状态 hooks
│   └── WorkspaceRouter.tsx     # 根据当前 view 选择业务页面
├── components/
│   ├── layout/                 # Sidebar、Topbar、个人菜单、通知列表
│   ├── dialogs/                # 搜索、委派、草稿审阅及弹窗组合
│   ├── decisions/              # 跨页面复用的决策结果行
│   └── ui/                     # 头像、标识、图表、进度、健康度、Toast 等
├── features/
│   ├── today/                  # 今日工作台及指标、决策、会议、动态组件
│   ├── work/                   # 任务页、筛选栏、看板、列表、卡片、详情抽屉
│   ├── crew/                   # 智能团队页、助手卡片、执行记录、雇佣流程
│   ├── records/                # 公司/联系人列表、筛选、表格、行、统计
│   ├── deals/                  # 商机页、筛选栏、看板、卡片、表格
│   ├── companies/              # 公司详情及概览、时间线、知识卡片
│   ├── people/                 # 联系人详情及动态
│   └── settings/               # 工作台设置页
├── hooks/                      # 路由、偏好、业务状态、弹窗、通知、快捷键
├── stores/                     # Zustand 业务、界面与偏好状态及其测试
├── types/                      # 业务实体、回调接口和 CSS 变量类型
├── contexts/                   # 语言上下文和 useT
├── data/                       # 示例业务数据、共享枚举、默认草稿
├── lib/                        # 路由映射、翻译、时间格式、CSV、决策结果文案
└── styles/
    ├── index.css               # 唯一全局样式入口，保留原有加载顺序
    ├── workspace.css           # 工作台样式分文件的导入入口
    └── workspace/              # 通用、任务、团队、档案、商机、详情样式
```

## 修改功能时从哪里开始

| 修改内容                                         | 主要文件或目录                                                    |
| ------------------------------------------------ | ----------------------------------------------------------------- |
| 导航、侧边栏、页头                               | `components/layout/`                                              |
| 今日指标、待决定事项、会议与动态                 | `features/today/components/`                                      |
| 任务筛选、看板、列表、任务详情                   | `features/work/components/`                                       |
| 助手状态、权限、执行记录                         | `features/crew/components/`                                       |
| 雇佣模板与默认配置                               | `features/crew/hire-config.ts`                                    |
| 公司/联系人列表、搜索、排序、分组                | `features/records/`                                               |
| 档案列表中的保存视图与信号配置                   | `features/records/record-config.ts`                               |
| 商机卡片、阶段看板、表格                         | `features/deals/components/`                                      |
| 商机中的助手信号                                 | `features/deals/deal-signals.ts`                                  |
| 公司概览、时间线、简报、任务、信号、联系人、商机 | `features/companies/components/Company*.tsx`                      |
| 公司详情专用文案与日期格式                       | `features/companies/company-config.ts`                            |
| 委派任务、草稿审阅、搜索弹窗                     | `components/dialogs/`                                             |
| 统一审批、忽略、撤销及跨页面同步                 | `stores/workspace-store.ts`、`hooks/useWorkspaceState.ts`         |
| 新增路径或修改 URL 映射                          | `lib/navigation.ts`、`app/WorkspaceRouter.tsx`                    |
| 主题和语言偏好                                   | `stores/preferences-store.ts`、`hooks/useWorkspacePreferences.ts` |
| 翻译词条                                         | `lib/i18n.ts`                                                     |
| 助手名称/标识/颜色、负责人、阶段等共享配置       | `data/catalogs.ts`                                                |
| 公司、联系人、商机与任务示例数据                 | `data/workspace.ts`                                               |
| 公司详情示例数据                                 | `data/company-details.ts`                                         |
| 今日决策、会议、动态与收藏示例数据               | `data/dashboard.ts`                                               |

## 状态职责

Zustand 按职责划分三个 store：

| Store               | 职责                                                                                   |
| ------------------- | -------------------------------------------------------------------------------------- |
| `workspace-store`   | 决策、审批结果、撤销历史、动态、任务、任务状态和已雇佣助手；审批和撤销原子更新相关状态 |
| `overlay-store`     | 搜索、通知、委派、移动导航、草稿和弹窗状态                                             |
| `preferences-store` | 语言、主题、已有助手的权限模式，以及偏好的读取和保存                                   |

Hooks 负责连接 store 与 React 生命周期、动画和提示：

| Hook                      | 职责                                                           |
| ------------------------- | -------------------------------------------------------------- |
| `useNavigation`           | 当前页面、浏览器前进/后退、URL 更新、锚点滚动                  |
| `useWorkspacePreferences` | 中英文切换、主题模式、系统主题监听、标题与本地偏好             |
| `useWorkspaceState`       | 决策、决策结果、撤销历史、动态、委派任务、任务状态、已雇佣助手 |
| `useOverlayState`         | 弹窗开关、通知开关、移动导航、草稿、委派说明                   |
| `useToast`                | 通知展示、关闭动画、自动消失                                   |
| `useKeyboardShortcuts`    | 搜索、委派、关闭与今日决策快捷键                               |
| `usePresence`             | 关闭动画期间保持组件挂载                                       |

业务页面持有自己的筛选、排序、分组和选中状态。审批和撤销等影响多个页面的操作由 `workspace-store` 统一处理，`useWorkspaceState` 补充本地化提示。页面可通过回调发起操作，也可使用 Zustand selector 订阅所需状态。

当前公司、联系人、商机和任务数据仍是示例数据。决策、委派任务、任务状态和已雇佣助手保存在内存，刷新后重置；语言、主题和已有助手的权限模式使用 `xcc-language`、`xcc-theme`、`xcc-agent-modes` 三个 `localStorage` 键保存。

## 类型约束

所有应用源码使用 `.ts/.tsx`。`tsconfig.json` 启用 `strict`，组件 props、业务数据、状态操作和 DOM 事件均有类型。共用业务类型放在 `types/domain.ts`，页面专用 props 就近定义。`npm run build` 会先执行类型检查。

## 后续扩展

### 增加页面

1. 在 `features/<功能>/` 新建页面，子组件放入该功能的 `components/`。
2. 在 `lib/navigation.ts` 补充路径和页面名称的双向映射。
3. 在 `app/WorkspaceRouter.tsx` 注册页面；需要导航入口时修改 `Sidebar`。
4. 添加翻译和所需样式。新页面需要在命令搜索中出现时更新 `SearchDialog`。

### 增加组件或替换数据来源

- 仅一个功能使用的组件放在该功能目录；跨功能复用的展示组件放在 `components/ui/`。
- 页面负责组合，展示组件通过 props 接收数据和操作回调。
- 共用的助手、阶段和负责人配置统一修改 `data/catalogs.ts`。
- 接入接口时，集中替换数据读取和业务状态逻辑，沿用展示组件的 props 接口。
- 通用工具放在 `lib/`；工具和数据模块不依赖页面组件。

### 修改样式

工作台样式已按原有规则顺序拆到 `styles/workspace/`。`shared.css` 是通用样式；其余文件对应任务看板、团队、档案列表、任务列表、商机、联系人详情和公司详情。

全局样式只从 `styles/index.css` 加载。`theme.css`、`refinements.css` 和 `interaction-polish.css` 仍包含跨功能覆盖规则，修改外观时也需检查这些文件。调整导入顺序会影响 CSS 层叠，应单独验证。

## 本次拆分的验证

- `npm run build` 通过。
- 拆分前后 138 个页面/语言组合的静态 HTML 完全一致，覆盖全部公司和联系人详情、任务列表分组、商机表格及中英文。
- CSS 构建产物与拆分前完全一致。
- 使用真实浏览器验证了审批与撤销、草稿编辑保存、快捷键搜索、档案搜索选择、详情导航、任务视图与历史、委派、商机表格、助手暂停/雇佣/移除/撤销、主题语言切换及移动导航。

## TypeScript 与 Zustand 迁移验证

- 严格类型检查和生产构建通过。
- 9 个状态测试覆盖审批原子更新、重复操作、撤销隔离、动画计时器、函数式更新，以及偏好恢复、无效数据和存储不可用。
- 浏览器检查覆盖 130 个页面/语言组合，以及审批撤销、草稿、搜索、任务、团队操作、偏好保存和移动导航，未发现运行错误。

日常验证使用 `npm test` 和 `npm run build`。当前仍是使用示例数据的前端项目，后端接口和服务端权限将在后续接入。
