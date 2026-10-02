import { describe, expect, it } from "vitest";
import {
  authorAgentId,
  costText,
  findAgent,
  isCLI,
  kindLabel,
  hostsText,
  runEntries,
  taskRunStatus,
} from "./logic";

describe("aiagents logic", () => {
  it("reads the agent id of a comment author", () => {
    expect(authorAgentId("agent:12")).toBe(12);
    expect(authorAgentId("")).toBeUndefined();
    expect(authorAgentId(undefined)).toBeUndefined();
    expect(authorAgentId("me")).toBeUndefined();
  });
  it("shows the month's cost with the budget", () => {
    expect(costText({ monthCostUsd: 1.234, monthlyBudgetUsd: 5 })).toBe(
      "$1.23 / $5",
    );
    expect(costText({ monthCostUsd: 0, monthlyBudgetUsd: null })).toBe("$0.00");
  });
  it("labels kinds and finds agents by id", () => {
    expect(kindLabel("builtin")).toBe("Built-in");
    expect(isCLI("codex")).toBe(true);
    expect(isCLI("builtin")).toBe(false);
    const list = [{ id: 3 }, { id: 4 }] as never[];
    expect(findAgent(list, "4")).toEqual({ id: 4 });
    expect(findAgent(list, "")).toBeUndefined();
  });
});

describe("绑定的机器（B60）", () => {
  it("lists up to three machines", () => {
    const more = (n: number) => `等 ${n} 台`;
    expect(hostsText(["a", "b"], more)).toBe("a、b");
    expect(hostsText(["a", "b", "c", "d"], more)).toBe("a、b、c 等 4 台");
  });
});

describe("B86 run entries", () => {
  const task = (over: Record<string, unknown>) =>
    ({
      id: 1,
      title: "登录页",
      status: "running",
      createdAt: "2026-10-02T01:00:00Z",
      error: "",
      prUrl: "",
      issueKey: "XC-1",
      ...over,
    }) as never;
  it("uses coding tasks when the runs API is not live, newest first", () => {
    const list = runEntries({
      tasks: [
        task({ id: 1, createdAt: "2026-10-02T01:00:00Z", status: "review" }),
        task({ id: 2, createdAt: "2026-10-02T02:00:00Z" }),
      ],
    });
    expect(list.map((e) => e.taskId)).toEqual([2, 1]);
    expect(list[0].active).toBe(true);
    expect(list[1].status).toBe("waiting");
    expect(list[1].active).toBe(false);
  });
  it("prefers the runs API when it answers", () => {
    const list = runEntries({
      runs: [
        {
          id: 9,
          agentId: 3,
          agentName: "整理员",
          kind: "builtin",
          issueKey: "XC-2",
          issueTitle: "拆清单",
          status: "running",
          createdAt: "2026-10-02T03:00:00Z",
        },
      ],
      tasks: [task({})],
    });
    expect(list).toHaveLength(1);
    expect(list[0]).toMatchObject({
      runId: 9,
      active: true,
      agentName: "整理员",
    });
  });
  it("maps task statuses", () => {
    expect(taskRunStatus("pr_opened")).toBe("pr_opened");
    expect(taskRunStatus("discarded")).toBe("canceled");
    expect(taskRunStatus("pushed")).toBe("done");
  });
});
