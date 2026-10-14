import { describe, expect, it } from "vitest";
import type { Meal, Recipe } from "./api";
import {
  cookedText,
  daysAgo,
  doableRecipes,
  groupMeals,
  hasIngredient,
  matchNutrition,
  slotFor,
} from "./pick";

const base: Omit<Recipe, "id" | "title" | "ingredients"> = {
  summary: "",
  servings: "",
  totalMinutes: null,
  steps: [],
  tips: "",
  tags: [],
  nutrition: [],
  sourceNoteId: null,
  sourceUrl: "",
  cookedCount: 0,
  lastCookedOn: "",
  avgRating: null,
  createdAt: "",
  updatedAt: "",
};
const recipe = (id: number, names: string[], extra: Partial<Recipe> = {}) =>
  ({
    ...base,
    id,
    title: `菜${id}`,
    ingredients: names.map((name) => ({ name })),
    ...extra,
  }) as Recipe;

const staples = ["盐", "生抽", "花生油", "开水"];

describe("我手头有", () => {
  it("食材名和输入的词互相包含就算有", () => {
    expect(hasIngredient("散养公鸡", ["鸡"])).toBe(true);
    expect(hasIngredient("鸡", ["散养公鸡"])).toBe(true);
    expect(hasIngredient("八角", ["鸡", "  "])).toBe(false);
    expect(hasIngredient("Tomato", ["tomato"])).toBe(true);
  });

  it("只留用得上的菜，按还缺几样排，常备调料不算缺", () => {
    const chicken = recipe(1, ["散养公鸡", "花椒", "八角", "盐", "花生油"]);
    const eggs = recipe(2, ["鸡蛋", "番茄", "盐"]);
    const fish = recipe(3, ["草鱼", "生姜"]);
    const out = doableRecipes([chicken, eggs, fish], ["鸡", "番茄"], staples);
    expect(out.map((d) => [d.recipe.id, d.missing])).toEqual([
      [2, []],
      [1, ["花椒", "八角"]],
    ]);
  });

  it("缺的一样多时，用上得多的在前", () => {
    const a = recipe(1, ["鸡", "土豆", "青椒"]);
    const b = recipe(2, ["鸡", "番茄", "青椒"]);
    const out = doableRecipes([a, b], ["鸡", "番茄"], []);
    expect(out.map((d) => d.recipe.id)).toEqual([2, 1]);
  });
});

describe("营养分类筛选", () => {
  it("选中的都要有", () => {
    const r = recipe(1, [], { nutrition: ["high_protein", "low_fat"] });
    expect(matchNutrition(r, [])).toBe(true);
    expect(matchNutrition(r, ["high_protein"])).toBe(true);
    expect(matchNutrition(r, ["high_protein", "veg_rich"])).toBe(false);
  });
});

describe("餐次和日期", () => {
  it("按时间猜是哪一顿", () => {
    const at = (h: number) => new Date(2026, 9, 14, h, 30);
    expect(slotFor(at(7))).toBe("breakfast");
    expect(slotFor(at(12))).toBe("lunch");
    expect(slotFor(at(18))).toBe("dinner");
    expect(slotFor(at(22))).toBe("snack");
    expect(slotFor(at(2))).toBe("snack");
  });

  it("往前数天数跨月", () => {
    expect(daysAgo(new Date(2026, 9, 3), 5)).toBe("2026-09-28");
  });
});

describe("饮食记录按天分组", () => {
  const meal = (id: number, date: string, slot: Meal["slot"]): Meal => ({
    id,
    date,
    slot,
    recipeId: null,
    title: `饭${id}`,
    cooked: false,
    rating: null,
    note: "",
    createdAt: "",
  });

  it("新的日子在前，每天按早午晚加餐排，带上当天体重", () => {
    const days = groupMeals(
      [
        meal(1, "2026-10-13", "dinner"),
        meal(2, "2026-10-14", "snack"),
        meal(3, "2026-10-14", "breakfast"),
        meal(4, "2026-10-14", "breakfast"),
      ],
      [{ date: "2026-10-14", weight: 72.4 }],
    );
    expect(days.map((d) => [d.date, d.weight])).toEqual([
      ["2026-10-14", 72.4],
      ["2026-10-13", null],
    ]);
    expect(
      days[0].slots.map((g) => [g.slot, g.meals.map((m) => m.id)]),
    ).toEqual([
      ["breakfast", [3, 4]],
      ["snack", [2]],
    ]);
  });
});

describe("做过几次", () => {
  const t = (s: string) =>
    ({ Cooked: "做过", times: "次", stars: "分" })[s] ?? s;
  it("没做过是空，有打分时带平均分", () => {
    expect(cookedText({ cookedCount: 0, avgRating: null }, t)).toBe("");
    expect(cookedText({ cookedCount: 3, avgRating: 4.5 }, t)).toBe(
      "做过 3 次 · 4.5 分",
    );
  });
});
