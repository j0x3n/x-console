import type { LucideIcon } from "lucide-react";
import {
  Bell,
  CalendarDays,
  Bot,
  FolderGit2,
  Files,
  Gauge,
  HardDrive,
  HeartPulse,
  Hourglass,
  Home,
  Mail,
  Monitor,
  NotebookPen,
  Radar,
  Router as RouterIcon,
  Server,
  SquareKanban,
  Sun,
  Workflow,
} from "lucide-react";

export type NavGroup = "main" | "personal" | "machines" | "integrations";

export interface NavItem {
  path: string;
  label: string;
  icon: LucideIcon;
  group: NavGroup;
}

// 侧边栏导航。每个模块一行，label 是英文原文，中文在 lib/i18n.ts。
// 设置不在这里，入口在左下角的个人菜单（Sidebar 的 ProfileMenu）。
export const navItems: NavItem[] = [
  { path: "/", label: "My day", icon: Sun, group: "main" },
  { path: "/projects", label: "Projects", icon: SquareKanban, group: "main" },
  { path: "/coding", label: "Agents", icon: Bot, group: "main" },
  // B70：GitHub 改名“仓库”，挪到 Agent 下面。地址还是 /github
  {
    path: "/github",
    label: "Repositories",
    icon: FolderGit2,
    group: "main",
  },
  { path: "/notes", label: "Notes", icon: NotebookPen, group: "personal" },
  { path: "/mail", label: "Mail", icon: Mail, group: "personal" },
  { path: "/reminders", label: "Reminders", icon: Bell, group: "personal" },
  { path: "/habits", label: "Habits", icon: HeartPulse, group: "personal" },
  {
    path: "/calendar",
    label: "Schedule & focus",
    icon: CalendarDays,
    group: "personal",
  },
  // B115：证件、合同和物品保修
  { path: "/documents", label: "Documents", icon: Files, group: "personal" },
  { path: "/servers", label: "Servers", icon: Server, group: "machines" },
  // 用户 2026-10-05 要求：云盘放在服务器下面
  { path: "/drive", label: "Drive", icon: HardDrive, group: "machines" },
  { path: "/pc", label: "Computer", icon: Monitor, group: "machines" },
  { path: "/monitoring", label: "Monitoring", icon: Radar, group: "machines" },
  // B116：电脑时间去向
  {
    path: "/screentime",
    label: "Screen time",
    icon: Hourglass,
    group: "machines",
  },
  { path: "/home", label: "Smart home", icon: Home, group: "integrations" },
  { path: "/router", label: "Router", icon: RouterIcon, group: "integrations" },
  { path: "/quotas", label: "AI quotas", icon: Gauge, group: "integrations" },
  {
    path: "/automations",
    label: "Automations",
    icon: Workflow,
    group: "integrations",
  },
];

export const navGroupLabels: Record<NavGroup, string> = {
  main: "",
  personal: "Personal",
  machines: "Machines",
  integrations: "Integrations",
};
