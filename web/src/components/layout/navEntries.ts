import type { LucideIcon } from "lucide-react";
import {
  defaultNavPins,
  entryOf,
  navEntryLabels,
  navEntryIcons,
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
    const members = navItems.filter(
      (n) =>
        (navMerges[entry] ?? [entry]).includes(n.path) &&
        visible.some((v) => v.path === n.path),
    );
    if (members.length === 0) continue;
    seen.add(entry);
    const head = navItems.find((n) => n.path === entry) ?? item;
    entries.push({
      path: entry,
      label: navEntryLabels[entry] ?? head.label,
      icon: navEntryIcons[entry] ?? head.icon,
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
  const list = normalizePins(pins ?? defaultNavPins)
    .map((p) => entries.find((e) => e.path === p))
    .filter((e): e is NavEntry => !!e);
  const here = current && entries.find((e) => e.path === entryOf(current));
  if (here && !list.includes(here)) list.push(here);
  return list;
}

/** B189：左栏的分隔线也存在 navPins 里，写成 /-/<编号>，后端的地址校验能通过。 */
export function isNavSeparator(pin: string): boolean {
  return pin.startsWith("/-/");
}

/** 旧功能固定项归到新入口，保留首次出现的顺序；只在用户操作时保存新列表。 */
export function normalizePins(pins: string[]): string[] {
  return [
    ...new Set(pins.map((pin) => (isNavSeparator(pin) ? pin : entryOf(pin)))),
  ];
}

/** 新分隔线的值：编号取没用过的最小值。 */
export function newSeparator(pins: string[]): string {
  let n = 1;
  while (pins.includes(`/-/${n}`)) n++;
  return `/-/${n}`;
}

export type RailItem =
  | { kind: "entry"; entry: NavEntry }
  | { kind: "sep"; key: string };

/**
 * 图标栏里的入口和分隔线（B189）。入口的规则同 railEntries。
 * 去掉开头、结尾和连着的分隔线，入口被隐藏后不会留下两条线挨在一起。
 */
export function railItems(
  entries: NavEntry[],
  pins: string[] | null,
  current: string | undefined,
): RailItem[] {
  const out: RailItem[] = [];
  for (const pin of normalizePins(pins ?? defaultNavPins)) {
    if (isNavSeparator(pin)) {
      if (out.length && out[out.length - 1].kind === "entry")
        out.push({ kind: "sep", key: pin });
      continue;
    }
    const entry = entries.find((e) => e.path === pin);
    if (entry) out.push({ kind: "entry", entry });
  }
  if (out.length && out[out.length - 1].kind === "sep") out.pop();
  const here = current && entries.find((e) => e.path === entryOf(current));
  if (here && !out.some((item) => item.kind === "entry" && item.entry === here))
    out.push({ kind: "entry", entry: here });
  return out;
}

/** 把 from 位置的固定项移到 to 位置，返回新的列表。 */
export function movePin(pins: string[], from: number, to: number): string[] {
  const list = [...pins];
  const [item] = list.splice(from, 1);
  list.splice(Math.max(0, Math.min(to, list.length)), 0, item);
  return list;
}

/** 固定或取消固定一个入口，返回新的列表。 */
export function togglePin(pins: string[] | null, path: string): string[] {
  const list = normalizePins(pins ?? defaultNavPins);
  const entry = entryOf(path);
  return list.includes(entry)
    ? list.filter((p) => p !== entry)
    : [...list, entry];
}

/** 这个入口有没有固定。 */
export function isPinned(pins: string[] | null, path: string): boolean {
  return normalizePins(pins ?? defaultNavPins).includes(entryOf(path));
}
