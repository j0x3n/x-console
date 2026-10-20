// @vitest-environment jsdom
import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useQuotaAccounts } from "../quotas/api";
import { useWatchers } from "../watchers/api";
import { useMonitors, useSubscriptions, type Subscription } from "./api";
import { useMonitoringBadge } from "./badge";

vi.mock("./api", () => ({
  useMonitors: vi.fn(),
  useSubscriptions: vi.fn(),
}));
vi.mock("../quotas/api", () => ({ useQuotaAccounts: vi.fn() }));
vi.mock("../watchers/api", () => ({ useWatchers: vi.fn() }));

function mockData<T extends (...args: never[]) => unknown>(
  hook: T,
  data: unknown[],
) {
  vi.mocked(hook).mockReturnValue({ data } as ReturnType<T>);
}

describe("监控角标", () => {
  beforeEach(() => {
    mockData(useMonitors, []);
    mockData(useSubscriptions, []);
    mockData(useQuotaAccounts, []);
    mockData(useWatchers, []);
  });

  it("订阅距离续费 3 天内才计入，再点一次跳到订阅", () => {
    mockData(useSubscriptions, [
      { daysLeft: 4 },
      { daysLeft: 3 },
      { daysLeft: 0 },
    ] as Subscription[]);

    const { result } = renderHook(() => useMonitoringBadge());
    expect(result.current).toEqual({
      count: 2,
      title: "2 个订阅快续费",
      to: "/monitoring/subscriptions",
    });
  });

  it("快到期的证书、出错的额度和等你处理的自动化都计入，先跳证书", () => {
    mockData(useMonitors, [
      { id: 1, kind: "tls", enabled: true, lastStatus: "up", daysLeft: 5 },
      { id: 2, kind: "tls", enabled: true, lastStatus: "up", daysLeft: 30 },
    ]);
    mockData(useQuotaAccounts, [{ status: "error" }, { status: "ok" }]);
    mockData(useWatchers, [{ status: "waiting" }, { status: "idle" }]);

    const { result } = renderHook(() => useMonitoringBadge());
    expect(result.current).toEqual({
      count: 3,
      title:
        "1 个证书或域名故障或快到期，1 个 AI 额度读取出错，1 个网站自动化等你处理",
      to: "/monitoring/certs",
    });
  });

  it("只有一个网站故障时直接打开它", () => {
    mockData(useMonitors, [
      { id: 7, kind: "http", enabled: true, lastStatus: "down" },
    ]);
    const { result } = renderHook(() => useMonitoringBadge());
    expect(result.current?.to).toBe("/monitoring?monitor=7");
  });
});
