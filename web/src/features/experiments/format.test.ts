import { describe, expect, it } from "vitest";
import { CHART_H, CHART_W, fmt, scatter, signed, verdictText } from "./format";

const metric = {
  key: "body:weight",
  label: "体重",
  unit: "kg",
  subjective: false,
};

describe("数字显示", () => {
  it("最多两位小数，没有值显示破折号", () => {
    expect(fmt(1.2345)).toBe("1.23");
    expect(fmt(2)).toBe("2");
    expect(fmt(undefined)).toBe("—");
    expect(fmt(NaN)).toBe("—");
    expect(signed(1.5)).toBe("+1.5");
    expect(signed(-1.5)).toBe("-1.5");
    expect(signed(0)).toBe("0");
  });
});

describe("结论一句话", () => {
  it("样本太少先说不能下结论", () => {
    expect(verdictText("too_few", { metric })).toMatch(/不能下结论/);
  });
  it("区间含 0 说在随机波动内", () => {
    expect(
      verdictText("inconclusive", {
        metric,
        diff: 0.2,
        ciLow: -1,
        ciHigh: 1.4,
      }),
    ).toMatch(/随机波动/);
  });
  it("有差异时带单位和区间", () => {
    const text = verdictText("lower", {
      metric,
      diff: -1.5,
      ciLow: -2,
      ciHigh: -1,
    });
    expect(text).toContain("平均低 1.5 kg");
    expect(text).toContain("-2 到 -1");
  });
});

describe("散点坐标", () => {
  const points = [
    { date: "2026-11-01", value: 10, phase: "baseline" as const },
    { date: "2026-11-05", value: 20, phase: "change" as const },
  ];
  it("头尾在边距上，最小值在底部，最大值在顶部", () => {
    const s = scatter(points, "2026-11-01", "2026-11-05");
    expect(s.dots[0].cx).toBe(10);
    expect(s.dots[1].cx).toBe(CHART_W - 10);
    expect(s.dots[0].cy).toBe(CHART_H - 10);
    expect(s.dots[1].cy).toBe(10);
  });
  it("所有值相同时不除以零", () => {
    const s = scatter(
      [{ ...points[0] }, { ...points[1], value: 10 }],
      "2026-11-01",
      "2026-11-05",
    );
    expect(Number.isFinite(s.dots[0].cy)).toBe(true);
  });
  it("没有点时也不出错", () => {
    expect(scatter([], "2026-11-01", "2026-11-05").dots).toEqual([]);
  });
});
