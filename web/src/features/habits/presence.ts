import type { HabitInput, HabitTemplate, HostPresence } from "./api";

/** 电脑使用状态的一句话（B83）：正在使用 / 空闲 12 分钟 / 已锁屏 / 离线 / 无法判断。 */
export function presenceText(
  p: Pick<HostPresence, "state" | "idleSeconds" | "online">,
  t: (s: string) => string,
): string {
  switch (p.state) {
    case "active":
      return t("In use");
    case "idle": {
      const minutes = Math.max(1, Math.round((p.idleSeconds ?? 0) / 60));
      return t("Idle for {n} min").replace("{n}", String(minutes));
    }
    case "locked":
      return t("Screen locked");
    case "offline":
      return t("Offline");
    default:
      return p.online ? t("Can't tell, counted as in use") : t("Can't tell");
  }
}

export function presenceTone(state: HostPresence["state"]) {
  return state === "active" ? "ok" : state === "idle" ? "warn" : "";
}

/** 新建习惯的模板（B83）。点了自动填好，服务端也按 template 补默认值。 */
export const HABIT_TEMPLATES: {
  id: HabitTemplate;
  label: string;
  input: Omit<HabitInput, "activeHostIds">;
}[] = [
  {
    id: "water",
    label: "Drink water",
    input: {
      name: "喝水",
      icon: "💧",
      unit: "杯",
      dailyTarget: 8,
      remindMode: "interval",
      remindIntervalMinutes: 60,
      remindWhen: ["awake"],
    },
  },
  {
    id: "eyes",
    label: "Rest eyes",
    input: {
      name: "护眼",
      icon: "👀",
      unit: "次",
      dailyTarget: 1,
      remindMode: "interval",
      remindIntervalMinutes: 20,
      remindWhen: ["active"],
    },
  },
  {
    id: "move",
    label: "Get up and move",
    input: {
      name: "起来活动",
      icon: "🚶",
      unit: "次",
      dailyTarget: 1,
      remindMode: "interval",
      remindIntervalMinutes: 45,
      remindWhen: ["active"],
    },
  },
  {
    id: "medicine",
    label: "Take medicine",
    input: {
      name: "吃药",
      icon: "💊",
      unit: "次",
      dailyTarget: 2,
      remindMode: "times",
      remindTimes: ["09:00", "21:00"],
      remindWhen: ["window"],
    },
  },
];

/** 睡觉时间早于起床时间就是跨到第二天。 */
export function crossesMidnight(wake: string, sleep: string): boolean {
  return sleep < wake;
}
