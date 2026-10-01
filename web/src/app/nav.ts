import type { LucideIcon } from "lucide-react";
import {
  Bell,
  CalendarDays,
  Bot,
  Github,
  HardDrive,
  HeartPulse,
  Home,
  Monitor,
  NotebookPen,
  Radar,
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
  { path: "/notes", label: "Notes", icon: NotebookPen, group: "personal" },
  { path: "/reminders", label: "Reminders", icon: Bell, group: "personal" },
  { path: "/habits", label: "Habits", icon: HeartPulse, group: "personal" },
  { path: "/drive", label: "Drive", icon: HardDrive, group: "personal" },
  {
    path: "/calendar",
    label: "Calendar",
    icon: CalendarDays,
    group: "personal",
  },
  { path: "/servers", label: "Servers", icon: Server, group: "machines" },
  { path: "/pc", label: "Computer", icon: Monitor, group: "machines" },
  { path: "/monitoring", label: "Monitoring", icon: Radar, group: "machines" },
  { path: "/home", label: "Smart home", icon: Home, group: "integrations" },
  {
    path: "/automations",
    label: "Automations",
    icon: Workflow,
    group: "integrations",
  },
  { path: "/github", label: "GitHub", icon: Github, group: "integrations" },
];

export const navGroupLabels: Record<NavGroup, string> = {
  main: "",
  personal: "Personal",
  machines: "Machines",
  integrations: "Integrations",
};
