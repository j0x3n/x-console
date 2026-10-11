import type { InboxItem, InboxKind } from "./api";

export const KIND_OPTIONS: Array<{ value: InboxKind; label: string }> = [
  { value: "reminder", label: "Inbox kind reminder" },
  { value: "card", label: "Inbox kind card" },
  { value: "note", label: "Inbox kind note" },
  { value: "readlater", label: "Inbox kind readlater" },
];

export const KIND_ZH: Record<InboxKind, string> = {
  reminder: "提醒",
  card: "看板卡片",
  note: "笔记",
  readlater: "稍后读",
  none: "拿不准",
};

export const SOURCE_LABELS: Record<InboxItem["source"], string> = {
  web: "Inbox from page",
  voice: "Inbox from voice",
  telegram: "Inbox from Telegram",
  telegram_voice: "Inbox from Telegram voice",
  action: "Inbox from assistant",
};

const pad = (n: number) => String(n).padStart(2, "0");

/** RFC 3339 时间转成 datetime-local 输入框的值（本地时间，到分钟）。空或不合法返回空字符串。 */
export function toLocalInput(at: string | undefined): string {
  if (!at) return "";
  const d = new Date(at);
  if (Number.isNaN(d.getTime())) return "";
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** datetime-local 输入框的值转成 RFC 3339（带本地时区偏移）。空返回空字符串。 */
export function fromLocalInput(value: string): string {
  if (!value) return "";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return "";
  const offset = -d.getTimezoneOffset();
  const sign = offset >= 0 ? "+" : "-";
  const abs = Math.abs(offset);
  return `${value}:00${sign}${pad(Math.floor(abs / 60))}:${pad(abs % 60)}`;
}

/** 表单的初始值：先用 AI 建议，没有建议时从原文起一个标题。 */
export function formFrom(item: InboxItem): {
  kind: InboxKind;
  title: string;
  body: string;
  at: string;
  url: string;
} {
  const s = item.suggestion;
  if (s) {
    return {
      kind: s.kind === "none" ? "note" : s.kind,
      title: s.title,
      body: s.body,
      at: toLocalInput(s.at),
      url: s.url,
    };
  }
  const firstLine = item.text.split("\n")[0].trim();
  return {
    kind: "note",
    title: firstLine.slice(0, 60),
    body: item.text,
    at: "",
    url: "",
  };
}
