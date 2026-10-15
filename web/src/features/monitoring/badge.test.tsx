// @vitest-environment jsdom
import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useMonitors, useSubscriptions, type Subscription } from "./api";
import { useMonitoringBadge } from "./badge";

vi.mock("./api", () => ({
  useMonitors: vi.fn(),
  useSubscriptions: vi.fn(),
}));

describe("监控角标", () => {
  beforeEach(() => {
    vi.mocked(useMonitors).mockReturnValue({
      data: [],
    } as unknown as ReturnType<typeof useMonitors>);
  });

  it("订阅距离续费 3 天内才计入", () => {
    vi.mocked(useSubscriptions).mockReturnValue({
      data: [
        { daysLeft: 4 },
        { daysLeft: 3 },
        { daysLeft: 0 },
      ] as Subscription[],
    } as unknown as ReturnType<typeof useSubscriptions>);

    const { result } = renderHook(() => useMonitoringBadge());
    expect(result.current).toEqual({ count: 2, title: "2 个订阅快续费" });
  });
});
