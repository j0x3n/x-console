import { lazy } from "react";
import type { RouteObject } from "react-router";
import { KeyRound, Plus } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import { registerNavAction } from "../../lib/navBadges";
import "./i18n";
import "./credentials.css";

registerNavAction("/credentials", {
  icon: Plus,
  label: "New credential",
  run: (navigate) => navigate("/credentials?new=1"),
});

// 页面按需加载（B6），主包里只留路由、命令和样式。
const CredentialsPage = lazy(() => import("./CredentialsPage"));

registerCommands([
  {
    id: "credentials.open",
    title: "打开密钥",
    group: "密钥",
    icon: KeyRound,
    keywords:
      "credentials api key token ssh secret rotate expire 密钥 令牌 到期 更换",
    run: ({ navigate }) => navigate("/credentials"),
  },
  {
    id: "credentials.new",
    title: "新建密钥记录",
    group: "密钥",
    icon: Plus,
    keywords: "credential new token key 新建 密钥 令牌",
    run: ({ navigate }) => navigate("/credentials?new=1"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "credentials",
    element: <CredentialsPage />,
    handle: { title: "Credentials" },
  },
];
