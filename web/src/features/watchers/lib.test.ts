import { describe, expect, it } from "vitest";
import {
  groupRuns,
  guessKind,
  minutesText,
  money,
  needsUser,
  parseHosts,
  parseModelRef,
  runShots,
  sortWatchers,
  toPage,
  xLabel,
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

  it("names X targets", () => {
    expect(xLabel("https://x.com/alice")).toBe("@alice");
    expect(
      xLabel(
        "https://x.com/search?f=live&q=%E7%BE%8E%E8%81%94%E5%82%A8+%E9%99%8D%E6%81%AF",
      ),
    ).toBe("搜索：美联储 降息");
    expect(xLabel("https://x.com/i/status/123")).toBe("推文 123 的回复");
  });

  it("formats money", () => {
    expect(money(undefined)).toBe("—");
    expect(money(0.0123)).toBe("$0.012");
    expect(money(2.5)).toBe("$2.50");
  });

  it("guesses the kind from the address (B213)", () => {
    expect(guessKind("")).toBeNull();
    expect(guessKind("@alice")).toEqual({ kind: "x", label: "X 账号" });
    expect(guessKind("https://x.com/alice")?.label).toBe("X 账号");
    expect(guessKind("twitter.com/alice")?.kind).toBe("x");
    expect(guessKind("https://mobile.twitter.com/a/status/12")?.label).toBe(
      "X 推文的回复",
    );
    expect(guessKind("x.com/search?q=fed")?.label).toBe("X 搜索");
    expect(guessKind("降息")).toEqual({ kind: "x", label: "X 搜索" });
    expect(guessKind(" https://www.jszwfw.gov.cn/ ")).toEqual({
      kind: "web",
      label: "网页",
    });
    // 没写 https:// 的网址认不出来，要用户补上
    expect(guessKind("example.com")).toBeNull();
    expect(guessKind("a/b")).toBeNull();
  });

  it("folds runs that end the same way (B234)", () => {
    const run = (id: number, outcome: string, detail = "提醒满了") => ({
      id,
      kind: "explore",
      outcome,
      detail,
    });
    const groups = groupRuns([
      run(9, "running", ""),
      run(8, "failed"),
      run(7, "failed"),
      run(6, "failed"),
      run(5, "ok", "找到了"),
      run(4, "failed"),
    ]);
    expect(groups.map((g) => [g.latest.id, g.count])).toEqual([
      [9, 1],
      [8, 3],
      [5, 1],
      [4, 1],
    ]);
    // 说明不同的失败不合并
    expect(
      groupRuns([run(2, "failed", "a"), run(1, "failed", "b")]),
    ).toHaveLength(2);
  });

  it("lists the screenshots of a run", () => {
    expect(
      runShots([
        { text: "打开", shotId: 3 },
        { text: "点击" },
        { text: "读到", shotId: 5 },
      ]),
    ).toEqual([
      { id: 3, text: "打开" },
      { id: 5, text: "读到" },
    ]);
  });

  it("parses a model choice", () => {
    expect(parseModelRef("3:gpt-4o:mini")).toEqual({
      providerId: 3,
      model: "gpt-4o:mini",
    });
    expect(parseModelRef("")).toBeUndefined();
    expect(parseModelRef("x:gpt")).toBeUndefined();
    expect(parseModelRef("3:")).toBeUndefined();
  });
});
