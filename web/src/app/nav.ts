import type { LucideIcon } from "lucide-react";
import {
  Bell,
  Bot,
  CalendarDays,
  Code2,
  Github,
  HeartPulse,
  Home,
  Monitor,
  NotebookPen,
  Radar,
  Server,
  Settings2,
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
export const navItems: NavItem[] = [
  { path: "/", label: "My day", icon: Sun, group: "main" },
  { path: "/projects", label: "Projects", icon: SquareKanban, group: "main" },
  { path: "/coding", label: "Coding tasks", icon: Code2, group: "main" },
  { path: "/notes", label: "Notes", icon: NotebookPen, group: "personal" },
  { path: "/reminders", label: "Reminders", icon: Bell, group: "personal" },
  { path: "/habits", label: "Habits", icon: HeartPulse, group: "personal" },
  {
    path: "/calendar",
    label: "Calendar",
    icon: CalendarDays,
    group: "personal",
  },
  { path: "/servers", label: "Servers", icon: Server, group: "machines" },
  { path: "/pc", label: "This PC", icon: Monitor, group: "machines" },
  { path: "/monitoring", label: "Monitoring", icon: Radar, group: "machines" },
  { path: "/home", label: "Smart home", icon: Home, group: "integrations" },
  {
    path: "/automations",
    label: "Automations",
    icon: Workflow,
    group: "integrations",
  },
  { path: "/github", label: "GitHub", icon: Github, group: "integrations" },
  { path: "/assistant", label: "Assistant", icon: Bot, group: "integrations" },
  {
    path: "/settings",
    label: "Settings",
    icon: Settings2,
    group: "integrations",
  },
];

export const navGroupLabels: Record<NavGroup, string> = {
  main: "",
  personal: "Personal",
  machines: "Machines",
  integrations: "Integrations",
};
