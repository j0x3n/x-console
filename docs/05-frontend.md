# 前端开发约定

## 技术栈

React 19、TypeScript（严格模式）、Vite、React Router、TanStack Query、Zustand、lucide-react、openapi-fetch。测试用 Vitest。

| 数据 | 放哪里 |
| --- | --- |
| 服务端数据 | TanStack Query。不要复制到 Zustand |
| 跨页面的界面状态 | Zustand，比如主题、提升权限弹窗、提示 |
| 页面内状态 | useState |
| 需要记住的界面偏好 | localStorage，读写包在 try/catch 里 |

## 目录

```
web/src/
  app/            App、Layout、routes.tsx（汇总路由）、nav.ts（侧边栏）、
                  TopbarActions.tsx 和 GlobalPanels.tsx（全局入口位置）
  api/            client.ts、query.ts、events.ts、core.ts、gen/<模块>.ts（生成的类型）
  auth/           登录、初始化、提升权限
  components/     layout/、ui/、command/
  features/<模块>/ 模块的全部前端代码
  hooks/ lib/ stores/ contexts/ styles/ types/
```

## 一个模块的文件

```
features/projects/
  routes.tsx        入口。导出 routes，注册命令和事件刷新，import 样式和 i18n
  api.ts            这个模块的 openapi 客户端、query key、useQuery/useMutation hooks
  i18n.ts           registerZh({...})
  projects.css      模块样式（可选）
  ProjectsPage.tsx  页面
  components/       组件
  *.test.ts(x)      测试
```

`routes.tsx` 示例：

```tsx
import type { RouteObject } from "react-router";
import { SquarePlus } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./projects.css";
import ProjectsPage from "./ProjectsPage";
import IssuePage from "./IssuePage";

registerCommands([
  { id: "projects.new-issue", title: "新建 Issue", group: "项目", icon: SquarePlus,
    run: ({ navigate }) => navigate("/projects?new=1") },
]);

export const routes: RouteObject[] = [
  { path: "projects", element: <ProjectsPage />, handle: { title: "Projects" } },
  { path: "projects/:key", element: <IssuePage />, handle: { title: "Projects" } },
];
```

`handle.title` 是页头显示的标题，写英文原文，中文在 i18n 里。

## 调接口

```ts
// features/projects/api.ts
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/projects";

export const projectsApi = createApi<paths>();
export type Issue = components["schemas"]["Issue"];

export const projectKeys = {
  all: ["projects"] as const,
  issues: (projectId: number) => ["projects", projectId, "issues"] as const,
};

// 收到 issue.* 事件时刷新所有 ["projects", ...] 查询
invalidateOn("issue.", projectKeys.all);

export function useIssues(projectId: number) {
  return useQuery({
    queryKey: projectKeys.issues(projectId),
    queryFn: () => unwrap(projectsApi.GET("/projects/{projectId}/issues", { params: { path: { projectId } } })),
  });
}
```

规则：

- 接口类型只从 `api/gen/<模块>.ts` 取。改了 yaml 就跑 `npm run gen:api`，生成文件要提交。
- query key 的第一段是模块名，方便按模块整体刷新。
- 不要写轮询。服务端数据变化会发事件，用 `invalidateOn` 或 `useServerEvent` 响应。实时指标这类高频数据用 `useServerEvent` 直接写进 Query 缓存。
- 出错时 `ApiError` 带 `status` 和 `code`。`integration_not_configured` 显示“去设置”的链接。
- 高危操作包在 `withElevation(() => ...)` 里，它会自动弹出验证码框并重试。
- 修改成功后用 `toast("已保存")` 提示，失败用 `toast({ message, tone: "error" })`。
- WebSocket 流（终端、日志）用 `wsUrl("/hosts/1/terminal")` 拼地址。

## 组件和样式

先用现成的：

| 需要 | 用这个 |
| --- | --- |
| 页面外框 | `<div className="xc-page">` + `<PageHeading title aside />` |
| 按钮 | `xc-btn`，加 `primary`、`danger`、`ghost`、`small` |
| 表单 | `xc-field` 包 `xc-input`、`xc-select`、`xc-textarea` |
| 卡片 | `xc-card`、`xc-card-head` |
| 表格 | `xc-table-wrap` 包 `xc-table` |
| 状态 | `xc-badge ok/warn/danger/info/accent`、`xc-dot ok/warn/danger` |
| 标签页 | `xc-tabs`，当前项加 `active` |
| 弹窗 | `components/ui/Dialog` |
| 加载、空、出错 | `components/ui/States` 里的 `Loading`、`EmptyState`、`ErrorState` |
| 小图表 | `components/ui/LineChart`、`Progress`、`MetricCard` |
| 时间和大小 | `lib/time.ts` 的 `relativeTime`、`formatDate`、`formatTime`、`formatBytes` |

- 颜色只用 `styles/tokens.css` 里的 `--xc-*` 变量，这样深浅主题都对。
- `PageHeading` 的标题只供无障碍工具读取。可见标题由左上角的 Topbar 显示，不在内容区重复显示。
- 模块样式的类名加模块前缀，比如 `.projects-board`。
- 390px 宽度下页面不能横向滚动。表格放在 `xc-table-wrap` 里自己滚动。
- 图标用 lucide-react，尺寸 14 到 17。

## 全局入口

- 命令面板：`registerCommands`。
- 页头按钮：在 `app/TopbarActions.tsx` 里加一行组件。
- 全局浮层（AI 助手面板、番茄钟）：在 `app/GlobalPanels.tsx` 里加一行组件。
- 设置页标签：在 `features/settings/tabs.tsx` 里加一行。
- 侧边栏：`app/nav.ts` 已经列好所有模块，一般不用改。

## 文案

- 界面默认中文。代码里写英文原文，`t("Save")`，中文在模块的 `i18n.ts` 里用 `registerZh` 注册。
- 中文词典是全局共用的。同一个英文键在所有模块里必须对应同一个中文。意思不同就换一个更具体的英文键，比如 `Lock screen` 和 `Lock`。`src/lib/i18n.test.ts` 会检查冲突。
- 只在当前模块内出现、不需要英文版的长句，可以直接写中文。
- 中文文案短句为主，一句一个意思，用日常词。

## 测试

- 纯逻辑（过滤、排序、分组、格式化、状态转换）写 Vitest 单元测试。
- 组件测试用 `@testing-library/react`，文件顶部加 `// @vitest-environment jsdom`。
- 提交前：`npm run typecheck`、`npm test`、`npm run build` 都通过。
