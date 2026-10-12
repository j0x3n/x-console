import { describe, expect, it } from "vitest";
import type { AiSession } from "./api";
import {
  filterSessions,
  groupSessions,
  isSelected,
  sessionBadge,
  sessionSource,
} from "./sessions";

const base = {
  sourceName: "",
  pendingCount: 0,
  updatedAt: "2026-10-13T08:00:00Z",
};
const list: AiSession[] = [
  {
    ...base,
    id: "chat:1",
    kind: "chat",
    refId: 1,
    title: "整理笔记",
    status: "idle",
  },
  {
    ...base,
    id: "host:2",
    kind: "host",
    refId: 2,
    title: "磁盘快满了",
    sourceName: "hk-1",
    hostId: "h1",
    status: "working",
    updatedAt: "2026-10-13T07:00:00Z",
  },
  {
    ...base,
    id: "run:3",
    kind: "agent",
    refId: 3,
    title: "XC-12 修登录",
    sourceName: "小码",
    agentId: 9,
    issueKey: "XC-12",
    status: "waiting",
    pendingCount: 2,
    updatedAt: "2026-10-13T09:00:00Z",
  },
];

describe("AI 会话列表", () => {
  it("按类型、工作中和关键字筛选", () => {
    const ids = (l: AiSession[]) => l.map((s) => s.id);
    expect(ids(filterSessions(list, "all", ""))).toEqual([
      "chat:1",
      "host:2",
      "run:3",
    ]);
    expect(ids(filterSessions(list, "active", ""))).toEqual([
      "host:2",
      "run:3",
    ]);
    expect(ids(filterSessions(list, "host", ""))).toEqual(["host:2"]);
    expect(ids(filterSessions(list, "all", "hk"))).toEqual(["host:2"]);
    expect(ids(filterSessions(list, "all", "xc-12"))).toEqual(["run:3"]);
  });

  it("进行中的单独一组，按时间倒序", () => {
    const g = groupSessions(list);
    expect(g.active.map((s) => s.id)).toEqual(["run:3", "host:2"]);
    expect(g.recent.map((s) => s.id)).toEqual(["chat:1"]);
  });

  it("顶栏标记：等确认的数量和在工作的数量", () => {
    expect(sessionBadge(list)).toEqual({ pending: 2, working: 1 });
    expect(sessionBadge(undefined)).toEqual({ pending: 0, working: 0 });
  });

  it("来源地址和选中", () => {
    expect(sessionSource(list[0])).toBeNull();
    expect(sessionSource(list[1])).toBe("/servers/h1/agent?c=2");
    expect(sessionSource(list[2])).toBe("/coding/agents/9?run=3");
    expect(isSelected(list[1], 2, null)).toBe(true);
    expect(isSelected(list[2], 3, null)).toBe(false);
    expect(isSelected(list[2], null, 3)).toBe(true);
  });
});
