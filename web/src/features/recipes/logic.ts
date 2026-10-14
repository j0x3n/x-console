import type {
  Recipe,
  RecipeIngredient,
  RecipeInput,
  RecipeNutrition,
  RecipeStep,
  RecipeTagCount,
} from "./api";
import { detectDurations } from "./durations";

/* 菜谱页面用到的纯逻辑（B209），都有单元测试。 */

/** 编辑表单里的菜谱。和 RecipeInput 一样，只是数组都一定有。 */
export interface Draft {
  title: string;
  summary: string;
  servings: string;
  totalMinutes: number | null;
  ingredients: RecipeIngredient[];
  steps: RecipeStep[];
  tips: string;
  tags: string[];
  sourceNoteId: number | null;
  sourceUrl: string;
  nutrition: RecipeNutrition[];
}

export function emptyDraft(): Draft {
  return {
    title: "",
    summary: "",
    servings: "",
    totalMinutes: null,
    ingredients: [{ name: "", amount: "", note: "" }],
    steps: [{ title: "", text: "", timers: [], ingredients: [] }],
    tips: "",
    tags: [],
    sourceNoteId: null,
    sourceUrl: "",
    nutrition: [],
  };
}

/** 接口返回的菜谱或 AI 草稿转成表单数据。缺的字段补上默认值。 */
export function toDraft(r: RecipeInput | Recipe): Draft {
  return {
    title: r.title ?? "",
    summary: r.summary ?? "",
    servings: r.servings ?? "",
    totalMinutes: r.totalMinutes ?? null,
    ingredients: (r.ingredients ?? []).map((i) => ({
      name: i.name ?? "",
      amount: i.amount ?? "",
      note: i.note ?? "",
    })),
    steps: (r.steps ?? []).map((s) => ({
      title: s.title ?? "",
      text: s.text ?? "",
      timers: (s.timers ?? []).map((t) => ({ ...t })),
      ingredients: [...(s.ingredients ?? [])],
    })),
    tips: r.tips ?? "",
    tags: [...(r.tags ?? [])],
    sourceNoteId: r.sourceNoteId ?? null,
    sourceUrl: r.sourceUrl ?? "",
    nutrition: [...(r.nutrition ?? [])],
  };
}

/** 保存前整理：去掉空的食材行和空步骤，步骤里的下标跟着改。 */
export function toInput(d: Draft): RecipeInput {
  const keep: number[] = [];
  d.ingredients.forEach((ing, i) => {
    if (ing.name.trim()) keep.push(i);
  });
  const remap = new Map(keep.map((old, i) => [old, i]));
  return {
    title: d.title.trim(),
    summary: d.summary.trim(),
    servings: d.servings.trim(),
    totalMinutes: d.totalMinutes,
    ingredients: keep.map((i) => ({
      name: d.ingredients[i].name.trim(),
      amount: (d.ingredients[i].amount ?? "").trim(),
      note: (d.ingredients[i].note ?? "").trim(),
    })),
    steps: d.steps
      .filter((s) => s.text.trim())
      .map((s) => ({
        title: (s.title ?? "").trim(),
        text: s.text.trim(),
        timers: (s.timers ?? []).filter((t) => t.label.trim() && t.seconds > 0),
        ingredients: (s.ingredients ?? [])
          .map((i) => remap.get(i))
          .filter((i): i is number => i !== undefined),
      })),
    tips: d.tips,
    tags: d.tags,
    sourceNoteId: d.sourceNoteId,
    sourceUrl: d.sourceUrl.trim(),
    nutrition: d.nutrition,
  };
}

/** 删掉第 index 个食材。步骤里指向它的去掉，后面的下标减一。 */
export function removeIngredient(d: Draft, index: number): Draft {
  return {
    ...d,
    ingredients: d.ingredients.filter((_, i) => i !== index),
    steps: d.steps.map((s) => ({
      ...s,
      ingredients: (s.ingredients ?? [])
        .filter((i) => i !== index)
        .map((i) => (i > index ? i - 1 : i)),
    })),
  };
}

/** 把第 from 个食材移到 to。步骤里的下标跟着换。 */
export function moveIngredient(d: Draft, from: number, to: number): Draft {
  if (to < 0 || to >= d.ingredients.length || from === to) return d;
  const order = d.ingredients.map((_, i) => i);
  const [moved] = order.splice(from, 1);
  order.splice(to, 0, moved);
  const remap = new Map(order.map((old, i) => [old, i]));
  return {
    ...d,
    ingredients: order.map((i) => d.ingredients[i]),
    steps: d.steps.map((s) => ({
      ...s,
      ingredients: (s.ingredients ?? []).map((i) => remap.get(i) ?? i),
    })),
  };
}

/** 把数组里第 from 个移到 to，越界时原样返回。 */
export function move<T>(list: T[], from: number, to: number): T[] {
  if (to < 0 || to >= list.length || from === to) return list;
  const next = [...list];
  const [item] = next.splice(from, 1);
  next.splice(to, 0, item);
  return next;
}

/**
 * 这一步要加的食材。数据里标了就用标的；没标就按食材名在正文里找。
 */
export function stepIngredients(
  step: RecipeStep,
  ingredients: RecipeIngredient[],
): number[] {
  const marked = (step.ingredients ?? []).filter(
    (i) => i >= 0 && i < ingredients.length,
  );
  if (marked.length > 0) return marked;
  const out: number[] = [];
  ingredients.forEach((ing, i) => {
    const name = ing.name.trim();
    if (name && step.text.includes(name)) out.push(i);
  });
  return out;
}

/** 做菜页的一个计时按钮。 */
export interface TimerChoice {
  label: string;
  seconds: number;
}

/**
 * 这一步的计时按钮：先是数据里的计时（有上限的给两个），
 * 再是正文里认出、但数据里没有的时间。
 */
export function stepTimerChoices(step: RecipeStep): TimerChoice[] {
  const out: TimerChoice[] = [];
  const seen = new Set<number>();
  const add = (label: string, seconds: number) => {
    if (seen.has(seconds)) return;
    seen.add(seconds);
    out.push({ label, seconds });
  };
  const name = (step.title ?? "").trim() || "计时";
  for (const t of step.timers ?? []) {
    add(t.label || name, t.seconds);
    if (t.maxSeconds && t.maxSeconds > t.seconds)
      add(t.label || name, t.maxSeconds);
  }
  for (const d of detectDurations(step.text)) {
    add(name, d.seconds);
    if (d.maxSeconds) add(name, d.maxSeconds);
  }
  return out;
}

/** 分类标签：最常用的 limit 个放在外面，其余收进下拉。选中的标签一定在外面。 */
export function splitTags(
  tags: RecipeTagCount[],
  selected: string,
  limit = 8,
): { shown: string[]; more: string[] } {
  const sorted = [...tags].sort(
    (a, b) => b.count - a.count || a.tag.localeCompare(b.tag, "zh-CN"),
  );
  const shown = sorted.slice(0, limit).map((t) => t.tag);
  const more = sorted.slice(limit).map((t) => t.tag);
  if (selected && more.includes(selected)) {
    shown.push(selected);
    more.splice(more.indexOf(selected), 1);
  }
  return { shown, more };
}

/** 菜单的筛选：标签完全相同，搜索找菜名、简介、食材名和标签。 */
export function filterRecipes(
  items: Recipe[],
  tag: string,
  q: string,
): Recipe[] {
  const needle = q.trim().toLowerCase();
  return items.filter(
    (r) =>
      (!tag || r.tags.includes(tag)) &&
      (!needle ||
        [r.title, r.summary, ...r.tags, ...r.ingredients.map((i) => i.name)]
          .join("\n")
          .toLowerCase()
          .includes(needle)),
  );
}

/** 加一个标签：去掉首尾空格，空的和重复的不加（不区分大小写）。 */
export function addTag(tags: string[], raw: string): string[] {
  const tag = raw.trim();
  if (!tag) return tags;
  if (tags.some((t) => t.toLowerCase() === tag.toLowerCase())) return tags;
  return [...tags, tag];
}

/** 做菜页记住看到第几步。 */
const stepKey = (id: number) => `recipes.cook.${id}`;

export function loadStep(id: number, count: number): number {
  try {
    const n = Number(localStorage.getItem(stepKey(id)));
    return Number.isInteger(n) && n >= 0 && n < count ? n : 0;
  } catch {
    return 0;
  }
}

export function saveStep(id: number, step: number) {
  try {
    if (step <= 0) localStorage.removeItem(stepKey(id));
    else localStorage.setItem(stepKey(id), String(step));
  } catch {
    // 存不了就算了，下次从第一步开始
  }
}
