import { describe, expect, it } from "vitest";
import {
  applyMove,
  column,
  dueState,
  emptyFilter,
  filterIssues,
  groupIssues,
  isNoopMove,
  parseIssueKey,
  planMove,
  sortIssues,
  sortOrderBetween,
  stepSelection,
  type Issue,
} from "./logic";

let seq = 0;
function issue(partial: Partial<Issue>): Issue {
  seq++;
  return {
    id: seq,
    key: `XC-${seq}`,
    projectId: 1,
    projectKey: "XC",
    number: seq,
    title: `Issue ${seq}`,
    description: "",
    status: "todo",
    priority: 0,
    sortOrder: seq * 1024,
    labels: [],
    externalSource: "",
    externalId: "",
    createdAt: "2026-09-01T00:00:00Z",
    updatedAt: `2026-09-${String(seq).padStart(2, "0")}T00:00:00Z`,
    ...partial,
  };
}

const keys = (list: Issue[]) => list.map((i) => i.key).join(",");

describe("filterIssues", () => {
  const a = issue({ key: "XC-1", title: "Fix login", priority: 1, labels: [{ id: 7, name: "bug", color: "" }] });
  const b = issue({ key: "XC-2", title: "Docs", status: "done", milestoneId: 3 });
  const c = issue({ key: "XC-3", title: "Login page", status: "in_progress", priority: 2 });
  const all = [a, b, c];

  it("matches everything with an empty filter", () => {
    expect(filterIssues(all, emptyFilter)).toHaveLength(3);
  });
  it("filters by several fields", () => {
    expect(keys(filterIssues(all, { ...emptyFilter, statuses: ["todo", "in_progress"] }))).toBe("XC-1,XC-3");
    expect(keys(filterIssues(all, { ...emptyFilter, priority: 2 }))).toBe("XC-3");
    expect(keys(filterIssues(all, { ...emptyFilter, labelId: 7 }))).toBe("XC-1");
    expect(keys(filterIssues(all, { ...emptyFilter, milestoneId: 3 }))).toBe("XC-2");
  });
  it("searches titles case-insensitively and exact keys", () => {
    expect(keys(filterIssues(all, { ...emptyFilter, q: "LOGIN" }))).toBe("XC-1,XC-3");
    expect(keys(filterIssues(all, { ...emptyFilter, q: "xc-2" }))).toBe("XC-2");
  });
});

describe("sorting and grouping", () => {
  const a = issue({ key: "A-1", priority: 0, dueDate: "2026-10-03", sortOrder: 3 });
  const b = issue({ key: "A-2", priority: 1, sortOrder: 1 });
  const c = issue({ key: "A-3", priority: 3, dueDate: "2026-10-01", sortOrder: 2, status: "done" });

  it("sorts by priority with no priority last", () => {
    expect(keys(sortIssues([a, b, c], "priority"))).toBe("A-2,A-3,A-1");
  });
  it("sorts by due date with empty dates last", () => {
    expect(keys(sortIssues([a, b, c], "due"))).toBe("A-3,A-1,A-2");
  });
  it("sorts manually and by update time", () => {
    expect(keys(sortIssues([a, b, c], "manual"))).toBe("A-2,A-3,A-1");
    expect(keys(sortIssues([a, b, c], "updated"))).toBe("A-3,A-2,A-1");
  });
  it("groups by status in board order", () => {
    const groups = groupIssues([a, b, c], "status", "manual");
    expect(groups.map((g) => g.id)).toEqual(["backlog", "todo", "in_progress", "in_review", "done", "canceled"]);
    expect(keys(groups[1].issues)).toBe("A-2,A-1");
    expect(keys(groups[4].issues)).toBe("A-3");
  });
  it("groups by priority with no priority last", () => {
    const groups = groupIssues([a, b, c], "priority", "manual");
    expect(groups.map((g) => g.priority)).toEqual([1, 2, 3, 4, 0]);
    expect(keys(groups[4].issues)).toBe("A-1");
  });
});

describe("board moves", () => {
  const a = issue({ key: "B-1", sortOrder: 1024 });
  const b = issue({ key: "B-2", sortOrder: 2048 });
  const c = issue({ key: "B-3", sortOrder: 3072 });
  const d = issue({ key: "B-4", status: "done", sortOrder: 0 });
  const all = [a, b, c, d];

  it("computes the midpoint like the server", () => {
    expect(sortOrderBetween(1, 3)).toBe(2);
    expect(sortOrderBetween(1, undefined)).toBe(1025);
    expect(sortOrderBetween(undefined, 0)).toBe(-1024);
    expect(sortOrderBetween(undefined, undefined)).toBe(0);
    expect(sortOrderBetween(1, 1 + 1e-9)).toBeNull();
  });
  it("plans a move between two issues", () => {
    const plan = planMove(all, "B-3", { status: "todo", index: 1 });
    expect(plan).toEqual({ status: "todo", afterKey: "B-1", beforeKey: "B-2", sortOrder: 1536 });
    expect(keys(column(applyMove(all, "B-3", plan), "todo"))).toBe("B-1,B-3,B-2");
  });
  it("plans moves to the top, the end and another column", () => {
    expect(planMove(all, "B-3", { status: "todo", index: 0 })).toMatchObject({ beforeKey: "B-1", afterKey: undefined, sortOrder: 0 });
    expect(planMove(all, "B-1", { status: "todo", index: 99 })).toMatchObject({ afterKey: "B-3", sortOrder: 3072 + 1024 });
    const toDone = planMove(all, "B-2", { status: "done", index: 1 });
    expect(toDone).toMatchObject({ status: "done", afterKey: "B-4", beforeKey: undefined });
    const moved = applyMove(all, "B-2", toDone).find((i) => i.key === "B-2")!;
    expect(moved.status).toBe("done");
    expect(moved.completedAt).toBeDefined();
  });
  it("detects drops that change nothing", () => {
    expect(isNoopMove(all, "B-2", planMove(all, "B-2", { status: "todo", index: 1 }))).toBe(true);
    expect(isNoopMove(all, "B-2", planMove(all, "B-2", { status: "todo", index: 0 }))).toBe(false);
  });
});

describe("helpers", () => {
  it("classifies due dates", () => {
    expect(dueState("2026-09-25", "2026-09-26")).toBe("overdue");
    expect(dueState("2026-09-26", "2026-09-26")).toBe("today");
    expect(dueState("2026-09-28", "2026-09-26")).toBe("soon");
    expect(dueState("2026-10-28", "2026-09-26")).toBe("later");
    expect(dueState("2026-09-25", "2026-09-26", "done")).toBe("later");
    expect(dueState(undefined, "2026-09-26")).toBeNull();
  });
  it("parses issue keys", () => {
    expect(parseIssueKey("xc-12")).toEqual({ projectKey: "XC", number: 12 });
    expect(parseIssueKey("nope")).toBeNull();
  });
  it("steps the keyboard selection", () => {
    const ks = ["a", "b", "c"];
    expect(stepSelection(ks, null, 1)).toBe("a");
    expect(stepSelection(ks, null, -1)).toBe("c");
    expect(stepSelection(ks, "b", 1)).toBe("c");
    expect(stepSelection(ks, "c", 1)).toBe("c");
    expect(stepSelection([], "a", 1)).toBeNull();
  });
});
