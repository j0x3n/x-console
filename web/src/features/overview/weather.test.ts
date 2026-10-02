import { describe, expect, it } from "vitest";
import {
  airTone,
  compareYesterday,
  rainSoon,
  rainSoonText,
  sortWarnings,
  warningLabel,
  warningTone,
} from "./weather";

const warn = (
  id: string,
  level: "red" | "yellow" | "blue",
  issuedAt: string,
) => ({
  id,
  title: "",
  typeName: "暴雨",
  level,
  sender: "",
  issuedAt,
  text: "",
});

describe("天气扩展（B58）", () => {
  it("预警名字和颜色", () => {
    expect(warningLabel({ typeName: "冰雹", level: "orange" })).toBe(
      "冰雹橙色预警",
    );
    expect(warningTone("red")).toBe("danger");
    expect(warningTone("unknown")).toBe("");
  });
  it("预警按颜色排序", () => {
    const list = sortWarnings([
      warn("a", "blue", "2026-10-01T08:00:00Z"),
      warn("b", "red", "2026-10-01T07:00:00Z"),
      warn("c", "yellow", "2026-10-01T09:00:00Z"),
    ]);
    expect(list.map((w) => w.id)).toEqual(["b", "c", "a"]);
  });
  it("分钟降水", () => {
    const now = new Date("2026-10-01T08:00:00Z");
    const points = [
      { time: "2026-10-01T08:00:00Z", precip: 0 },
      { time: "2026-10-01T08:20:00Z", precip: 0.3, kind: "snow" as const },
    ];
    const r = rainSoon({ summary: "", points }, now);
    expect(r).toEqual({ kind: "snow", inMinutes: 20 });
    expect(rainSoonText(r!)).toBe("20 分钟后有雪");
    expect(rainSoonText({ kind: "rain", inMinutes: 0 })).toBe("正在下雨");
    expect(rainSoon({ summary: "", points: [points[0]] }, now)).toBeNull();
    expect(rainSoon(undefined, now)).toBeNull();
  });
  it("空气和昨天", () => {
    expect(airTone(2)).toBe("ok");
    expect(airTone(5)).toBe("danger");
    expect(compareYesterday({ high: 25 }, { high: 22, low: 15 })).toBe(
      "比昨天高 3°",
    );
    expect(compareYesterday({ high: 20 }, { high: 22.4, low: 15 })).toBe(
      "比昨天低 2°",
    );
    expect(compareYesterday({ high: 20 }, undefined)).toBe("");
  });
});

describe("B90 rain now", () => {
  const now = new Date("2026-10-02T06:00:00Z");
  const at = (min: number) =>
    new Date(now.getTime() + min * 60_000).toISOString();
  it("says how long the rain lasts", () => {
    const points = [0, 5, 10, 15, 20, 25, 30].map((m, i) => ({
      time: at(m),
      precip: i < 5 ? 0.2 : 0,
    }));
    const r = rainSoon({ summary: "", points }, now)!;
    expect(r.endsIn).toBe(25);
    expect(rainSoonText(r)).toBe("正在下雨，约 25 分钟后停");
  });
  it("says it will not stop within two hours", () => {
    const points = [0, 5, 10].map((m) => ({ time: at(m), precip: 1 }));
    expect(rainSoonText(rainSoon({ summary: "", points }, now)!)).toBe(
      "正在下雨，两小时内不会停",
    );
  });
});
