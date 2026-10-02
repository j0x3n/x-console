import { describe, expect, it } from "vitest";
import {
  cardDefs,
  arrange,
  columnCards,
  defaultLayout,
  moveCard,
  normalizeLayout,
  placeCard,
  shiftInColumn,
  shiftCard,
  spreadSide,
  toggleCard,
} from "./layout";
import {
  daysLate,
  dueToday,
  greetingKey,
  localDateKey,
  nextEvent,
  overdue,
  sortEvents,
  summaryLine,
} from "./today";

const at = (h: number, m = 0) => new Date(2026, 8, 27, h, m);

describe("greetingKey", () => {
  it("follows the local hour", () => {
    expect(greetingKey(at(6))).toBe("Good morning");
    expect(greetingKey(at(12))).toBe("Good noon");
    expect(greetingKey(at(15))).toBe("Good afternoon");
    expect(greetingKey(at(22))).toBe("Good evening");
    expect(greetingKey(at(2))).toBe("Good evening");
  });
});

describe("issue dates", () => {
  const now = at(10);
  const items = [
    { id: 1, dueDate: "2026-09-27" },
    { id: 2, dueDate: "2026-09-20" },
    { id: 3, dueDate: "2026-09-25" },
    { id: 4, dueDate: "2026-09-30" },
    { id: 5 },
  ];

  it("uses the local date", () => {
    expect(localDateKey(new Date(2026, 0, 5, 23, 30))).toBe("2026-01-05");
  });

  it("splits due today and overdue", () => {
    expect(dueToday(items, now).map((i) => i.id)).toEqual([1]);
    expect(overdue(items, now).map((i) => i.id)).toEqual([2, 3]);
  });

  it("counts days late", () => {
    expect(daysLate("2026-09-20", now)).toBe(7);
    expect(daysLate("2026-09-27", now)).toBe(0);
    expect(daysLate("2026-09-30", now)).toBe(0);
  });
});

describe("events", () => {
  const events = [
    {
      id: "b",
      start: at(14).toISOString(),
      end: at(15).toISOString(),
      allDay: false,
    },
    {
      id: "a",
      start: at(9).toISOString(),
      end: at(10).toISOString(),
      allDay: false,
    },
    {
      id: "d",
      start: at(0).toISOString(),
      end: at(24).toISOString(),
      allDay: true,
    },
  ];

  it("puts all-day events first, then by start time", () => {
    expect(sortEvents(events).map((e) => e.id)).toEqual(["d", "a", "b"]);
  });

  it("finds the next timed event that has not ended", () => {
    expect(nextEvent(events, at(9, 30))?.id).toBe("a");
    expect(nextEvent(events, at(11))?.id).toBe("b");
    expect(nextEvent(events, at(16))).toBeUndefined();
  });
});

describe("summaryLine", () => {
  it("joins what is known", () => {
    expect(
      summaryLine(
        {
          dueToday: 3,
          reminders: 2,
          serversTotal: 2,
          serversOffline: 0,
          alerts: 0,
        },
        "zh",
      ),
    ).toBe("今天 3 个待办，2 个提醒，服务器全部在线");
  });

  it("reports offline servers before alerts and skips missing parts", () => {
    expect(
      summaryLine({ serversTotal: 3, serversOffline: 1, alerts: 4 }, "zh"),
    ).toBe("1 台服务器离线");
    expect(summaryLine({ dueToday: 0, serversTotal: 0 }, "en")).toBe(
      "0 due today",
    );
  });
});

describe("layout", () => {
  it("fills in unknown and missing cards", () => {
    const layout = normalizeLayout([
      { id: "activity", visible: false, order: 0 },
      { id: "gone", visible: true, order: 1 },
      { id: "todos", visible: true, order: 2 },
      { id: "todos", visible: false, order: 3 },
    ]);
    expect(layout.map((c) => c.id)).toEqual([
      "activity",
      "todos",
      ...cardDefs
        .map((d) => d.id)
        .filter((id) => id !== "activity" && id !== "todos"),
    ]);
    expect(layout[0]).toEqual({ id: "activity", visible: false, order: 0 });
    expect(layout[1].visible).toBe(true);
  });

  it("moves cards only inside their column", () => {
    const base = defaultLayout();
    const moved = moveCard(base, "activity", "schedule");
    expect(columnCards(moved, "side").map((c) => c.id)).toEqual([
      "activity",
      "schedule",
      "habits",
      "home",
      "network",
      "mail",
      "monitoring",
      "fitness",
    ]);
    expect(moveCard(base, "todos", "schedule")).toBe(base);
  });

  it("shifts and toggles", () => {
    const base = defaultLayout();
    expect(
      columnCards(shiftCard(base, "decisions", -1), "main").map((c) => c.id),
    ).toEqual(["decisions", "todos"]);
    expect(shiftCard(base, "todos", -1)).toBe(base);
    expect(toggleCard(base, "home").find((c) => c.id === "home")?.visible).toBe(
      false,
    );
  });

  it("keeps optional cards hidden until turned on", () => {
    expect(defaultLayout().find((c) => c.id === "fitness")?.visible).toBe(
      false,
    );
    expect(normalizeLayout([]).find((c) => c.id === "fitness")?.visible).toBe(
      false,
    );
  });

  it("spreads side cards over columns by height", () => {
    const side = columnCards(defaultLayout(), "side");
    expect(spreadSide(side, 1)[0]).toHaveLength(side.length);
    const two = spreadSide(side, 2).map((col) => col.map((c) => c.id));
    expect(two).toEqual([
      ["schedule", "network", "mail", "activity"],
      ["habits", "home", "monitoring", "fitness"],
    ]);
  });

  it("keeps order unique after a move", () => {
    const orders = moveCard(defaultLayout(), "home", "schedule").map(
      (c) => c.order,
    );
    expect(new Set(orders).size).toBe(orders.length);
  });
});

describe("B88 free columns", () => {
  const ids = (cols: { id: string }[][]) => cols.map((c) => c.map((x) => x.id));
  it("keeps the old arrangement when no card has a column", () => {
    const cols = arrange(defaultLayout(), 3, (c) => c.visible);
    expect(cols[0].map((c) => c.id)).toEqual(["todos", "decisions"]);
    expect(cols[1].length + cols[2].length).toBeGreaterThan(0);
  });
  it("moves a main card to the right column and back", () => {
    const base = defaultLayout();
    const moved = placeCard(base, "todos", 2, null, 3);
    const cols = ids(arrange(moved, 3));
    expect(cols[0]).toEqual(["decisions"]);
    expect(cols[2][cols[2].length - 1]).toBe("todos");
    const back = placeCard(moved, "schedule", 0, "decisions", 3);
    expect(ids(arrange(back, 3))[0]).toEqual(["schedule", "decisions"]);
  });
  it("puts cards of a missing column into the last one", () => {
    const moved = placeCard(defaultLayout(), "todos", 3, null, 4);
    const cols = ids(arrange(moved, 2));
    expect(cols[1]).toContain("todos");
  });
  it("shifts within a column", () => {
    const base = placeCard(defaultLayout(), "todos", 1, "schedule", 3);
    const col = ids(arrange(base, 3))[1];
    expect(col[0]).toBe("todos");
    const down = ids(arrange(shiftInColumn(base, "todos", 1, 3), 3))[1];
    expect(down.indexOf("todos")).toBe(1);
  });
  it("reorders across main and side on one column", () => {
    const moved = placeCard(defaultLayout(), "schedule", 0, "todos", 1);
    expect(ids(arrange(moved, 1))[0].slice(0, 2)).toEqual([
      "schedule",
      "todos",
    ]);
  });
});
