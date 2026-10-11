import { describe, expect, it } from "vitest";
import type { RoiGroup, RoiTask } from "./api";
import {
  breakdown,
  groupCell,
  minutesText,
  moneyText,
  nextSort,
  percentText,
  ratioText,
  secondsText,
  sortRows,
  taskCell,
  tokensText,
} from "./format";

const t = (key: string) => ({ min: "分钟", hr: "小时", sec: "秒" })[key] ?? key;

describe("number text", () => {
  it("keeps no record apart from zero", () => {
    expect(moneyText(null)).toBeNull();
    expect(moneyText(undefined)).toBeNull();
    expect(moneyText(0)).toBe("$0.00");
    expect(minutesText(t, null)).toBeNull();
    expect(minutesText(t, 0)).toBe("0 分钟");
    expect(secondsText(t, null)).toBeNull();
    expect(tokensText(null)).toBeNull();
    expect(percentText(null)).toBeNull();
    expect(ratioText(null)).toBeNull();
  });

  it("formats money by size", () => {
    expect(moneyText(0.5)).toBe("$0.50");
    expect(moneyText(0.0123)).toBe("$0.0123");
    expect(moneyText(12.345)).toBe("$12.35");
  });

  it("formats time", () => {
    expect(secondsText(t, 45)).toBe("45 秒");
    expect(secondsText(t, 1200)).toBe("20 分钟");
    expect(secondsText(t, 3900)).toBe("1 小时 5 分钟");
    expect(minutesText(t, 120)).toBe("2 小时");
    expect(minutesText(t, 34.6)).toBe("35 分钟");
  });

  it("formats tokens, percent and ratio", () => {
    expect(tokensText(950)).toBe("950");
    expect(tokensText(1500)).toBe("1.5k");
    expect(tokensText(120_000)).toBe("120k");
    expect(tokensText(2_500_000)).toBe("2.5M");
    expect(percentText(0.5)).toBe("50%");
    expect(ratioText(1.5)).toBe("1.5×");
    expect(ratioText(12)).toBe("12×");
  });
});

describe("sorting", () => {
  const rows = [
    { name: "b", n: 2 as number | null },
    { name: "a", n: null },
    { name: "c", n: 9 },
    { name: "d", n: 5 },
  ];
  const value = (r: (typeof rows)[number], key: string) =>
    key === "n" ? r.n : r.name;

  it("puts rows with no record last in both directions", () => {
    expect(
      sortRows(rows, { key: "n", desc: true }, value).map((r) => r.name),
    ).toEqual(["c", "d", "b", "a"]);
    expect(
      sortRows(rows, { key: "n", desc: false }, value).map((r) => r.name),
    ).toEqual(["b", "d", "c", "a"]);
  });

  it("sorts text and keeps ties in order", () => {
    expect(
      sortRows(rows, { key: "name", desc: false }, value).map((r) => r.name),
    ).toEqual(["a", "b", "c", "d"]);
    const ties = [
      { name: "x", n: 1 },
      { name: "y", n: 1 },
    ];
    expect(
      sortRows(ties, { key: "n", desc: true }, value).map((r) => r.name),
    ).toEqual(["x", "y"]);
  });

  it("flips the same column and starts a new one descending", () => {
    expect(nextSort({ key: "cost", desc: true }, "cost")).toEqual({
      key: "cost",
      desc: false,
    });
    expect(nextSort({ key: "cost", desc: false }, "run")).toEqual({
      key: "run",
      desc: true,
    });
  });
});

describe("cells", () => {
  const group = {
    key: "x",
    label: "x-console",
    tasks: 3,
    merged: 1,
    reverted: 1,
    canceled: 0,
    failed: 2,
    waiting: 1,
    active: 0,
    costMissing: 0,
    tokenMissing: 0,
    runMissing: 0,
    reviewMissing: 0,
    rounds: 1,
    cost: null,
  } as RoiGroup;

  it("reads group columns and leaves a missing value null", () => {
    expect(groupCell(group, "tasks")).toBe(3);
    expect(groupCell(group, "cost")).toBeNull();
    expect(groupCell(group, "nothing")).toBeNull();
  });

  it("lists only what is not zero under a group", () => {
    expect(breakdown(t, group)).toBe("1 Waiting to merge · 2 Failed");
  });

  it("adds input and output tokens for a task", () => {
    const task = { inputTokens: 100, outputTokens: 20 } as RoiTask;
    expect(taskCell(task, "tokens")).toBe(120);
    expect(taskCell({} as RoiTask, "tokens")).toBeNull();
  });
});
