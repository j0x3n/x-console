import { describe, expect, it } from "vitest";
import type { QuotaAccount, QuotaWindow } from "./api";
import {
  durationText,
  groupByKind,
  isStale,
  moveWithinKind,
  nextReset,
  resetText,
  tightestWindow,
  toneOf,
  viewWindow,
} from "./format";

const now = new Date("2026-10-06T12:00:00Z");
const at = (minutes: number) =>
  new Date(now.getTime() + minutes * 60_000).toISOString();

function win(over: Partial<QuotaWindow> = {}): QuotaWindow {
  return { name: "5 小时", usedPercent: 40, ...over };
}

function account(over: Partial<QuotaAccount> = {}): QuotaAccount {
  return {
    id: 1,
    kind: "codex",
    name: "A",
    hostId: "h1",
    home: "",
    keySet: false,
    status: "ok",
    windows: [],
    balances: [],
    createdAt: "2026-10-01T00:00:00Z",
    ...over,
  };
}

describe("quota display", () => {
  it("colours by what is left", () => {
    expect(toneOf(80)).toBe("ok");
    expect(toneOf(20)).toBe("ok");
    expect(toneOf(19)).toBe("warn");
    expect(toneOf(5)).toBe("warn");
    expect(toneOf(4)).toBe("danger");
    expect(toneOf(0)).toBe("danger");
  });

  it("counts a window whose reset time has passed as unused", () => {
    const v = viewWindow(win({ usedPercent: 97, resetsAt: at(-1) }), now);
    expect(v).toMatchObject({ used: 0, remaining: 100, reset: true });
    const live = viewWindow(win({ usedPercent: 97, resetsAt: at(30) }), now);
    expect(live).toMatchObject({
      used: 97,
      remaining: 3,
      reset: false,
      tone: "danger",
    });
    expect(viewWindow(win({ usedPercent: 140 }), now).remaining).toBe(0);
    expect(viewWindow(win({ usedPercent: -3 }), now).remaining).toBe(100);
  });

  it("finds the tightest window and skips extra ones", () => {
    const a = account({
      windows: [
        win({ name: "5 小时", usedPercent: 10 }),
        win({ name: "7 天", usedPercent: 85 }),
        win({ name: "按量付费", usedPercent: 99, aside: true }),
      ],
    });
    expect(tightestWindow(a, now)?.window.name).toBe("7 天");
    expect(tightestWindow(account(), now)).toBeNull();
    // a window that has reset no longer counts as tight
    const b = account({
      windows: [
        win({ name: "7 天", usedPercent: 99, resetsAt: at(-5) }),
        win({ name: "5 小时", usedPercent: 50 }),
      ],
    });
    expect(tightestWindow(b, now)?.window.name).toBe("5 小时");
  });

  it("finds the next reset in the future", () => {
    const accounts = [
      account({ id: 1, windows: [win({ resetsAt: at(300) })] }),
      account({
        id: 2,
        name: "B",
        windows: [
          win({ name: "7 天", resetsAt: at(90) }),
          win({ resetsAt: at(-10) }),
        ],
      }),
      account({ id: 3, windows: [win({ resetsAt: at(10), aside: true })] }),
    ];
    const next = nextReset(accounts, now);
    expect(next?.account.id).toBe(2);
    expect(next?.window.name).toBe("7 天");
    expect(nextReset([account()], now)).toBeNull();
  });

  it("writes durations in Chinese and English", () => {
    const min = 60_000;
    expect(durationText(5 * min, "zh")).toBe("5 分");
    expect(durationText(134 * min, "zh")).toBe("2 小时 14 分");
    expect(durationText(120 * min, "zh")).toBe("2 小时");
    expect(durationText((3 * 1440 + 4 * 60) * min, "zh")).toBe("3 天 4 小时");
    expect(durationText(2 * 1440 * min, "zh")).toBe("2 天");
    expect(durationText(134 * min, "en")).toBe("2h 14m");
    expect(durationText(0, "zh")).toBe("1 分");
  });

  it("says when a window resets", () => {
    expect(resetText(win({ resetsAt: at(134) }), now, "zh")).toBe(
      "2 小时 14 分后重置",
    );
    expect(resetText(win({ resetsAt: at(134) }), now, "en")).toBe(
      "Resets in 2h 14m",
    );
    expect(resetText(win({ resetsAt: at(-1) }), now, "zh")).toContain("已重置");
    expect(resetText(win(), now, "zh")).toBe("");
  });

  it("knows an account that shows old numbers", () => {
    expect(isStale(account({ status: "error", windows: [win()] }))).toBe(true);
    expect(isStale(account({ status: "error" }))).toBe(false);
    expect(isStale(account({ status: "ok", windows: [win()] }))).toBe(false);
  });

  it("groups by service in a fixed order", () => {
    const groups = groupByKind([
      account({ id: 1, kind: "deepseek" }),
      account({ id: 2, kind: "claude" }),
      account({ id: 3, kind: "deepseek" }),
    ]);
    expect(groups.map((g) => [g.kind, g.items.length])).toEqual([
      ["claude", 1],
      ["deepseek", 2],
    ]);
  });

  it("moves an account within its own service only", () => {
    const list = [
      account({ id: 1, kind: "codex" }),
      account({ id: 2, kind: "claude" }),
      account({ id: 3, kind: "codex" }),
      account({ id: 4, kind: "codex" }),
    ];
    expect(moveWithinKind(list, 3, -1)).toEqual([3, 2, 1, 4]);
    expect(moveWithinKind(list, 3, 1)).toEqual([1, 2, 4, 3]);
    expect(moveWithinKind(list, 1, -1)).toBeNull();
    expect(moveWithinKind(list, 4, 1)).toBeNull();
    expect(moveWithinKind(list, 2, 1)).toBeNull();
    expect(moveWithinKind(list, 99, 1)).toBeNull();
  });
});
