import type { MeDay, MeDuration, MeState, MeStateName } from "./api";

/* 我的状态的纯逻辑：名称、颜色、手动设定的选项、几点开始的写法。 */

type T = (key: string) => string;

export const STATE_NAMES: MeStateName[] = [
  "working",
  "away",
  "sleeping",
  "out",
  "home",
  "unknown",
];

/** 手动设定能选的状态。“不确定”不是一个可以选的状态。 */
export const MANUAL_STATES: MeStateName[] = [
  "working",
  "away",
  "sleeping",
  "out",
  "home",
];

/** 英文原文，界面里用 t()。 */
export const STATE_LABELS: Record<MeStateName, string> = {
  working: "At work",
  away: "Stepped away",
  sleeping: "Asleep",
  out: "Out of home",
  home: "At home",
  unknown: "Not sure",
};

export const STATE_TONES: Record<MeStateName, string> = {
  working: "accent",
  away: "warn",
  sleeping: "info",
  out: "warn",
  home: "ok",
  unknown: "",
};

export const DURATIONS: { id: MeDuration; label: string }[] = [
  { id: "1h", label: "1 hour" },
  { id: "2h", label: "2 hours" },
  { id: "wake", label: "Until the next wake-up time" },
  { id: "forever", label: "Until I change it back" },
];

/** 判断依据的说明，鼠标停上去时显示。 */
export const SOURCE_LABELS: Record<string, string> = {
  pc: "Judged from your computer",
  phone: "Reported by your phone",
  ha: "Reported by Home Assistant",
  manual: "Set by hand",
  schedule: "Guessed from your daily schedule",
};

function sameDay(a: Date, b: Date) {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

/** 几点开始：今天只写时间，昨天加“昨天”，更早的加日期。 */
export function sinceText(
  t: T,
  iso: string,
  now: Date,
  time: (d: Date) => string,
  date: (d: Date) => string,
): string {
  const at = new Date(iso);
  if (sameDay(at, now)) return time(at);
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (sameDay(at, yesterday)) return `${t("Yesterday")} ${time(at)}`;
  return `${date(at)} ${time(at)}`;
}

/** 手动设定的标记：“手动”，有到期时间时加上。 */
export function manualBadge(
  t: T,
  state: MeState,
  now: Date,
  time: (d: Date) => string,
  date: (d: Date) => string,
): string {
  if (!state.manual) return "";
  if (!state.manualUntil) return t("Manual");
  return `${t("Manual")} · ${t("until")} ${sinceText(t, state.manualUntil, now, time, date)}`;
}

/** 一天的睡觉时间：“03:40 睡，11:55 起，8 小时 15 分钟”。没有时返回空。 */
export function sleepLine(t: T, day: MeDay, time: (d: Date) => string): string {
  if (!day.sleptAt || !day.wokeAt || day.sleepMinutes == null) return "";
  const h = Math.floor(day.sleepMinutes / 60);
  const m = day.sleepMinutes % 60;
  const length =
    h === 0
      ? `${m} ${t("min")}`
      : m === 0
        ? `${h} ${t("hr")}`
        : `${h} ${t("hr")} ${m} ${t("min")}`;
  return `${time(new Date(day.sleptAt))} → ${time(new Date(day.wokeAt))} · ${length}`;
}

/** 在家实体填的是否像 person.xxx 或 device_tracker.xxx。空表示不用。 */
export function validHomeEntity(v: string): boolean {
  return (
    v.trim() === "" || /^(person|device_tracker)\.[a-z0-9_]+$/.test(v.trim())
  );
}

/** 睡眠专注实体填的是否像 binary_sensor.xxx 或 input_boolean.xxx。空表示不用。 */
export function validSleepEntity(v: string): boolean {
  return (
    v.trim() === "" ||
    /^(binary_sensor|input_boolean)\.[a-z0-9_]+$/.test(v.trim())
  );
}
