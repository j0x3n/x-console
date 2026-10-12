import type { LucideIcon } from "lucide-react";
import {
  defaultNavPins,
  entryOf,
  navEntryLabels,
  navItems,
  navMerges,
  type NavGroup,
  type NavItem,
} from "../../app/nav";

/**
 * 左栏和启动台里的一个入口（B179）。合并的组是一个入口，members 是组里能看到的功能；
 * 没合并的功能 members 只有它自己。
 */
export interface NavEntry {
  path: string;
  label: string;
  icon: LucideIcon;
  group: NavGroup;
  members: NavItem[];
}

/** 把能看到的功能整理成入口，顺序跟 navItems 一样。今日不在里面，它一直固定在顶部。 */
export function buildEntries(visible: NavItem[]): NavEntry[] {
  const seen = new Set<string>();
  const entries: NavEntry[] = [];
  for (const item of navItems) {
    if (item.path === "/") continue;
    const entry = entryOf(item.path);
    if (seen.has(entry)) continue;
    const members = (navMerges[entry] ?? [entry])
      .map((path) => visible.find((v) => v.path === path))
      .filter((m): m is NavItem => !!m);
    if (members.length === 0) continue;
    seen.add(entry);
    const head = navItems.find((n) => n.path === entry) ?? item;
    entries.push({
      path: entry,
      label: navEntryLabels[entry] ?? head.label,
      icon: head.icon,
      group: head.group,
      members,
    });
  }
  return entries;
}

/**
 * 图标栏里显示的入口：固定的按固定顺序排，当前所在的入口没固定时临时排在最后。
 * pins 为 null 时用默认值。不认识的地址忽略。
 */
export function railEntries(
  entries: NavEntry[],
  pins: string[] | null,
  current: string | undefined,
): NavEntry[] {
  const list = (pins ?? defaultNavPins)
    .map((p) => entries.find((e) => e.path === p))
    .filter((e): e is NavEntry => !!e);
  const here = current && entries.find((e) => e.path === entryOf(current));
  if (here && !list.includes(here)) list.push(here);
  return list;
}

/** 固定或取消固定一个入口，返回新的列表。 */
export function togglePin(pins: string[] | null, path: string): string[] {
  const list = pins ?? defaultNavPins;
  return list.includes(path) ? list.filter((p) => p !== path) : [...list, path];
}

/** 这个入口有没有固定。 */
export function isPinned(pins: string[] | null, path: string): boolean {
  return (pins ?? defaultNavPins).includes(path);
}
