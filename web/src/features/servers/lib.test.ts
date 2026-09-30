import { describe, expect, it } from "vitest";
import type { Host, MetricsSample, MetricsSeries } from "./api";
import {
  appendPoint,
  breadcrumbs,
  formatRate,
  formatUptime,
  fullestDisk,
  joinPath,
  niceMax,
  niceRateMax,
  percent,
  pickDesktop,
  pointFromSample,
  splitSegments,
  usageTone,
  withSample,
  cumulativeUsed,
  daysLeft,
  trafficTitle,
  trafficUsed,
} from "./lib";
import { tabsFor } from "./tabs";
import { canViewAsText } from "./lib";
import { filterProcesses } from "./components/ProcessesTab";
import { filterServices } from "./components/ServicesTab";
import { describeRule } from "./components/AlertRules";

const sample = (at: string, cpu = 10): MetricsSample => ({
  at,
  cpu,
  cpuPerCore: [cpu],
  memUsed: 25,
  memTotal: 100,
  swapUsed: 0,
  swapTotal: 0,
  disks: [
    { mount: "/", fsType: "ext4", used: 50, total: 100 },
    { mount: "/data", fsType: "ext4", used: 93, total: 100 },
  ],
  netRx: 1000,
  netTx: 10,
  diskRead: 0,
  diskWrite: 0,
  load1: 0.5,
  load5: 0.4,
  load15: 0.3,
  uptimeSeconds: 90000,
  procs: 100,
});

const host = (id: string, online = false): Host => ({
  id,
  name: id,
  kind: "server",
  source: "agent",
  online,
  os: "linux",
  hostname: id,
  activeAlerts: 0,
});

describe("numbers", () => {
  it("computes percentages and tones", () => {
    expect(percent(1, 3)).toBe(33.3);
    expect(percent(1, 0)).toBe(0);
    expect(fullestDisk(sample("2026-01-01T00:00:00Z"))).toBe(93);
    expect(usageTone(50)).toBe("ok");
    expect(usageTone(85)).toBe("warn");
    expect(usageTone(95)).toBe("danger");
  });
  it("formats rates and uptime", () => {
    expect(formatRate(512)).toBe("512 B/s");
    expect(formatRate(1536)).toBe("1.5 KB/s");
    expect(formatUptime(90061)).toBe("1 天 1 小时");
    expect(formatUptime(3700, false)).toBe("1h 1m");
  });
  it("rounds axis maxima", () => {
    expect(niceMax(0)).toBe(1);
    expect(niceMax(7)).toBe(10);
    expect(niceMax(1800)).toBe(2000);
    expect(niceMax(2300)).toBe(2500);
    expect(niceRateMax(80_000)).toBe(100 * 1024);
    expect(niceRateMax(500)).toBe(500);
    expect(niceRateMax(3 * 1024 * 1024)).toBe(5 * 1024 * 1024);
  });
});

describe("live metrics", () => {
  it("merges a sample into a host", () => {
    const h = withSample(host("a"), sample("2026-01-01T00:00:00Z", 42));
    expect(h.online).toBe(true);
    expect(h.cpu).toBe(42);
    expect(h.memory).toBe(25);
    expect(h.disk).toBe(93);
  });
  it("appends to the 1h series and drops old points", () => {
    const old = pointFromSample(sample("2026-01-01T00:00:00Z"));
    const series: MetricsSeries = {
      range: "1h",
      stepSeconds: 10,
      points: [old],
    };
    const next = appendPoint(
      series,
      pointFromSample(sample("2026-01-01T01:00:10Z", 50)),
    )!;
    expect(next.points).toHaveLength(1);
    expect(next.points[0].cpu).toBe(50);
    const kept = appendPoint(
      series,
      pointFromSample(sample("2026-01-01T00:00:10Z")),
    )!;
    expect(kept.points).toHaveLength(2);
    const day: MetricsSeries = { range: "24h", stepSeconds: 300, points: [] };
    expect(appendPoint(day, old)).toBe(day);
  });
  it("splits lines at gaps", () => {
    const pts = [
      "00:00:00",
      "00:00:10",
      "00:00:20",
      "00:05:00",
      "00:05:10",
    ].map((t) => ({ at: `2026-01-01T${t}Z` }));
    expect(splitSegments(pts, 10).map((s) => s.length)).toEqual([3, 2]);
  });
});

describe("files", () => {
  it("joins paths", () => {
    expect(joinPath("/home", "a", "/")).toBe("/home/a");
    expect(joinPath("/", "etc", "/")).toBe("/etc");
    expect(joinPath("C:\\", "Users", "\\")).toBe("C:\\Users");
  });
  it("builds breadcrumbs", () => {
    expect(breadcrumbs("/home/jo", "/")).toEqual([
      { label: "/", path: "/" },
      { label: "home", path: "/home" },
      { label: "jo", path: "/home/jo" },
    ]);
    expect(breadcrumbs("C:\\Users\\me", "\\")).toEqual([
      { label: "C:", path: "C:\\" },
      { label: "Users", path: "C:\\Users" },
      { label: "me", path: "C:\\Users\\me" },
    ]);
  });
});

describe("tabs and filters", () => {
  it("shows only tabs the host supports", () => {
    expect(tabsFor({ capabilities: ["pty"] }).map((t) => t.id)).toEqual([
      "overview",
      "terminal",
      "alerts",
      "agent",
    ]);
  });
  it("offers system logs and Agent on a paired desktop", () => {
    const tabs = tabsFor({
      capabilities: ["syslog", "files"],
      source: "agent",
    });
    expect(tabs.map((tab) => tab.id)).toContain("logs");
    expect(tabs.map((tab) => tab.id)).toContain("agent");
    expect(tabs.map((tab) => tab.id)).toContain("files");
  });
  it("picks the online desktop", () => {
    const hosts = [host("a"), host("b", true)];
    expect(pickDesktop(hosts)?.id).toBe("b");
    expect(pickDesktop(hosts, "a")?.id).toBe("a");
    expect(pickDesktop([])).toBeUndefined();
  });
  it("filters processes and services", () => {
    const procs = [
      {
        pid: 1,
        ppid: 0,
        name: "systemd",
        user: "root",
        cpu: 0,
        memRss: 0,
        memPercent: 0,
        cmdline: "/sbin/init",
        startedAt: "",
        status: "",
      },
      {
        pid: 42,
        ppid: 1,
        name: "nginx",
        user: "www",
        cpu: 0,
        memRss: 0,
        memPercent: 0,
        cmdline: "nginx -g",
        startedAt: "",
        status: "",
      },
    ];
    expect(filterProcesses(procs, "www").map((p) => p.pid)).toEqual([42]);
    expect(filterProcesses(procs, "42").map((p) => p.pid)).toEqual([42]);
    expect(filterProcesses(procs, "")).toHaveLength(2);
    const svcs = [
      {
        name: "nginx.service",
        description: "web",
        state: "running",
        subState: "",
        enabled: true,
        startType: "",
      },
      {
        name: "cron.service",
        description: "jobs",
        state: "failed",
        subState: "",
        enabled: true,
        startType: "",
      },
    ];
    expect(filterServices(svcs, "", "failed").map((s) => s.name)).toEqual([
      "cron.service",
    ]);
    expect(filterServices(svcs, "web", "all")).toHaveLength(1);
  });
  it("describes rules", () => {
    const t = (s: string) => s;
    expect(
      describeRule(
        { metric: "cpu", op: "gt", threshold: 90, durationSeconds: 300 },
        t,
      ),
    ).toBe("CPU > 90%，for 5 min");
    expect(
      describeRule(
        { metric: "offline", op: "gt", threshold: 0, durationSeconds: 600 },
        t,
      ),
    ).toBe("Offline over 10 min");
    expect(
      describeRule(
        { metric: "offline", op: "gt", threshold: 0, durationSeconds: 0 },
        t,
      ),
    ).toBe("Offline");
  });

  it("counts traffic by the plan (B27)", () => {
    expect(trafficUsed(10, 3, "both")).toBe(13);
    expect(trafficUsed(10, 3, "out")).toBe(3);
    expect(trafficUsed(10, 3, "in")).toBe(10);
    expect(trafficUsed(10, 3, "max")).toBe(10);
    // 取大按累计比较：前两天流入多，第三天流出追上来
    expect(
      cumulativeUsed(
        [
          { rx: 5, tx: 1 },
          { rx: 1, tx: 3 },
          { rx: 0, tx: 5 },
        ],
        "max",
      ),
    ).toEqual([5, 6, 9]);
    expect(trafficTitle({ startDay: 1, periodMonths: 1 })).toBe(
      "Traffic this month",
    );
    expect(trafficTitle({ startDay: 4, periodMonths: 1 })).toBe(
      "Traffic this cycle",
    );
    expect(daysLeft("2026-10-03", new Date(2026, 8, 28))).toBe(6);
    expect(daysLeft("2026-09-01", new Date(2026, 8, 28))).toBe(0);
  });
});

describe("remote text files (B33)", () => {
  it("opens logs, config and rotated logs but not archives", () => {
    for (const name of [
      "syslog",
      "syslog.1",
      "app.log",
      "app.log.2",
      "nginx.conf",
      "Dockerfile",
      "config.YAML",
    ])
      expect(canViewAsText(name), name).toBe(true);
    for (const name of ["app.log.gz", "photo.jpg", "backup.tar", "db.sqlite"])
      expect(canViewAsText(name), name).toBe(false);
  });
});
