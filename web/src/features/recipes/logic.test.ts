import { describe, expect, it } from "vitest";
import type { Recipe } from "./api";
import { clock, detectDurations, durationLabel } from "./durations";
import {
  addTag,
  emptyDraft,
  filterRecipes,
  moveIngredient,
  removeIngredient,
  splitTags,
  stepIngredients,
  stepTimerChoices,
  toInput,
  type Draft,
} from "./logic";

const pick = (text: string) =>
  detectDurations(text).map((d) => [d.seconds, d.maxSeconds ?? null]);

describe("detectDurations", () => {
  it("认数字加单位", () => {
    expect(pick("烧开后转小火，炖 20 分钟即可")).toEqual([[1200, null]]);
    expect(pick("炖20分钟")).toEqual([[1200, null]]);
    expect(pick("焖 1.5 小时")).toEqual([[5400, null]]);
    expect(pick("静置 30 秒")).toEqual([[30, null]]);
  });

  it("认半小时和中文数字", () => {
    expect(pick("用冷水浸泡半小时，中途换一次水")).toEqual([[1800, null]]);
    expect(pick("泡半个小时")).toEqual([[1800, null]]);
    expect(pick("炖一个半小时")).toEqual([[5400, null]]);
    expect(pick("腌十五分钟")).toEqual([[900, null]]);
    expect(pick("煮二十分钟")).toEqual([[1200, null]]);
    expect(pick("等两分钟")).toEqual([[120, null]]);
    expect(pick("炖一小时")).toEqual([[3600, null]]);
  });

  it("认范围", () => {
    expect(pick("大火翻炒一两分钟")).toEqual([[60, 120]]);
    expect(pick("炖 20 到 40 分钟")).toEqual([[1200, 2400]]);
    expect(pick("炖20-40分钟")).toEqual([[1200, 2400]]);
    expect(pick("焖三五分钟")).toEqual([[180, 300]]);
  });

  it("“以上”只取前面的数，不是分钟小时秒的不认", () => {
    expect(pick("一年以上的老公鸡要炖 40 分钟以上")).toEqual([[2400, null]]);
    expect(pick("散养 12 到 18 个月")).toEqual([]);
    expect(pick("半年左右的小公鸡")).toEqual([]);
    expect(pick("水量大约没过鸡块的三分之二")).toEqual([]);
    expect(pick("多炖几分钟")).toEqual([]);
  });

  it("一步里有几个时间时按顺序给，去掉重复", () => {
    expect(pick("小公鸡炖 20 分钟。老公鸡炖 40 分钟。再焖 20 分钟。")).toEqual([
      [1200, null],
      [2400, null],
    ]);
  });
});

describe("durationLabel 和 clock", () => {
  it("写成中文时长", () => {
    expect(durationLabel(45)).toBe("45 秒");
    expect(durationLabel(1800)).toBe("30 分钟");
    expect(durationLabel(5400)).toBe("1 小时 30 分钟");
    expect(durationLabel(90)).toBe("1 分钟 30 秒");
  });
  it("倒数", () => {
    expect(clock(1798)).toBe("29:58");
    expect(clock(3900)).toBe("1:05:00");
    expect(clock(-3)).toBe("0:00");
  });
});

function draft(): Draft {
  return {
    ...emptyDraft(),
    title: "临沂炒鸡",
    ingredients: [
      { name: "公鸡", amount: "1500 克" },
      { name: "花椒", amount: "1 汤匙" },
      { name: "八角", amount: "2 颗" },
      { name: "生姜", amount: "30 克" },
    ],
    steps: [
      { text: "煸鸡", ingredients: [0] },
      { text: "炒香料", ingredients: [1, 2, 3] },
    ],
  };
}

describe("食材下标", () => {
  it("删掉食材时步骤里的下标跟着改", () => {
    const d = removeIngredient(draft(), 1);
    expect(d.ingredients.map((i) => i.name)).toEqual(["公鸡", "八角", "生姜"]);
    expect(d.steps.map((s) => s.ingredients)).toEqual([[0], [1, 2]]);
  });

  it("移动食材时步骤里的下标跟着换", () => {
    const d = moveIngredient(draft(), 3, 0);
    expect(d.ingredients.map((i) => i.name)).toEqual([
      "生姜",
      "公鸡",
      "花椒",
      "八角",
    ]);
    expect(d.steps.map((s) => s.ingredients)).toEqual([[1], [2, 3, 0]]);
    expect(moveIngredient(draft(), 0, -1)).toEqual(draft());
  });

  it("保存前去掉空的食材和步骤，下标重排", () => {
    const d = draft();
    d.ingredients.splice(1, 0, { name: "  ", amount: "" });
    d.steps = [
      { text: "煸鸡", ingredients: [0] },
      { text: "  ", ingredients: [] },
      { text: "炒香料", ingredients: [2, 3, 4] },
    ];
    const out = toInput(d);
    expect(out.ingredients?.map((i) => i.name)).toEqual([
      "公鸡",
      "花椒",
      "八角",
      "生姜",
    ]);
    expect(out.steps?.map((s) => s.ingredients)).toEqual([[0], [1, 2, 3]]);
  });

  it("没标食材的步骤按名字在正文里找", () => {
    const ings = draft().ingredients;
    expect(stepIngredients({ text: "放入花椒和生姜" }, ings)).toEqual([1, 3]);
    expect(
      stepIngredients({ text: "放入花椒", ingredients: [2] }, ings),
    ).toEqual([2]);
  });
});

describe("stepTimerChoices", () => {
  it("数据里的计时在前，有上限的给两个，正文里多认出的补在后面", () => {
    const choices = stepTimerChoices({
      title: "加开水炖",
      text: "炖 20 分钟，中途焖 3 分钟",
      timers: [{ label: "炖煮", seconds: 1200, maxSeconds: 2400 }],
    });
    expect(choices).toEqual([
      { label: "炖煮", seconds: 1200 },
      { label: "炖煮", seconds: 2400 },
      { label: "加开水炖", seconds: 180 },
    ]);
  });
});

describe("标签和筛选", () => {
  it("常用的标签放外面，选中的一定在外面", () => {
    const tags = [
      { tag: "辣", count: 5 },
      { tag: "鲁菜", count: 3 },
      { tag: "鸡肉", count: 2 },
      { tag: "清淡", count: 1 },
    ];
    expect(splitTags(tags, "", 2)).toEqual({
      shown: ["辣", "鲁菜"],
      more: ["鸡肉", "清淡"],
    });
    expect(splitTags(tags, "清淡", 2)).toEqual({
      shown: ["辣", "鲁菜", "清淡"],
      more: ["鸡肉"],
    });
  });

  it("加标签时去重，不区分大小写", () => {
    expect(addTag(["辣"], " 辣 ")).toEqual(["辣"]);
    expect(addTag(["BBQ"], "bbq")).toEqual(["BBQ"]);
    expect(addTag(["辣"], "鲁菜")).toEqual(["辣", "鲁菜"]);
    expect(addTag(["辣"], "  ")).toEqual(["辣"]);
  });

  it("按标签和关键字筛选，关键字也找食材名", () => {
    const base = {
      summary: "",
      servings: "",
      totalMinutes: null,
      steps: [],
      tips: "",
      sourceNoteId: null,
      sourceUrl: "",
      nutrition: [],
      cookedCount: 0,
      lastCookedOn: "",
      avgRating: null,
      createdAt: "",
      updatedAt: "",
    };
    const items: Recipe[] = [
      {
        ...base,
        id: 1,
        title: "临沂炒鸡",
        tags: ["辣"],
        ingredients: [{ name: "皱皮辣椒" }],
      },
      {
        ...base,
        id: 2,
        title: "番茄炒蛋",
        tags: ["家常"],
        ingredients: [{ name: "鸡蛋" }],
      },
    ];
    expect(filterRecipes(items, "辣", "").map((r) => r.id)).toEqual([1]);
    expect(filterRecipes(items, "", "鸡蛋").map((r) => r.id)).toEqual([2]);
    expect(filterRecipes(items, "", "").map((r) => r.id)).toEqual([1, 2]);
  });
});
