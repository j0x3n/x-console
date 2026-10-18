import type { LucideIcon } from "lucide-react";
import {
  Bell,
  Bookmark,
  CalendarClock,
  CalendarDays,
  Bot,
  FolderGit2,
  Files,
  GraduationCap,
  Headphones,
  HardDrive,
  HeartPulse,
  Inbox,
  FlaskConical,
  KeyRound,
  Hourglass,
  Home,
  Layers,
  Newspaper,
  Library,
  Mail,
  Monitor,
  NotebookPen,
  Radar,
  Router as RouterIcon,
  Scale,
  Server,
  SquareKanban,
  Sun,
  Target,
  Users,
  ChefHat,
  Workflow,
  Network,
  CircleUserRound,
  SlidersHorizontal,
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
  // 用户 2026-10-10 要求：稍后阅读放在笔记下面
  {
    path: "/readlater",
    label: "Read later",
    icon: Bookmark,
    group: "personal",
  },
  // B130：复习卡片，放在稍后阅读下面
  {
    path: "/cards",
    label: "Review cards",
    icon: Layers,
    group: "personal",
  },
  { path: "/feeds", label: "Feeds", icon: Newspaper, group: "personal" },
  { path: "/ask", label: "Ask my data", icon: Library, group: "personal" },
  { path: "/mail", label: "Mail", icon: Mail, group: "personal" },
  {
    path: "/calendar",
    label: "Schedule & focus",
    icon: CalendarDays,
    group: "personal",
  },
  { path: "/reminders", label: "Reminders", icon: Bell, group: "personal" },
  { path: "/plan", label: "Day plan", icon: CalendarClock, group: "personal" },
  { path: "/inbox", label: "Capture inbox", icon: Inbox, group: "personal" },
  // B147：音乐。歌放在云盘里，全局播放器在 GlobalPanels
  { path: "/music", label: "Music", icon: Headphones, group: "personal" },
  { path: "/drive", label: "Drive", icon: HardDrive, group: "machines" },
  { path: "/servers", label: "Servers", icon: Server, group: "machines" },
  { path: "/pc", label: "Computer", icon: Monitor, group: "machines" },
  { path: "/router", label: "Router", icon: RouterIcon, group: "integrations" },
  { path: "/home", label: "Smart home", icon: Home, group: "integrations" },
  {
    path: "/screentime",
    label: "Screen time",
    icon: Hourglass,
    group: "machines",
  },
  { path: "/habits", label: "Habits", icon: HeartPulse, group: "personal" },
  // B128：季度目标和每周复盘
  { path: "/goals", label: "Goals", icon: Target, group: "personal" },
  {
    path: "/checkins",
    label: "Check-ins",
    icon: HeartPulse,
    group: "personal",
  },
  {
    path: "/experiments",
    label: "Experiments",
    icon: FlaskConical,
    group: "personal",
  },
  { path: "/skills", label: "Skills", icon: GraduationCap, group: "personal" },
  // B115：证件、合同和物品保修
  { path: "/documents", label: "Documents", icon: Files, group: "personal" },
  // B122：联系人和重要日期
  { path: "/contacts", label: "Contacts", icon: Users, group: "personal" },
  { path: "/recipes", label: "Recipes", icon: ChefHat, group: "personal" },
  { path: "/forecasts", label: "Forecasts", icon: Scale, group: "personal" },
  { path: "/monitoring", label: "Monitoring", icon: Radar, group: "machines" },
  // B120：密钥和令牌。用户 2026-10-10 要求放在监控下面
  {
    path: "/credentials",
    label: "Credentials",
    icon: KeyRound,
    group: "machines",
  },
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

/**
 * 合并的入口（B179）。键是入口地址，值是组里的功能，第一个就是入口自己。
 * 合并后在左栏和启动台里只占一个位置，二级菜单顶部可以在组里切换。地址都不变。
 */
export const navMerges: Record<string, string[]> = {
  "/coding": ["/coding", "/github"],
  // B198：知识库单独成一个入口，合在笔记里找不到
  "/notes": ["/notes", "/readlater", "/cards", "/feeds"],
  "/calendar": ["/calendar", "/reminders", "/plan", "/inbox"],
  "/servers": ["/servers", "/pc", "/router", "/home", "/screentime"],
  "/habits": [
    "/habits",
    "/goals",
    "/checkins",
    "/experiments",
    "/skills",
    "/documents",
    "/contacts",
    "/recipes",
    "/forecasts",
  ],
  "/monitoring": ["/monitoring", "/credentials", "/automations"],
};

/** 合并入口在左栏和启动台里用的名字，没写的用入口自己的名字。 */
export const navEntryLabels: Record<string, string> = {
  "/servers": "Devices",
  "/habits": "Personal",
  "/monitoring": "Ops",
};

/** 合并入口使用独立图标，功能本身的图标保持原样。 */
export const navEntryIcons: Record<string, LucideIcon> = {
  "/servers": Network,
  "/habits": CircleUserRound,
  "/monitoring": SlidersHorizontal,
};

/** 左栏默认固定的入口（B179）。今日一直在顶部，不在这里。 */
export const defaultNavPins = [
  "/projects",
  "/coding",
  "/-/1",
  "/notes",
  "/ask",
  "/mail",
  "/calendar",
  "/music",
  "/drive",
  "/-/2",
  "/servers",
  "/habits",
  "/monitoring",
];

/** 功能所在的入口地址。不在合并组里的就是它自己。 */
export function entryOf(path: string): string {
  for (const [entry, members] of Object.entries(navMerges))
    if (members.includes(path)) return entry;
  return path;
}

/** 启动台里分组的名字。main 组在左栏没有标题，启动台里叫“工作”。 */
export const launcherGroupLabels: Record<NavGroup, string> = {
  main: "Work",
  personal: "Personal",
  machines: "Machines",
  integrations: "Integrations",
};
