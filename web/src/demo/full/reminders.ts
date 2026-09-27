import { fail, json, noContent, route } from "../router";
import { at, day } from "./util";

interface Reminder {
  id: number;
  title: string;
  body: string;
  link: string;
  rrule: string;
  dtstart: string;
  nextAt?: string;
  dueAt?: string;
  lastFiredAt?: string;
  snoozedUntil?: string;
  doneAt?: string;
  enabled: boolean;
  status: "scheduled" | "pending" | "snoozed" | "done" | "ended";
  createdAt: string;
}

let nextId = 1;
const reminders: Reminder[] = [];
export function addReminder(
  title: string,
  when: string,
  rrule = "",
  link = "",
  body = "",
) {
  const r: Reminder = {
    id: nextId++,
    title,
    body,
    link,
    rrule,
    dtstart: when,
    nextAt: when,
    enabled: true,
    status: "scheduled",
    createdAt: at(-60 * 24),
  };
  reminders.push(r);
  return r;
}
addReminder("交电费", at(90), "", "", "网上国网，大约 180 元");
addReminder("组会", day(0, "15:00"), "FREQ=WEEKLY;BYDAY=SA");
addReminder("喝水", at(25), "FREQ=HOURLY");
addReminder("给妈妈打电话", day(1, "20:00"));
addReminder("体检", day(2, "08:30"), "", "", "空腹，带身份证");
addReminder(
  "续费域名 example.com",
  day(5, "10:00"),
  "",
  "/monitoring/subscriptions",
);
addReminder("周报", day(6, "17:00"), "FREQ=WEEKLY;BYDAY=FR");
const late = addReminder("取快递", at(-60));
late.status = "pending";
late.dueAt = late.nextAt;
late.lastFiredAt = late.nextAt;
const done = addReminder("交房租", day(-1, "09:00"));
Object.assign(done, {
  status: "done",
  doneAt: day(-1, "09:20"),
  nextAt: undefined,
});
const done2 = addReminder("还信用卡", day(-2, "10:00"));
Object.assign(done2, {
  status: "done",
  doneAt: day(-2, "11:00"),
  nextAt: undefined,
});

const endOfToday = () => {
  const d = new Date();
  d.setHours(23, 59, 59, 999);
  return d.toISOString();
};

export function register() {
  route("GET", "/reminders", ({ query }) => {
    const range = query.get("range") ?? "upcoming";
    const eod = endOfToday();
    const items = reminders
      .filter((r) =>
        range === "done"
          ? r.status === "done"
          : range === "today"
            ? r.status !== "done" &&
              (r.status === "pending" || (!!r.nextAt && r.nextAt <= eod))
            : r.status !== "done" && !!r.nextAt && r.nextAt > eod,
      )
      .sort((a, b) =>
        range === "done"
          ? (b.doneAt ?? "").localeCompare(a.doneAt ?? "")
          : (a.nextAt ?? "").localeCompare(b.nextAt ?? ""),
      );
    return json({ items });
  });
  route("POST", "/reminders", ({ body }) =>
    json(
      addReminder(
        body.title,
        body.at,
        body.rrule ?? "",
        body.link ?? "",
        body.body ?? "",
      ),
      201,
    ),
  );
  route("GET", "/reminders/:id", ({ params }) => {
    const r = reminders.find((x) => x.id === Number(params.id));
    return r ? json(r) : fail(404, "not_found", "资源不存在");
  });
  route("PATCH", "/reminders/:id", ({ params, body }) => {
    const r = reminders.find((x) => x.id === Number(params.id));
    if (!r) return fail(404, "not_found", "资源不存在");
    for (const k of ["title", "body", "link", "rrule", "enabled"] as const)
      if (body[k] !== undefined)
        (r as unknown as Record<string, unknown>)[k] = body[k];
    if (body.at)
      Object.assign(r, {
        dtstart: body.at,
        nextAt: body.at,
        status: "scheduled",
        dueAt: undefined,
      });
    return json(r);
  });
  route("DELETE", "/reminders/:id", ({ params }) => {
    const i = reminders.findIndex((x) => x.id === Number(params.id));
    if (i >= 0) reminders.splice(i, 1);
    return noContent();
  });
  route("POST", "/reminders/:id/done", ({ params }) => {
    const r = reminders.find((x) => x.id === Number(params.id));
    if (!r) return fail(404, "not_found", "资源不存在");
    Object.assign(r, {
      status: "done",
      doneAt: at(0),
      nextAt: undefined,
      dueAt: undefined,
    });
    return json(r);
  });
  route("POST", "/reminders/:id/snooze", ({ params, body }) => {
    const r = reminders.find((x) => x.id === Number(params.id));
    if (!r) return fail(404, "not_found", "资源不存在");
    const until = body?.until ?? at(body?.minutes ?? 10);
    Object.assign(r, {
      status: "snoozed",
      snoozedUntil: until,
      nextAt: until,
      dueAt: undefined,
    });
    return json(r);
  });
}
