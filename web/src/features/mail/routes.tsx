import { lazy } from "react";
import type { RouteObject } from "react-router";
import { Mail, MailPlus } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import "./mail.css";
import { registerNavBadge } from "../../lib/navBadges";
import { useMailBadge } from "./badge";

registerNavBadge("/mail", useMailBadge);

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
