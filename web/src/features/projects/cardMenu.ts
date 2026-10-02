import type { CardColor } from "./api";

/*
 * B85：卡片右键菜单用的纯函数。
 */

/** 颜色的顺序和 Trello 差不多。空字符串是“不设”，不在这里。 */
export const CARD_COLORS: Exclude<CardColor, "">[] = [
  "green",
  "yellow",
  "orange",
  "red",
  "purple",
  "blue",
  "sky",
  "lime",
  "pink",
  "gray",
];

export interface DueShortcut {
  /** 英文原文 */
  label: string;
  /** 比如“10月3日 18:00” */
  hint: string;
  at: Date;
}

function at(base: Date, days: number, hour: number) {
  const d = new Date(base);
  d.setDate(d.getDate() + days);
  d.setHours(hour, 0, 0, 0);
  return d;
}

function hintOf(d: Date) {
  const hh = String(d.getHours()).padStart(2, "0");
  return `${d.getMonth() + 1}月${d.getDate()}日 ${hh}:00`;
}

/** 到期时间的快捷项：今天 18:00（已经过了就不给）、明天 18:00、下周一 09:00。 */
export function dueShortcuts(now: Date): DueShortcut[] {
  const list: { label: string; at: Date }[] = [];
  const today = at(now, 0, 18);
  if (today > now) list.push({ label: "Today 18:00", at: today });
  list.push({ label: "Tomorrow 18:00", at: at(now, 1, 18) });
  // 下周一：今天是周一时也是下一个周一
  const toMonday = (8 - now.getDay()) % 7 || 7;
  list.push({ label: "Next Monday 09:00", at: at(now, toMonday, 9) });
  return list.map((x) => ({ ...x, hint: hintOf(x.at) }));
}
