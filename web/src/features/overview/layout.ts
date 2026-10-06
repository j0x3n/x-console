/*
 * “今日”页的卡片布局。纯逻辑，不依赖 React。
 * 每张卡片固定在一栏，order 只在同一栏里比较。
 * top 是标题下面的一行小条（天气），main 是左栏，side 是右边的卡片。
 * 宽屏时 side 会分成几列，见 spreadSide。
 */

export type Column = "top" | "main" | "side";

export interface CardDef {
  id: string;
  title: string; // 英文原文，界面里用 t()
  column: Column;
  /** 大概有多高，分列时用来让几列高度差不多。 */
  weight?: number;
  /** 默认不显示，要在编辑布局里打开。 */
  hidden?: boolean;
  /** 属于哪个模块。模块被隐藏时这张卡片不出现（B57） */
  module?: string;
}

/** 默认顺序就是这个数组的顺序。 */
export const cardDefs: CardDef[] = [
  { id: "todos", title: "To do today", column: "main", module: "projects" },
  {
    id: "decisions",
    title: "Needs your call",
    column: "main",
    module: "projects",
  },
  {
    id: "schedule",
    title: "Schedule",
    column: "side",
    weight: 3,
    module: "calendar",
  },
  {
    id: "habits",
    title: "Habits",
    column: "side",
    weight: 2,
    module: "habits",
  },
  { id: "weather", title: "Weather", column: "top" },
  {
    id: "home",
    title: "Smart home",
    column: "side",
    weight: 2,
    module: "home",
  },
  {
    id: "network",
    title: "Network",
    column: "side",
    weight: 1,
    module: "router",
  },
  // B111：没有额度账号时这张卡片不出现（TodayPage 里判断）
  { id: "quotas", title: "AI quotas", column: "side", weight: 2 },
  {
    id: "mail",
    title: "Mail",
    column: "side",
    weight: 2,
    module: "mail",
  },
  {
    id: "monitoring",
    title: "Monitoring",
    column: "side",
    weight: 2,
    module: "monitoring",
  },
  { id: "activity", title: "Recent activity", column: "side", weight: 3 },
  {
    id: "fitness",
    title: "Workout",
    column: "side",
    weight: 2,
    hidden: true,
    module: "habits",
  },
];

export interface LayoutCard {
  id: string;
  visible: boolean;
  order: number;
  /**
   * B88：放在第几列，0 是主列。没有时按卡片默认的位置（main 在主列，
   * side 平均分到右边几列）。列数比这个少时放到最后一列。
   */
  column?: number;
}

export function defaultLayout(): LayoutCard[] {
  return cardDefs.map((d, i) => ({ id: d.id, visible: !d.hidden, order: i }));
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
    .map((d) => ({ id: d.id, visible: !d.hidden, order: 0 }));
  return [...kept, ...missing].map((c, i) => {
    const out: LayoutCard = { id: c.id, visible: c.visible, order: i };
    const col = (c as LayoutCard).column;
    if (typeof col === "number" && col >= 0 && col <= 3) out.column = col;
    return out;
  });
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
  // 各栏连续编号，存下来的 order 在同一栏里不重复就行。
  const all = (["top", "main", "side"] as Column[]).flatMap((col) =>
    col === column ? ordered : columnCards(layout, col),
  );
  return all.map((c, i) => ({ ...c, order: i }));
}

/**
 * 把右边的卡片按顺序分到 n 列：每张放进当前最矮的一列。
 * n 为 1 时就是原来的一列。
 */
export function spreadSide(cards: LayoutCard[], n: number): LayoutCard[][] {
  const cols: LayoutCard[][] = Array.from({ length: Math.max(1, n) }, () => []);
  const heights = cols.map(() => 0);
  for (const c of cards) {
    const i = heights.indexOf(Math.min(...heights));
    cols[i].push(c);
    heights[i] += cardDefs.find((d) => d.id === c.id)?.weight ?? 2;
  }
  return cols;
}

/** 卡片所属的模块，不属于任何模块时为 null（B57）。 */
export function cardModule(id: string): string | null {
  return cardDefs.find((d) => d.id === id)?.module ?? null;
}

// ---- B88：各列之间自由拖动 ----

const weightOf = (id: string) => cardDefs.find((d) => d.id === id)?.weight ?? 2;

/** 没指定列的卡片默认放哪：main 在主列，side 是 -1（交给 spread 分）。 */
function defaultColumn(c: LayoutCard): number {
  if (c.column !== undefined) return c.column;
  return columnOf(c.id) === "main" ? 0 : -1;
}

/**
 * 把卡片（不含天气条）排成 n 列。指定了列的放进那一列（超出时放最后一列），
 * 没指定的照旧：main 在主列，side 按高度分到右边几列。每列按 order 排。
 * 只有一列时，按“列号、order”排成一列。
 */
export function arrange(
  layout: LayoutCard[],
  n: number,
  include: (c: LayoutCard) => boolean = () => true,
): LayoutCard[][] {
  const list = layout
    .filter((c) => columnOf(c.id) !== "top" && include(c))
    .sort((a, b) => a.order - b.order);
  if (n <= 1) {
    const rank = (c: LayoutCard) => {
      const col = defaultColumn(c);
      return col < 0 ? 1 : col;
    };
    return [[...list].sort((a, b) => rank(a) - rank(b) || a.order - b.order)];
  }
  const cols: LayoutCard[][] = Array.from({ length: n }, () => []);
  const heights = cols.map(() => 0);
  const auto: LayoutCard[] = [];
  for (const c of list) {
    const col = defaultColumn(c);
    if (col < 0) {
      auto.push(c);
      continue;
    }
    const i = Math.min(col, n - 1);
    cols[i].push(c);
    heights[i] += weightOf(c.id);
  }
  for (const c of auto) {
    const side = heights.slice(1);
    const i = 1 + side.indexOf(Math.min(...side));
    cols[i].push(c);
    heights[i] += weightOf(c.id);
  }
  return cols.map((col) => col.sort((a, b) => a.order - b.order));
}

/**
 * 把卡片放到第 col 列、beforeId 这张卡片的前面（null 表示放到这一列最后）。
 * 放完后所有卡片都记下自己在哪一列，order 按列重新编号。
 */
export function placeCard(
  layout: LayoutCard[],
  id: string,
  col: number,
  beforeId: string | null,
  n: number,
): LayoutCard[] {
  if (id === beforeId || columnOf(id) === "top") return layout;
  const moving = layout.find((c) => c.id === id);
  if (!moving) return layout;
  const cols = arrange(layout, n).map((list) =>
    list.filter((c) => c.id !== id),
  );
  const target = Math.max(0, Math.min(col, cols.length - 1));
  if (n <= 1) {
    // 一列时记下的是“排在哪一段”：跟着后面那张卡片，或者最后一张
    const list = cols[0];
    const at = beforeId ? list.findIndex((c) => c.id === beforeId) : -1;
    const ref = at >= 0 ? list[at] : list[list.length - 1];
    const refCol = ref ? defaultColumn(ref) : 0;
    const placed = { ...moving, column: refCol < 0 ? 1 : refCol };
    if (at >= 0) list.splice(at, 0, placed);
    else list.push(placed);
    return renumberColumns(layout, cols, true);
  }
  const list = cols[target];
  const at = beforeId ? list.findIndex((c) => c.id === beforeId) : -1;
  const placed = { ...moving };
  if (at >= 0) list.splice(at, 0, placed);
  else list.push(placed);
  return renumberColumns(layout, cols, false);
}

function renumberColumns(
  layout: LayoutCard[],
  cols: LayoutCard[][],
  keepColumn: boolean,
): LayoutCard[] {
  const top = layout.filter((c) => columnOf(c.id) === "top");
  const rest = cols.flatMap((list, i) =>
    list.map((c) => {
      if (keepColumn) {
        const col = defaultColumn(c);
        return { ...c, column: col < 0 ? 1 : col };
      }
      return { ...c, column: i };
    }),
  );
  return [...top, ...rest].map((c, i) => ({ ...c, order: i }));
}

/** 在自己那一列里上移（-1）或下移（1）一格。 */
export function shiftInColumn(
  layout: LayoutCard[],
  id: string,
  delta: -1 | 1,
  n: number,
  include?: (c: LayoutCard) => boolean,
): LayoutCard[] {
  const cols = arrange(layout, n, include);
  const col = cols.findIndex((list) => list.some((c) => c.id === id));
  if (col < 0) return layout;
  const list = cols[col];
  const i = list.findIndex((c) => c.id === id);
  if (delta < 0) {
    if (i === 0) return layout;
    return placeCard(layout, id, col, list[i - 1].id, n);
  }
  if (i === list.length - 1) return layout;
  return placeCard(layout, id, col, list[i + 2]?.id ?? null, n);
}
