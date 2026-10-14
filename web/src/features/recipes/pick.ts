import type { DayWeight, Meal, MealSlot, Recipe, RecipeNutrition } from "./api";

/* B210 的纯逻辑：营养分类、“我手头有”、饮食记录按天分组。都有单元测试。 */

export const NUTRITION: RecipeNutrition[] = [
  "high_protein",
  "veg_rich",
  "low_fat",
  "oily",
  "high_carb",
  "sugary",
];

/** 营养分类的英文键，中文在 i18n.ts。 */
export const NUTRITION_LABELS: Record<RecipeNutrition, string> = {
  high_protein: "High protein",
  veg_rich: "Lots of vegetables",
  low_fat: "Little oil",
  oily: "Oily",
  high_carb: "Mostly staple food",
  sugary: "Sweet",
};

/** 少油、蔬菜多这种用绿色，油重、偏甜用琥珀色，其余不加颜色。 */
export function nutritionTone(n: RecipeNutrition): string {
  if (n === "veg_rich" || n === "low_fat" || n === "high_protein") return "ok";
  if (n === "oily" || n === "sugary") return "warn";
  return "";
}

export const SLOTS: MealSlot[] = ["breakfast", "lunch", "dinner", "snack"];

export const SLOT_LABELS: Record<MealSlot, string> = {
  breakfast: "Breakfast",
  lunch: "Lunch",
  dinner: "Dinner",
  snack: "Snack",
};

/** 按当前时间猜是哪一顿：10 点前早饭，10 到 15 点午饭，15 到 21 点晚饭，其余加餐。 */
export function slotFor(date: Date): MealSlot {
  const h = date.getHours();
  if (h >= 5 && h < 10) return "breakfast";
  if (h >= 10 && h < 15) return "lunch";
  if (h >= 15 && h < 21) return "dinner";
  return "snack";
}

/** 本地日期 YYYY-MM-DD。 */
export function localDate(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** 往前数 days 天的本地日期。 */
export function daysAgo(today: Date, days: number): string {
  const d = new Date(today);
  d.setDate(d.getDate() - days);
  return localDate(d);
}

const norm = (s: string) => s.trim().toLowerCase();

/** 食材名和输入的词互相包含就算有，比如“鸡”能对上“散养公鸡”。 */
export function hasIngredient(name: string, words: string[]): boolean {
  const n = norm(name);
  if (!n) return true;
  return words.some((w) => {
    const v = norm(w);
    return v !== "" && (n.includes(v) || v.includes(n));
  });
}

export interface Doable {
  recipe: Recipe;
  /** 还缺的食材名，常备调料不算 */
  missing: string[];
  /** 用上了几样手头有的 */
  used: number;
}

/**
 * “我手头有”：只留至少用上一样手头食材的菜，按还缺几样从少到多排，
 * 一样多时用上得多的在前。常备调料不算缺。
 */
export function doableRecipes(
  recipes: Recipe[],
  have: string[],
  staples: string[],
): Doable[] {
  const out: Doable[] = [];
  for (const recipe of recipes) {
    const missing: string[] = [];
    let used = 0;
    for (const ing of recipe.ingredients) {
      if (hasIngredient(ing.name, have)) used++;
      else if (!hasIngredient(ing.name, staples)) missing.push(ing.name);
    }
    if (used > 0) out.push({ recipe, missing, used });
  }
  return out.sort(
    (a, b) => a.missing.length - b.missing.length || b.used - a.used,
  );
}

/** 营养分类筛选：选中的都要有。 */
export function matchNutrition(r: Recipe, wanted: RecipeNutrition[]): boolean {
  return wanted.every((n) => (r.nutrition ?? []).includes(n));
}

export interface MealDay {
  date: string;
  weight: number | null;
  slots: { slot: MealSlot; meals: Meal[] }[];
}

/** 饮食记录按天分组，新的日子在前；每天按早午晚加餐排，空的餐次不列。 */
export function groupMeals(meals: Meal[], weights: DayWeight[]): MealDay[] {
  const byDay = new Map<string, Meal[]>();
  for (const m of meals) {
    const list = byDay.get(m.date) ?? [];
    list.push(m);
    byDay.set(m.date, list);
  }
  const weightOf = new Map(weights.map((w) => [w.date, w.weight]));
  return [...byDay.keys()]
    .sort((a, b) => b.localeCompare(a))
    .map((date) => {
      const list = byDay.get(date)!;
      return {
        date,
        weight: weightOf.get(date) ?? null,
        slots: SLOTS.map((slot) => ({
          slot,
          meals: list
            .filter((m) => m.slot === slot)
            .sort((a, b) => a.id - b.id),
        })).filter((g) => g.meals.length > 0),
      };
    });
}

/** “做过 3 次 · 4.5 分”这一行。没做过返回空字符串。 */
export function cookedText(
  r: Pick<Recipe, "cookedCount" | "avgRating">,
  t: (s: string) => string,
): string {
  const parts: string[] = [];
  if (r.cookedCount > 0)
    parts.push(`${t("Cooked")} ${r.cookedCount} ${t("times")}`);
  if (r.avgRating != null) parts.push(`${r.avgRating} ${t("stars")}`);
  return parts.join(" · ");
}
