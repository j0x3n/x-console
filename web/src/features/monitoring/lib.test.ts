import { describe, expect, it } from "vitest";
import {
  addMonths,
  appendLog,
  containerTone,
  emptyLog,
  expiryTone,
  formatMoney,
  formatPorts,
  latencyPoints,
  parseLogFrame,
  appendLogLines,
  mergeStats,
  monitorTone,
  nextRenewal,
  addCycle,
  cycleOf,
  cycleText,
  legacyCycle,
  niceCeil,
  parseRemindDays,
  renewalTone,
  spendView,
  sortSubscriptions,
  stripSegments,
} from "./lib";
import type { DockerContainer, DockerStats, Subscription } from "./api";

describe("tones", () => {
  it("colors monitors by status and failures", () => {
    expect(
      monitorTone({
        enabled: true,
        lastStatus: "down",
        consecutiveFailures: 3,
      }),
    ).toBe("danger");
    expect(
      monitorTone({ enabled: true, lastStatus: "up", consecutiveFailures: 1 }),
    ).toBe("warn");
    expect(
      monitorTone({ enabled: true, lastStatus: "up", consecutiveFailures: 0 }),
    ).toBe("ok");
    expect(
      monitorTone({
        enabled: false,
        lastStatus: "down",
        consecutiveFailures: 3,
      }),
    ).toBe("");
    expect(
      monitorTone({
        enabled: true,
        lastStatus: "unknown",
        consecutiveFailures: 0,
      }),
    ).toBe("");
  });

  it("uses different windows for certificates and domains", () => {
    expect(expiryTone("tls", 20)).toBe("ok");
    expect(expiryTone("tls", 10)).toBe("warn");
    expect(expiryTone("tls", 2)).toBe("danger");
    expect(expiryTone("domain", 20)).toBe("warn");
    expect(expiryTone("domain", 40)).toBe("ok");
    expect(expiryTone("domain", -1)).toBe("danger");
    expect(expiryTone("tls", undefined)).toBe("");
    expect(renewalTone(-1)).toBe("danger");
    expect(renewalTone(3)).toBe("danger");
    expect(renewalTone(4)).toBe("warn");
    expect(renewalTone(7)).toBe("warn");
    expect(renewalTone(8)).toBe("");
    expect(renewalTone(30)).toBe("");
    expect(containerTone("running")).toBe("ok");
    expect(containerTone("restarting")).toBe("warn");
    expect(containerTone("exited")).toBe("");
  });
});

describe("money and dates", () => {
  it("formats amounts", () => {
    expect(formatMoney(10, "CNY")).toBe("¥10");
    expect(formatMoney(9.9, "USD")).toBe("$9.9");
    expect(formatMoney(12.345, "EUR")).toBe("€12.35");
    expect(formatMoney(5, "BTC")).toBe("5 BTC");
  });

  it("advances renewal dates like the server", () => {
    expect(addMonths("2026-01-31", 1)).toBe("2026-02-28");
    expect(addMonths("2028-01-31", 1)).toBe("2028-02-29");
    expect(addMonths("2026-12-15", 1)).toBe("2027-01-15");
    expect(nextRenewal("2028-02-29", "yearly", 0)).toBe("2029-02-28");
    expect(nextRenewal("2026-10-01", "custom_days", 90)).toBe("2026-12-30");
    expect(nextRenewal("2026-10-01", "monthly", 0)).toBe("2026-11-01");
  });

  it("parses reminder days", () => {
    expect(parseRemindDays("7, 1")).toEqual([7, 1]);
    expect(parseRemindDays("1 30，7 7")).toEqual([30, 7, 1]);
    expect(parseRemindDays("")).toEqual([]);
    expect(parseRemindDays("a")).toBeNull();
    expect(parseRemindDays("400")).toBeNull();
  });

  it("sorts subscriptions by days left", () => {
    const sub = (name: string, daysLeft: number) =>
      ({ name, daysLeft }) as Subscription;
    expect(
      sortSubscriptions([sub("b", 5), sub("a", -2), sub("c", 5)]).map(
        (s) => s.name,
      ),
    ).toEqual(["a", "b", "c"]);
  });
});

describe("chart", () => {
  it("rounds the axis", () => {
    expect(niceCeil(7)).toBe(10);
    expect(niceCeil(130)).toBe(200);
    expect(niceCeil(450)).toBe(500);
    expect(niceCeil(1)).toBe(1);
  });

  it("fills the status strip without gaps", () => {
    expect(stripSegments([0, 600], 600)).toEqual([
      [0, 300],
      [300, 300],
    ]);
    expect(stripSegments([300], 600)).toEqual([[0, 600]]);
  });

  it("maps results to coordinates", () => {
    const { points, max } = latencyPoints(
      [
        { at: "2026-09-01T00:00:00Z", latencyMs: 50, ok: true },
        { at: "2026-09-01T00:01:00Z", latencyMs: 100, ok: false },
      ],
      200,
      100,
    );
    expect(max).toBe(100);
    expect(points).toEqual([
      { x: 0, y: 50, ok: true },
      { x: 200, y: 0, ok: false },
    ]);
    expect(latencyPoints([], 100, 100).points).toEqual([]);
  });
});

describe("docker", () => {
  const container = (
    id: string,
    name: string,
    state: string,
  ): DockerContainer => ({
    id,
    name,
    state,
    image: "img",
    status: "",
    created: "2026-01-01T00:00:00Z",
    ports: [],
  });

  it("merges stats and puts running containers first", () => {
    const stats = [{ id: "b", cpuPercent: 5 } as DockerStats];
    const rows = mergeStats(
      [container("a", "zeta", "exited"), container("b", "beta", "running")],
      stats,
    );
    expect(rows.map((r) => r.name)).toEqual(["beta", "zeta"]);
    expect(rows[0].stats?.cpuPercent).toBe(5);
    expect(rows[1].stats).toBeUndefined();
  });

  it("formats ports without duplicates", () => {
    expect(
      formatPorts([
        { privatePort: 80, publicPort: 8080, type: "tcp", ip: "0.0.0.0" },
        { privatePort: 80, publicPort: 8080, type: "tcp", ip: "::" },
        { privatePort: 443, type: "tcp" },
      ]),
    ).toBe("8080→80/tcp, 443/tcp");
  });

  it("buffers log lines", () => {
    const logText = (b: typeof emptyLog) =>
      [...b.lines.map((l) => l.text), ...(b.partial ? [b.partial] : [])].join(
        "\n",
      );
    let buf = appendLog(emptyLog, "one\ntw");
    expect(buf.lines.map((l) => l.text)).toEqual(["one"]);
    expect(buf.partial).toBe("tw");
    buf = appendLog(buf, "o\r\nthree\n");
    expect(buf.lines.map((l) => l.text)).toEqual(["one", "two", "three"]);
    expect(buf.partial).toBe("");
    expect(logText(appendLog(buf, "four"))).toBe("one\ntwo\nthree\nfour");
    expect(appendLog(buf, "x\ny\n", 2).lines.map((l) => l.text)).toEqual([
      "x",
      "y",
    ]);
    const json = parseLogFrame(
      '[{"stream":"stderr","text":"boom"},{"stream":"stdout","text":"ok"}]',
    );
    expect(json?.length).toBe(2);
    expect(parseLogFrame("[2026] plain text")).toBeNull();
    const mixed = appendLogLines(buf, json!);
    expect(mixed.lines.at(-2)).toMatchObject({
      text: "boom",
      stream: "stderr",
    });
  });

  it("reads and converts subscription cycles (B23)", () => {
    const base = { cycle: "monthly" as const, cycleDays: 0 };
    expect(cycleOf(base)).toEqual({ count: 1, unit: "month" });
    expect(cycleOf({ ...base, cycle: "custom_days", cycleDays: 14 })).toEqual({
      count: 2,
      unit: "week",
    });
    expect(cycleOf({ ...base, cycle: "custom_days", cycleDays: 10 })).toEqual({
      count: 10,
      unit: "day",
    });
    expect(cycleOf({ ...base, cycleCount: 3, cycleUnit: "month" })).toEqual({
      count: 3,
      unit: "month",
    });
    expect(legacyCycle({ count: 1, unit: "year" })).toEqual({
      cycle: "yearly",
    });
    expect(legacyCycle({ count: 2, unit: "week" })).toEqual({
      cycle: "custom_days",
      cycleDays: 14,
    });
    expect(legacyCycle({ count: 3, unit: "month" })).toBeNull();
    expect(legacyCycle({ count: 5, unit: "minute" })).toBeNull();
  });

  it("moves the renewal date by a cycle (B23)", () => {
    expect(addCycle("2026-01-31", { count: 1, unit: "month" })).toBe(
      "2026-02-28",
    );
    expect(addCycle("2026-01-31", { count: 3, unit: "month" })).toBe(
      "2026-04-30",
    );
    expect(addCycle("2028-02-29", { count: 2, unit: "year" })).toBe(
      "2030-02-28",
    );
    expect(addCycle("2026-10-01", { count: 2, unit: "week" })).toBe(
      "2026-10-15",
    );
    // 分钟和小时至少推一天
    expect(addCycle("2026-10-01", { count: 30, unit: "minute" })).toBe(
      "2026-10-02",
    );
    expect(addCycle("2026-10-01", { count: 49, unit: "hour" })).toBe(
      "2026-10-04",
    );
  });

  it("writes the cycle in words (B23)", () => {
    const t = (key: string) => key;
    expect(cycleText({ count: 1, unit: "month" }, t)).toBe("Every month");
    expect(cycleText({ count: 3, unit: "month" }, t)).toBe("Every 3 Month(s)");
  });
});

describe("spendView", () => {
  const totals = [
    { currency: "CNY", monthly: 100, yearly: 1200, count: 2 },
    { currency: "USD", monthly: 10, yearly: 120, count: 1 },
  ];
  it("有换算结果时用换算结果", () => {
    const v = spendView(
      {
        totals,
        byCategory: [],
        converted: [
          { currency: "CNY", monthly: 172, yearly: 2064, count: 3 },
          { currency: "USD", monthly: 24, yearly: 288, count: 3 },
        ],
        unconverted: [],
      },
      "USD",
    );
    expect(v).toEqual({
      currency: "USD",
      monthly: 24,
      converted: true,
      missing: [],
    });
  });
  it("没有换算结果时退回同币种合计", () => {
    const v = spendView({ totals, byCategory: [] }, "USD");
    expect(v).toEqual({
      currency: "USD",
      monthly: 10,
      converted: false,
      missing: ["CNY"],
    });
  });
  it("没有这个币种时用第一个", () => {
    const v = spendView({ totals: [totals[1]], byCategory: [] }, "CNY");
    expect(v?.currency).toBe("USD");
  });
  it("没有订阅时为空", () => {
    expect(spendView({ totals: [], byCategory: [] }, "CNY")).toBeNull();
  });
});
