import { describe, expect, it } from "vitest";
import type { ExternalReminder, Reminder } from "./api";
import { mergeEntries } from "./external";

const reminder = (id: number, at: string): Reminder => ({
  id,
  title: `提醒 ${id}`,
  body: "",
  link: "",
  rrule: "",
  dtstart: at,
  enabled: true,
  status: "scheduled",
  createdAt: at,
});

const external = (id: string, at: string): ExternalReminder => ({
  id,
  source: "subscription",
  sourceLabel: "订阅",
  title: `续费 ${id}`,
  at,
  link: "/monitoring/subscriptions",
  done: false,
});

describe("merging other modules into the reminders list (B37)", () => {
  it("sorts by time and puts own reminders first on a tie", () => {
    const entries = mergeEntries(
      [
        reminder(1, "2026-10-02T09:00:00Z"),
        {
          ...reminder(2, "2026-10-01T09:00:00Z"),
          dueAt: "2026-10-03T09:00:00Z",
        },
      ],
      [
        external("a", "2026-10-02T09:00:00Z"),
        external("b", "2026-10-01T00:00:00Z"),
      ],
    );
    expect(entries.map((e) => e.key)).toEqual([
      "xsubscription:b",
      "r1",
      "xsubscription:a",
      "r2",
    ]);
  });

  it("works with nothing from other modules", () => {
    expect(
      mergeEntries([reminder(1, "2026-10-02T09:00:00Z")], []),
    ).toHaveLength(1);
  });
});
