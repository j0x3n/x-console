import { fail, json, noContent, route } from "../router";
import { at, date, day } from "./util";

const calendars = [
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
);

const events = seed.map((e, i) => {
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
  };
});
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
const briefSettings = {
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
  route("GET", "/weather", () =>
    json({
      location: "北京",
      latitude: 39.9,
      longitude: 116.4,
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
