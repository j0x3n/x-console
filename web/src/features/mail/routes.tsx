import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Mail, MailPlus, Plus } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./mail.css";
import { registerNavAction, registerNavBadge } from "../../lib/navBadges";
import { registerNavChildren } from "../../lib/navChildren";
import { useMailBadge } from "./badge";
import MailNavChildren from "./NavChildren";

registerNavBadge("/mail", useMailBadge);
// 二级菜单：全部收件箱、未读、每个邮箱（用户 2026-10-05 要求）
registerNavChildren("/mail", MailNavChildren);
registerNavAction("/mail", {
  icon: Plus,
  label: "Add mailbox",
  run: (navigate) => navigate("/mail?new=1"),
});

// 页面按需加载（B6）
const MailPage = lazy(() => import("./MailPage"));

registerCommands([
  {
    id: "mail.open",
    title: "未读邮件",
    group: "邮件",
    keywords: "mail inbox gmail unread",
    icon: Mail,
    run: ({ navigate }) => navigate("/mail?unread=1"),
  },
  {
    id: "mail.add",
    title: "添加邮箱",
    group: "邮件",
    keywords: "mail account imap gmail",
    icon: MailPlus,
    run: ({ navigate }) => navigate("/mail?new=1"),
  },
]);

export const routes: RouteObject[] = [
  { path: "mail", element: <MailPage />, handle: { title: "Mail" } },
];
