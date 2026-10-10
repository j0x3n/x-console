import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Plus, Users } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import { registerNavAction } from "../../lib/navBadges";
import "./i18n";
import "./contacts.css";

registerNavAction("/contacts", {
  icon: Plus,
  label: "New contact",
  run: (navigate) => navigate("/contacts?new=1"),
});

// 页面按需加载（B6），主包里只留路由、命令和样式。
const ContactsPage = lazy(() => import("./ContactsPage"));

registerCommands([
  {
    id: "contacts.open",
    title: "打开联系人",
    group: "联系人",
    icon: Users,
    keywords:
      "contacts birthday anniversary people 联系人 生日 纪念日 朋友 家人",
    run: ({ navigate }) => navigate("/contacts"),
  },
  {
    id: "contacts.new",
    title: "新建联系人",
    group: "联系人",
    icon: Plus,
    keywords: "contact new birthday 新建 联系人 生日",
    run: ({ navigate }) => navigate("/contacts?new=1"),
  },
]);

export const routes: RouteObject[] = [
  {
    path: "contacts",
    element: <ContactsPage />,
    handle: { title: "Contacts" },
  },
];
