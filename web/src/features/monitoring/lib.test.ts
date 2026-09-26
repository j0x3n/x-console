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
  logText,
  mergeStats,
  monitorTone,
  nextRenewal,
  niceCeil,
  parseRemindDays,
  renewalTone,
  sortSubscriptions,
  stripSegments,
} from "./lib";
import type { DockerContainer, DockerStats, Subscription } from "./api";

describe("tones", () => {
  it("colors monitors by status and failures", () => {
    expect(monitorTone({ enabled: true, lastStatus: "down", consecutiveFailures: 3 })).toBe("danger");
    expect(monitorTone({ enabled: true, lastStatus: "up", consecutiveFailures: 1 })).toBe("warn");
    expect(monitorTone({ enabled: true, lastStatus: "up", consecutiveFailures: 0 })).toBe("ok");
    expect(monitorTone({ enabled: false, lastStatus: "down", consecutiveFailures: 3 })).toBe("");
    expect(monitorTone({ enabled: true, lastStatus: "unknown", consecutiveFailures: 0 })).toBe("");
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
    expect(renewalTone(3)).toBe("warn");
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
    const sub = (name: string, daysLeft: number) => ({ name, daysLeft }) as Subscription;
    expect(sortSubscriptions([sub("b", 5), sub("a", -2), sub("c", 5)]).map((s) => s.name)).toEqual(["a", "b", "c"]);
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
  const container = (id: string, name: string, state: string): DockerContainer => ({
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
    const rows = mergeStats([container("a", "zeta", "exited"), container("b", "beta", "running")], stats);
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
    let buf = appendLog(emptyLog, "one\ntw");
    expect(buf).toEqual({ lines: ["one"], partial: "tw" });
    buf = appendLog(buf, "o\r\nthree\n");
    expect(buf).toEqual({ lines: ["one", "two", "three"], partial: "" });
    expect(logText(appendLog(buf, "four"))).toBe("one\ntwo\nthree\nfour");
    expect(appendLog(buf, "x\ny\n", 2).lines).toEqual(["x", "y"]);
  });
});
