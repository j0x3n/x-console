import { describe, expect, it } from "vitest";
import {
  issuesInView,
  matchesIssueSearch,
  coverImage,
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
  UNCATEGORIZED,
  applyItemMove,
  categoryPaths,
  categoryTree,
  checklistProgress,
  isNoopItemMove,
  issueDue,
  issueDueState,
  joinDue,
  localTime,
  overdueBy,
  planCategoryStep,
  planItemMove,
  sortItems,
  splitDue,
  type Category,
  type Checklist,
  type ChecklistItem,
  type Issue,
  applyListMove,
  isNoopListMove,
  listColumn,
  planListMove,
  toggleMember,
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
  const a = issue({
    key: "XC-1",
    title: "Fix login",
    priority: 1,
    labels: [{ id: 7, name: "bug", color: "" }],
  });
  const b = issue({
    key: "XC-2",
    title: "Docs",
    status: "done",
    milestoneId: 3,
  });
  const c = issue({
    key: "XC-3",
    title: "Login page",
    status: "in_progress",
    priority: 2,
  });
  const all = [a, b, c];

  it("matches everything with an empty filter", () => {
    expect(filterIssues(all, emptyFilter)).toHaveLength(3);
  });
  it("filters by several fields", () => {
    expect(
      keys(
        filterIssues(all, {
          ...emptyFilter,
          statuses: ["todo", "in_progress"],
        }),
      ),
    ).toBe("XC-1,XC-3");
    expect(keys(filterIssues(all, { ...emptyFilter, priority: 2 }))).toBe(
      "XC-3",
    );
    expect(keys(filterIssues(all, { ...emptyFilter, labelId: 7 }))).toBe(
      "XC-1",
    );
    expect(keys(filterIssues(all, { ...emptyFilter, milestoneId: 3 }))).toBe(
      "XC-2",
    );
  });
  it("searches titles case-insensitively and exact keys", () => {
    expect(keys(filterIssues(all, { ...emptyFilter, q: "LOGIN" }))).toBe(
      "XC-1,XC-3",
    );
    expect(keys(filterIssues(all, { ...emptyFilter, q: "xc-2" }))).toBe("XC-2");
  });
});

describe("sorting and grouping", () => {
  const a = issue({
    key: "A-1",
    priority: 0,
    dueDate: "2026-10-03",
    sortOrder: 3,
  });
  const b = issue({ key: "A-2", priority: 1, sortOrder: 1 });
  const c = issue({
    key: "A-3",
    priority: 3,
    dueDate: "2026-10-01",
    sortOrder: 2,
    status: "done",
  });

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
    expect(groups.map((g) => g.id)).toEqual([
      "backlog",
      "todo",
      "in_progress",
      "in_review",
      "done",
      "canceled",
    ]);
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
    expect(plan).toEqual({
      status: "todo",
      afterKey: "B-1",
      beforeKey: "B-2",
      sortOrder: 1536,
    });
    expect(keys(column(applyMove(all, "B-3", plan), "todo"))).toBe(
      "B-1,B-3,B-2",
    );
  });
  it("plans moves to the top, the end and another column", () => {
    expect(planMove(all, "B-3", { status: "todo", index: 0 })).toMatchObject({
      beforeKey: "B-1",
      afterKey: undefined,
      sortOrder: 0,
    });
    expect(planMove(all, "B-1", { status: "todo", index: 99 })).toMatchObject({
      afterKey: "B-3",
      sortOrder: 3072 + 1024,
    });
    const toDone = planMove(all, "B-2", { status: "done", index: 1 });
    expect(toDone).toMatchObject({
      status: "done",
      afterKey: "B-4",
      beforeKey: undefined,
    });
    const moved = applyMove(all, "B-2", toDone).find((i) => i.key === "B-2")!;
    expect(moved.status).toBe("done");
    expect(moved.completedAt).toBeDefined();
  });
  it("detects drops that change nothing", () => {
    expect(
      isNoopMove(
        all,
        "B-2",
        planMove(all, "B-2", { status: "todo", index: 1 }),
      ),
    ).toBe(true);
    expect(
      isNoopMove(
        all,
        "B-2",
        planMove(all, "B-2", { status: "todo", index: 0 }),
      ),
    ).toBe(false);
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

describe("B36 categories", () => {
  const cats: Category[] = [
    { id: 1, projectId: 1, name: "后端", position: 2, issueCount: 0 },
    { id: 2, projectId: 1, name: "前端", position: 1, issueCount: 0 },
    {
      id: 3,
      projectId: 1,
      parentId: 1,
      name: "云盘",
      position: 2,
      issueCount: 0,
    },
    {
      id: 4,
      projectId: 1,
      parentId: 1,
      name: "服务器",
      position: 1,
      issueCount: 0,
    },
  ];
  it("orders the tree and builds paths", () => {
    expect(categoryTree(cats).map((n) => n.path)).toEqual([
      "前端",
      "后端",
      "后端 / 服务器",
      "后端 / 云盘",
    ]);
    expect(categoryPaths(cats).get(3)).toBe("后端 / 云盘");
  });
  it("filters a top-level category with its children, and uncategorized", () => {
    const a = issue({ categoryId: 3 });
    const b = issue({ categoryId: 2 });
    const c = issue({});
    const all = [a, b, c];
    expect(
      keys(filterIssues(all, { ...emptyFilter, categoryId: 1 }, cats)),
    ).toBe(a.key);
    expect(
      keys(
        filterIssues(all, { ...emptyFilter, categoryId: UNCATEGORIZED }, cats),
      ),
    ).toBe(c.key);
  });
  it("groups by category with uncategorized last", () => {
    const a = issue({ categoryId: 3 });
    const c = issue({});
    const groups = groupIssues([a, c], "category", "manual", cats);
    expect(groups.map((g) => g.label)).toEqual([
      "前端",
      "后端",
      "后端 / 服务器",
      "后端 / 云盘",
      "Uncategorized",
    ]);
    expect(groups.at(-2)?.issues).toEqual([a]);
    expect(groups.at(-1)?.issues).toEqual([c]);
  });
  it("plans moving a category up and down among its siblings", () => {
    expect(planCategoryStep(cats, 3, -1)).toEqual({
      afterId: undefined,
      beforeId: 4,
    });
    expect(planCategoryStep(cats, 3, 1)).toBeNull();
    expect(planCategoryStep(cats, 2, 1)).toEqual({
      afterId: 1,
      beforeId: undefined,
    });
  });
});

describe("B36 due time", () => {
  const now = new Date(2026, 8, 29, 12, 0);
  it("falls back to 23:59 for old date-only data", () => {
    const at = issueDue({ dueDate: "2026-09-29" })!;
    expect(localTime(at)).toBe("23:59");
    expect(
      issueDue({
        dueAt: "2026-09-29T10:00:00Z",
        dueDate: "2026-09-01",
      })?.toISOString(),
    ).toBe("2026-09-29T10:00:00.000Z");
  });
  it("classifies due times against the current minute", () => {
    const at = (h: number, d = 29) => new Date(2026, 8, d, h, 0).toISOString();
    expect(issueDueState(issue({ dueAt: at(10) }), now)).toBe("overdue");
    expect(issueDueState(issue({ dueAt: at(18) }), now)).toBe("today");
    expect(issueDueState(issue({ dueAt: at(18, 30) }), now)).toBe("soon");
    expect(issueDueState(issue({ dueAt: at(10), status: "done" }), now)).toBe(
      "later",
    );
  });
  it("says how long ago it was due", () => {
    const ago = (ms: number) => new Date(now.getTime() - ms);
    expect(overdueBy(ago(5 * 60_000), now)).toEqual({
      value: 5,
      unit: "minutes",
    });
    expect(overdueBy(ago(2 * 3_600_000), now)).toEqual({
      value: 2,
      unit: "hours",
    });
    expect(overdueBy(ago(3 * 86_400_000), now)).toEqual({
      value: 3,
      unit: "days",
    });
  });
  it("splits and joins local date and time", () => {
    const iso = joinDue("2026-10-03", "18:30")!;
    expect(splitDue({ dueAt: iso })).toEqual({
      date: "2026-10-03",
      time: "18:30",
    });
    expect(splitDue({ dueAt: joinDue("2026-10-03", "")! }).time).toBe("23:59");
    expect(joinDue("", "18:30")).toBeNull();
  });
  it("filters today, this week and overdue", () => {
    const at = (h: number, d = 29) => new Date(2026, 8, d, h, 0).toISOString();
    const late = issue({ dueAt: at(10) });
    const today = issue({ dueAt: at(18) });
    const week = issue({ dueAt: at(9, 30) });
    const done = issue({ dueAt: at(10), status: "done" });
    const all = [late, today, week, done];
    expect(
      keys(filterIssues(all, { ...emptyFilter, due: "overdue" }, [], now)),
    ).toBe(late.key);
    expect(
      keys(filterIssues(all, { ...emptyFilter, due: "today" }, [], now)),
    ).toBe(`${late.key},${today.key}`);
    expect(
      keys(filterIssues(all, { ...emptyFilter, due: "week" }, [], now)),
    ).toBe(`${late.key},${today.key},${week.key}`);
  });
  it("sorts by due time", () => {
    const a = issue({ dueAt: "2026-10-01T09:00:00Z" });
    const b = issue({ dueAt: "2026-10-01T08:00:00Z" });
    const c = issue({});
    expect(keys(sortIssues([c, a, b], "due"))).toBe(
      `${b.key},${a.key},${c.key}`,
    );
  });
});

describe("B36 checklists", () => {
  const item = (id: number, position: number, done = false): ChecklistItem => ({
    id,
    checklistId: 1,
    text: `item ${id}`,
    done,
    position,
  });
  const items = [item(1, 1, true), item(2, 2), item(3, 3, true)];
  it("counts progress across checklists", () => {
    const list = (id: number, its: ChecklistItem[]): Checklist => ({
      id,
      issueId: 1,
      title: "",
      position: id,
      items: its,
    });
    expect(checklistProgress([list(1, items), list(2, [item(4, 1)])])).toEqual({
      done: 2,
      total: 4,
    });
  });
  it("plans and applies an item move", () => {
    const plan = planItemMove(items, 3, 0);
    expect(plan).toEqual({ afterId: undefined, beforeId: 1 });
    expect(isNoopItemMove(items, 3, plan)).toBe(false);
    expect(isNoopItemMove(items, 2, planItemMove(items, 2, 1))).toBe(true);
    const moved = sortItems(applyItemMove(items, 3, plan)).map((i) => i.id);
    expect(moved).toEqual([3, 1, 2]);
  });
});

describe("B46 list moves", () => {
  const a = issue({ listId: 1, sortOrder: 100 });
  const b = issue({ listId: 1, sortOrder: 200 });
  const c = issue({ listId: 2, sortOrder: 100, status: "in_progress" });
  const all = [a, b, c];

  it("plans a move between neighbours and into another list", () => {
    expect(planListMove(all, a.key, { listId: 1, index: 1 })).toMatchObject({
      listId: 1,
      afterKey: b.key,
      beforeKey: undefined,
      sortOrder: 1224,
    });
    const plan = planListMove(
      all,
      a.key,
      { listId: 2, index: 0 },
      "in_progress",
    );
    expect(plan).toMatchObject({
      listId: 2,
      beforeKey: c.key,
      status: "in_progress",
    });
    const moved = applyListMove(all, a.key, plan);
    expect(keys(listColumn(moved, 2))).toBe(`${a.key},${c.key}`);
    expect(moved.find((i) => i.key === a.key)?.status).toBe("in_progress");
  });

  it("keeps the status for a list without one and spots no-op moves", () => {
    const plan = planListMove(all, c.key, { listId: 1, index: 0 });
    expect(
      applyListMove(all, c.key, plan).find((i) => i.key === c.key)?.status,
    ).toBe("in_progress");
    expect(
      isNoopListMove(
        all,
        a.key,
        planListMove(all, a.key, { listId: 1, index: 0 }),
      ),
    ).toBe(true);
  });

  it("toggles me as a member", () => {
    const withMe = { ...a, members: [{ kind: "me" as const, id: "" }] };
    expect(toggleMember(a, "me")).toEqual([{ kind: "me", id: "" }]);
    expect(toggleMember(withMe, "me")).toEqual([]);
  });
});

describe("coverImage", () => {
  it("取描述里第一张图", () => {
    expect(
      coverImage("文字\n![截图](/api/v1/files/12)\n![b](/api/v1/files/13)"),
    ).toBe("/api/v1/files/12");
    expect(coverImage('![x](https://a.com/p.png "标题")')).toBe(
      "https://a.com/p.png",
    );
  });
  it("没有图或者地址不对时为空", () => {
    expect(coverImage("")).toBeNull();
    expect(coverImage("[链接](/x)")).toBeNull();
    expect(coverImage("![x](javascript:alert(1))")).toBeNull();
  });
});

describe("issuesInView", () => {
  const issue = (key: string, dueDate?: string) =>
    ({ key, title: key, status: "todo", dueDate }) as Issue;
  const all = [
    issue("A-1", "2026-10-04"),
    issue("A-2", "2026-10-05"),
    issue("A-3", "2026-10-11"),
    issue("A-4", "2026-10-12"),
    issue("A-5"),
  ];
  const keys = (list: Issue[]) => list.map((i) => i.key);
  it("按截止日期分到各个视图", () => {
    expect(keys(issuesInView("mine", all, "2026-10-05"))).toHaveLength(5);
    expect(keys(issuesInView("overdue", all, "2026-10-05"))).toEqual(["A-1"]);
    expect(keys(issuesInView("today", all, "2026-10-05"))).toEqual(["A-2"]);
    expect(keys(issuesInView("week", all, "2026-10-05"))).toEqual([
      "A-2",
      "A-3",
    ]);
  });
  it("搜索标题和编号", () => {
    expect(matchesIssueSearch(issue("XC-12"), "xc-1")).toBe(true);
    expect(matchesIssueSearch(issue("XC-12"), "  ")).toBe(true);
    expect(matchesIssueSearch(issue("XC-12"), "abc")).toBe(false);
  });
});
