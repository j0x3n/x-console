/*
 * “今日”页的卡片布局。纯逻辑，不依赖 React。
 * 每张卡片固定在左栏或右栏，order 只在同一栏里比较。
 */

export type Column = "main" | "side";

export interface CardDef {
  id: string;
  title: string; // 英文原文，界面里用 t()
  column: Column;
}

/** 默认顺序就是这个数组的顺序。 */
export const cardDefs: CardDef[] = [
  { id: "todos", title: "To do today", column: "main" },
  { id: "decisions", title: "Needs your call", column: "main" },
  { id: "schedule", title: "Schedule", column: "side" },
  { id: "habits", title: "Habits", column: "side" },
  { id: "weather", title: "Weather", column: "side" },
  { id: "home", title: "Smart home", column: "side" },
  { id: "activity", title: "Recent activity", column: "side" },
];

export interface LayoutCard {
  id: string;
  visible: boolean;
  order: number;
}

export function defaultLayout(): LayoutCard[] {
  return cardDefs.map((d, i) => ({ id: d.id, visible: true, order: i }));
}

/**
 * 把服务端存的布局和已知卡片合起来：丢掉不认识的 id，
 * 没存过的卡片按默认显示并排在后面，order 重新编成 0、1、2……
 */
export function normalizeLayout(saved: LayoutCard[] | undefined): LayoutCard[] {
  const known = new Map(cardDefs.map((d, i) => [d.id, i]));
  const seen = new Set<string>();
  const kept = (saved ?? [])
    .filter((c) => known.has(c.id) && !seen.has(c.id) && seen.add(c.id))
    .sort((a, b) => a.order - b.order);
  const missing = cardDefs
    .filter((d) => !seen.has(d.id))
    .map((d) => ({ id: d.id, visible: true, order: 0 }));
  return [...kept, ...missing].map((c, i) => ({
    id: c.id,
    visible: c.visible,
    order: i,
  }));
}

export function columnOf(id: string): Column {
  return cardDefs.find((d) => d.id === id)?.column ?? "main";
}

/** 某一栏的卡片，按 order 排好。 */
export function columnCards(
  layout: LayoutCard[],
  column: Column,
): LayoutCard[] {
  return layout
    .filter((c) => columnOf(c.id) === column)
    .sort((a, b) => a.order - b.order);
}

/** 把卡片移到同一栏里另一张卡片的位置。不同栏之间不移动。 */
export function moveCard(
  layout: LayoutCard[],
  id: string,
  targetId: string,
): LayoutCard[] {
  const column = columnOf(id);
  if (id === targetId || columnOf(targetId) !== column) return layout;
  const list = columnCards(layout, column);
  const from = list.findIndex((c) => c.id === id);
  const to = list.findIndex((c) => c.id === targetId);
  if (from < 0 || to < 0) return layout;
  const [item] = list.splice(from, 1);
  list.splice(to, 0, item);
  return renumber(layout, column, list);
}

/** 在同一栏里上移（-1）或下移（1）一格。 */
export function shiftCard(
  layout: LayoutCard[],
  id: string,
  delta: -1 | 1,
): LayoutCard[] {
  const list = columnCards(layout, columnOf(id));
  const index = list.findIndex((c) => c.id === id);
  const target = list[index + delta];
  return target ? moveCard(layout, id, target.id) : layout;
}

export function toggleCard(layout: LayoutCard[], id: string): LayoutCard[] {
  return layout.map((c) => (c.id === id ? { ...c, visible: !c.visible } : c));
}

function renumber(
  layout: LayoutCard[],
  column: Column,
  ordered: LayoutCard[],
): LayoutCard[] {
  const other = layout.filter((c) => columnOf(c.id) !== column);
  // 两栏各自连续编号，存下来的 order 在同一栏里不重复就行。
  const main = column === "main" ? ordered : columnCards(other, "main");
  const side = column === "side" ? ordered : columnCards(other, "side");
  return [...main, ...side].map((c, i) => ({ ...c, order: i }));
}
