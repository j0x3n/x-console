import type { RouteObject } from "react-router";
import ComingSoon from "../../components/ComingSoon";

// 模块入口：路由、命令、事件订阅都从这里注册。开发这个模块时替换占位页面。
export const routes: RouteObject[] = [
  {
    index: true,
    element: <ComingSoon title="Overview" />,
    handle: { title: "Overview" },
  },
];
