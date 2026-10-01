import { describe, expect, it } from "vitest";
import { authorAgentId, costText, findAgent, isCLI, kindLabel } from "./logic";

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
