import { fail, json, noContent, route } from "../router";
import { at, date, day } from "./util";

const calendars: {
  id: number;
  name: string;
  kind: string;
  url: string;
  username: string;
  hasPassword: boolean;
  color: string;
  enabled: boolean;
  lastSyncedAt?: string;
  lastError: string;
  eventCount: number;
  writable: boolean;
  createdAt: string;
}[] = [
  {
    id: 4,
    name: "我的日程",
    kind: "local",
    url: "",
    username: "",
    hasPassword: false,
    color: "#cc7752",
    enabled: true,
    lastError: "",
    eventCount: 0,
    writable: true,
    createdAt: at(-60 * 24 * 30),
  },
  {
    id: 1,
    name: "工作",
    kind: "caldav",
    url: "https://caldav.example.com/work",
    username: "jo",
    hasPassword: true,
    color: "#70b5f7",
    enabled: true,
    lastSyncedAt: at(-8),
    lastError: "",
    eventCount: 0,
    writable: true,
    createdAt: at(-60 * 24 * 90),
  },
  {
    id: 2,
    name: "生活",
    kind: "ics",
    url: "https://calendar.example.com/life.ics",
    username: "",
    hasPassword: false,
    color: "#5cc98b",
    enabled: true,
    lastSyncedAt: at(-12),
    lastError: "",
    eventCount: 0,
    writable: false,
    createdAt: at(-60 * 24 * 60),
  },
  {
    id: 3,
    name: "节假日",
    kind: "ics",
    url: "https://calendar.example.com/holidays.ics",
    username: "",
    hasPassword: false,
    color: "#e8b454",
    enabled: true,
    lastSyncedAt: at(-60 * 3),
    lastError: "",
    eventCount: 0,
    writable: false,
    createdAt: at(-60 * 24 * 200),
  },
];

type Ev = {
  cal: number;
  title: string;
  d: number;
  start?: string;
  end?: string;
  allDay?: boolean;
  loc?: string;
  recurring?: boolean;
};
const seed: Ev[] = [];
for (let d = -21; d <= 35; d++) {
  const wd = new Date(Date.now() + d * 86400000).getDay();
  if (wd >= 1 && wd <= 5)
    seed.push({
      cal: 1,
      title: "站会",
      d,
      start: "09:30",
      end: "09:45",
      recurring: true,
      loc: "线上",
    });
  if (wd === 3)
    seed.push({
      cal: 1,
      title: "技术评审",
      d,
      start: "14:00",
      end: "15:30",
      loc: "3 楼会议室",
    });
  if (wd === 6)
    seed.push({
      cal: 2,
      title: "爬山",
      d,
      start: "08:00",
      end: "12:00",
      loc: "香山",
    });
  if (wd === 0)
    seed.push({ cal: 2, title: "和爸妈吃饭", d, start: "18:00", end: "20:00" });
}
seed.push(
  {
    cal: 1,
    title: "季度复盘",
    d: 0,
    start: "16:00",
    end: "17:00",
    loc: "大会议室",
  },
  {
    cal: 2,
    title: "牙医",
    d: 1,
    start: "10:30",
    end: "11:30",
    loc: "口腔医院",
  },
  { cal: 3, title: "国庆节", d: 4, allDay: true },
  { cal: 2, title: "云南旅行", d: 6, allDay: true },
  { cal: 1, title: "发版 2.0", d: 12, start: "20:00", end: "22:00" },
  { cal: 4, title: "取快递", d: 0, start: "12:00", end: "12:30" },
  {
    cal: 4,
    title: "健身",
    d: 1,
    start: "19:00",
    end: "20:00",
    loc: "楼下健身房",
  },
  { cal: 4, title: "交房租", d: 3, allDay: true },
);

type DemoEvent = {
  id: string;
  eventId: number;
  calendarId: number;
  calendar: string;
  color: string;
  title: string;
  start: string;
  end: string;
  allDay: boolean;
  startDate?: string;
  endDate?: string;
  location: string;
  description: string;
  recurring: boolean;
  writable: boolean;
};
const events: DemoEvent[] = seed.map((e, i) => {
  const c = calendars.find((x) => x.id === e.cal)!;
  return {
    id: `demo-${i}`,
    eventId: i + 1,
    calendarId: c.id,
    calendar: c.name,
    color: c.color,
    title: e.title,
    start: e.allDay ? day(e.d, "00:00") : day(e.d, e.start),
    end: e.allDay ? day(e.d + 1, "00:00") : day(e.d, e.end),
    allDay: !!e.allDay,
    startDate: e.allDay ? date(e.d) : undefined,
    endDate: e.allDay ? date(e.d + 1) : undefined,
    location: e.loc ?? "",
    description: "",
    recurring: !!e.recurring,
    writable: c.writable && !e.recurring,
  };
});

/** 把请求体变成一条假事件。全天事件用本地零点。 */
function fillEvent(target: DemoEvent, body: Record<string, unknown>) {
  const c = calendars.find(
    (x) => x.id === Number(body.calendarId ?? target.calendarId),
  );
  if (!c || !c.writable) return "这个日历不能改";
  Object.assign(target, {
    calendarId: c.id,
    calendar: c.name,
    color: c.color,
    title: (body.title as string) ?? target.title,
    location: (body.location as string) ?? target.location,
    description: (body.description as string) ?? target.description,
    allDay: (body.allDay as boolean) ?? target.allDay,
  });
  if (target.allDay) {
    target.startDate = (body.startDate as string) ?? target.startDate;
    target.endDate = (body.endDate as string) ?? target.endDate;
    target.start = new Date(`${target.startDate}T00:00:00`).toISOString();
    target.end = new Date(`${target.endDate}T00:00:00`).toISOString();
  } else {
    target.start = (body.start as string) ?? target.start;
    target.end = (body.end as string) ?? target.end;
    target.startDate = undefined;
    target.endDate = undefined;
  }
  target.id = `demo-${target.eventId}-${target.start}`;
  return null;
}
let nextEventId = 1000;
let nextCalendarId = 10;
const countEvents = () => {
  for (const c of calendars)
    c.eventCount = events.filter((e) => e.calendarId === c.id).length;
};
for (const c of calendars)
  c.eventCount = events.filter((e) => e.calendarId === c.id).length;

const md = (s: string) => s.trim();
const briefs = [0, -1, -2, -3, -4].map((d, i) => ({
  id: 10 - i,
  date: date(d),
  createdAt: day(d, "07:30"),
  sentAt: day(d, "07:30"),
  content: "",
  sections: [
    {
      key: "summary",
      title: "概况",
      markdown: md(
        `今天有 ${3 - (i % 2)} 个日程、4 个要到期的 Issue。天气晴，最高 26 度。`,
      ),
    },
    { key: "weather", title: "天气", markdown: "晴，16 到 26 度，降水 10%。" },
    {
      key: "calendar",
      title: "日程",
      markdown: "- 09:30 站会\n- 16:00 季度复盘",
    },
    {
      key: "issues",
      title: "要到期的 Issue",
      markdown:
        "- [XC-2](/projects/XC/2) 登录页支持只用密码登录（今天）\n- [TRIP-2](/projects/TRIP/2) 买机票（今天）",
    },
    { key: "reminders", title: "提醒", markdown: "- 交电费\n- 组会" },
    {
      key: "alerts",
      title: "告警",
      markdown:
        i === 1 ? "- web-1 CPU 超过 90%，持续 6 分钟，已恢复" : "没有告警。",
    },
    { key: "habits", title: "习惯", markdown: "喝水 5/8 杯，跑步已完成。" },
  ],
}));
for (const b of briefs)
  b.content = b.sections
    .map((s) => `## ${s.title}\n\n${s.markdown}`)
    .join("\n\n");
const demoPlaces = [
  {
    name: "北京",
    region: "北京",
    country: "中国",
    lat: 39.9075,
    lon: 116.3972,
  },
  {
    name: "上海",
    region: "上海",
    country: "中国",
    lat: 31.2222,
    lon: 121.4581,
  },
  {
    name: "深圳",
    region: "广东",
    country: "中国",
    lat: 22.5455,
    lon: 114.0683,
  },
  { name: "广州", region: "广东", country: "中国", lat: 23.1167, lon: 113.25 },
  {
    name: "杭州",
    region: "浙江",
    country: "中国",
    lat: 30.2936,
    lon: 120.1614,
  },
  {
    name: "成都",
    region: "四川",
    country: "中国",
    lat: 30.6667,
    lon: 104.0667,
  },
];
const rainAlert = { enabled: true, threshold: 60, leadHours: 2 };
const briefSettings: {
  location?: { lat: number; lon: number; name?: string };
  [key: string]: unknown;
} = {
  enabled: true,
  time: "07:30",
  channels: ["webpush", "telegram"],
  location: { lat: 39.9, lon: 116.4, name: "北京" },
  sections: [
    "summary",
    "weather",
    "calendar",
    "issues",
    "reminders",
    "alerts",
    "habits",
  ],
  aiPolish: false,
  availableChannels: ["webpush", "telegram", "bark"],
  aiAvailable: false,
  nextRunAt: day(1, "07:30"),
};

let focus: Record<string, unknown> | null = null;
const recent = [0, -1, -1, -2, -3].map((d, i) => ({
  id: 20 - i,
  issueKey: ["XC-1", "XC-10", "BLOG-2", "XC-3", "READ-1"][i],
  issueTitle: [
    "云盘上传大文件时内存占满",
    "隐藏内容的审计日志",
    "文章加全文搜索",
    "AI 助手接上真的模型",
    "《原则》读完写笔记",
  ][i],
  startedAt: day(d, `${10 + i}:00`),
  endsAt: day(d, `${10 + i}:25`),
  endedAt: day(d, `${10 + i}:25`),
  plannedMinutes: 25,
  actualSeconds: 1500,
  completed: true,
  note: "",
}));

export function register() {
  route("GET", "/calendars", () => json(calendars));
  route("POST", "/calendars", ({ body }) => {
    const c = {
      id: nextCalendarId++,
      name: body.name,
      kind: body.kind,
      url: body.url ?? "",
      username: body.username ?? "",
      hasPassword: !!body.password,
      color: body.color ?? "",
      enabled: true,
      lastSyncedAt: body.kind === "local" ? undefined : at(0),
      lastError: "",
      eventCount: 0,
      writable: body.kind !== "ics",
      createdAt: at(0),
    };
    calendars.push(c);
    return json(c, 201);
  });
  route("PATCH", "/calendars/:id", ({ params, body }) => {
    const c = calendars.find((x) => x.id === Number(params.id));
    if (!c) return fail(404, "not_found", "资源不存在");
    const { password, ...rest } = body;
    Object.assign(c, rest);
    if (password) c.hasPassword = true;
    for (const e of events)
      if (e.calendarId === c.id) {
        e.calendar = c.name;
        e.color = c.color;
      }
    return json(c);
  });
  route("DELETE", "/calendars/:id", ({ params }) => {
    const i = calendars.findIndex((x) => x.id === Number(params.id));
    if (i >= 0) calendars.splice(i, 1);
    for (let j = events.length - 1; j >= 0; j--)
      if (events[j].calendarId === Number(params.id)) events.splice(j, 1);
    return noContent();
  });
  route("POST", "/calendar/events", ({ body }) => {
    const e = {
      eventId: nextEventId++,
      recurring: false,
      writable: true,
      location: "",
      description: "",
    } as DemoEvent;
    const err = fillEvent(e, body);
    if (err) return fail(400, "invalid", err);
    events.push(e);
    countEvents();
    return json(e, 201);
  });
  route("PATCH", "/calendar/events/:id", ({ params, body }) => {
    const e = events.find((x) => x.eventId === Number(params.id));
    if (!e) return fail(404, "not_found", "资源不存在");
    if (!e.writable) return fail(400, "invalid", "这个日程不能改");
    const err = fillEvent(e, body);
    if (err) return fail(400, "invalid", err);
    countEvents();
    return json(e);
  });
  route("DELETE", "/calendar/events/:id", ({ params }) => {
    const i = events.findIndex((x) => x.eventId === Number(params.id));
    if (i >= 0) events.splice(i, 1);
    countEvents();
    return noContent();
  });
  route("GET", "/calendars/:id", ({ params }) => {
    const c = calendars.find((x) => x.id === Number(params.id));
    return c ? json(c) : fail(404, "not_found", "资源不存在");
  });
  route("POST", "/calendars/:id/sync", ({ params }) => {
    const c = calendars.find((x) => x.id === Number(params.id));
    if (c) c.lastSyncedAt = at(0);
    return noContent(202);
  });
  route("GET", "/calendar/events", ({ query }) => {
    const from = query.get("from") ?? day(-30);
    const to = query.get("to") ?? day(60);
    return json(
      events
        .filter((e) => e.end >= from && e.start <= to)
        .sort((a, b) => a.start.localeCompare(b.start)),
    );
  });
  route("GET", "/briefs", () => json({ items: briefs }));
  route("GET", "/briefs/settings", () => json(briefSettings));
  route("PUT", "/briefs/settings", ({ body }) =>
    json(Object.assign(briefSettings, body)),
  );
  route("POST", "/briefs/generate", () => json(briefs[0]));
  route("GET", "/briefs/:date", ({ params }) => {
    const b = briefs.find((x) => x.date === params.date);
    return b ? json(b) : fail(404, "not_found", "那天没有早报");
  });
  route("GET", "/weather/places", ({ query }) => {
    const q = query.get("q") ?? "";
    return json(
      demoPlaces.filter((p) => p.name.includes(q) || q.includes(p.name)),
    );
  });
  route("GET", "/weather/alert", () => json(rainAlert));
  route("PUT", "/weather/alert", ({ body }) =>
    json(Object.assign(rainAlert, body)),
  );
  route("GET", "/weather", () =>
    json({
      location: briefSettings.location?.name ?? "北京",
      latitude: briefSettings.location?.lat ?? 39.9,
      longitude: briefSettings.location?.lon ?? 116.4,
      temperature: 22.5,
      weatherCode: 1,
      summary: "晴",
      high: 26,
      low: 16,
      precipitationChance: 10,
      fetchedAt: at(-5),
    }),
  );
  route("GET", "/focus/current", () => json(focus ? { session: focus } : {}));
  route("POST", "/focus/start", ({ body }) => {
    const minutes = body?.minutes ?? 25;
    focus = {
      id: 99,
      issueKey: body?.issueKey ?? "",
      startedAt: at(0),
      endsAt: at(minutes),
      plannedMinutes: minutes,
      actualSeconds: 0,
      completed: false,
      note: body?.note ?? "",
    };
    return json(focus, 201);
  });
  route("POST", "/focus/:id/stop", () => {
    const s = focus;
    focus = null;
    return json({ ...s, endedAt: at(0), completed: false });
  });
  route("GET", "/focus/stats", ({ query }) => {
    const n = Number(query.get("days") ?? 7);
    const days = Array.from({ length: n }, (_, i) => ({
      date: date(i - n + 1),
      seconds: [3000, 1500, 4500, 0, 3000, 6000, 1500][i % 7],
      sessions: [2, 1, 3, 0, 2, 4, 1][i % 7],
    }));
    return json({
      days,
      totalSeconds: days.reduce((s, d) => s + d.seconds, 0),
      completed: days.reduce((s, d) => s + d.sessions, 0),
      byIssue: [
        { issueKey: "XC-1", seconds: 7500 },
        { issueKey: "BLOG-2", seconds: 4500 },
        { issueKey: "XC-10", seconds: 3000 },
      ],
      recent,
    });
  });
}
