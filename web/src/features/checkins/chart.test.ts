import { describe, expect, it } from "vitest";
import { CHART_H, CHART_W, fmtScore, linePoints, pathOf } from "./chart";

describe("折线坐标", () => {
  const pts = [
    { date: "2026-11-01", average: 1, count: 1 },
    { date: "2026-11-03", average: 3, count: 2 },
    { date: "2026-11-05", average: 5, count: 1 },
  ];
  it("头尾落在边距上，中间按天数和分数摆", () => {
    const out = linePoints(pts, "2026-11-01", "2026-11-05");
    expect(out[0].x).toBe(8);
    expect(out[2].x).toBe(CHART_W - 8);
    expect(out[1].x).toBeCloseTo(CHART_W / 2);
    expect(out[0].y).toBe(CHART_H - 8);
    expect(out[2].y).toBe(8);
    expect(out[1].y).toBeCloseTo(CHART_H / 2);
  });
  it("只有一天时不除以零", () => {
    const out = linePoints([pts[0]], "2026-11-01", "2026-11-01");
    expect(Number.isFinite(out[0].x)).toBe(true);
  });
  it("拼路径", () => {
    expect(
      pathOf([
        { x: 1, y: 2 },
        { x: 3, y: 4 },
      ]),
    ).toBe("M1.0 2.0 L3.0 4.0");
    expect(pathOf([])).toBe("");
  });
  it("没有回答时平均分显示破折号", () => {
    expect(fmtScore(0)).toBe("—");
    expect(fmtScore(3.25)).toBe("3.3");
  });
});
