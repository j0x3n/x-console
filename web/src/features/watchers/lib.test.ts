import { describe, expect, it } from "vitest";
import {
  minutesText,
  money,
  needsUser,
  parseHosts,
  sortWatchers,
  toPage,
} from "./lib";

describe("watchers lib", () => {
  it("formats intervals", () => {
    expect(minutesText(0)).toBe("不保活");
    expect(minutesText(15)).toBe("15 分钟");
    expect(minutesText(120)).toBe("2 小时");
    expect(minutesText(1440)).toBe("1 天");
  });

  it("splits hosts", () => {
    expect(parseHosts(" a.gov.cn, b.cn，c.cn\nd.cn ")).toEqual([
      "a.gov.cn",
      "b.cn",
      "c.cn",
      "d.cn",
    ]);
    expect(parseHosts("")).toEqual([]);
  });

  it("puts watchers that need the user first", () => {
    const list = [
      { name: "b", status: "ok" as const },
      { name: "a", status: "paused" as const },
      { name: "c", status: "error" as const },
      { name: "d", status: "login" as const },
    ];
    expect(sortWatchers(list).map((w) => w.name)).toEqual(["d", "c", "b", "a"]);
    expect(needsUser({ status: "waiting" })).toBe(true);
    expect(needsUser({ status: "ok" })).toBe(false);
  });

  it("maps a click on the picture to the page", () => {
    const rect = { left: 10, top: 20, width: 640, height: 450 };
    const page = { width: 1280, height: 900 };
    expect(toPage(330, 245, rect, page)).toEqual({ x: 640, y: 450 });
    expect(toPage(0, 0, rect, page)).toEqual({ x: 0, y: 0 });
    expect(toPage(9999, 9999, rect, page)).toEqual({ x: 1280, y: 900 });
  });

  it("formats money", () => {
    expect(money(undefined)).toBe("—");
    expect(money(0.0123)).toBe("$0.012");
    expect(money(2.5)).toBe("$2.50");
  });
});
