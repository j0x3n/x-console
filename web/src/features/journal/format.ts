import {
  Bell,
  BookOpen,
  CalendarDays,
  CheckSquare,
  Dumbbell,
  GitCommitHorizontal,
  GitPullRequest,
  HeartPulse,
  Bookmark,
  Monitor,
  NotebookPen,
  Timer,
  Bot,
  type LucideIcon,
} from "lucide-react";
import type { JournalCount, JournalItem, JournalKind } from "./api";

/** 类型的图标和文字（英文键，中文在 i18n.ts）。 */
export const KIND_META: Record<
  JournalKind,
  { icon: LucideIcon; label: string }
> = {
  commit: { icon: GitCommitHorizontal, label: "Commit" },
  pr: { icon: GitPullRequest, label: "Pull request" },
  card: { icon: CheckSquare, label: "Card" },
  task: { icon: Bot, label: "Agent task" },
  focus: { icon: Timer, label: "Focus" },
  habit: { icon: HeartPulse, label: "Habit" },
  workout: { icon: Dumbbell, label: "Workout record" },
  event: { icon: CalendarDays, label: "Event" },
  alert: { icon: Bell, label: "Server alert" },
  screen: { icon: Monitor, label: "Computer time" },
  link: { icon: Bookmark, label: "Saved link" },
  note: { icon: NotebookPen, label: "Note" },
};

export const DIARY_META = { icon: BookOpen, label: "Diary" };

export function kindMeta(kind: string): { icon: LucideIcon; label: string } {
  if (kind === "diary") return DIARY_META;
  return KIND_META[kind as JournalKind] ?? DIARY_META;
}

function sum(counts: JournalCount[], kinds: JournalKind[]) {
  return counts
    .filter((c) => kinds.includes(c.kind))
    .reduce((n, c) => n + c.count, 0);
}

/** 概要卡片上的四个数。 */
export function summaryNumbers(counts: JournalCount[]) {
  return {
    code: sum(counts, ["commit", "pr"]),
    done: sum(counts, ["card", "task"]),
    focusMinutes: counts
      .filter((c) => c.kind === "focus")
      .reduce((n, c) => n + c.minutes, 0),
    habits: sum(counts, ["habit", "workout"]),
  };
}

/** 14:05，没有时间的整天事项也显示 00:00。 */
export function timeLabel(item: JournalItem): string {
  const d = new Date(item.at);
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

/** 在日期上加减天数，日期是 YYYY-MM-DD。 */
export function shiftDay(day: string, delta: number): string {
  const d = new Date(`${day}T00:00:00`);
  d.setDate(d.getDate() + delta);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

export function dayLabel(day: string, language: string): string {
  return new Date(`${day}T00:00:00`).toLocaleDateString(
    language === "zh" ? "zh-CN" : "en",
    { year: "numeric", month: "long", day: "numeric", weekday: "long" },
  );
}

/** 日期条上的短标签：10/10 周六。 */
export function chipLabel(day: string, language: string): [string, string] {
  const d = new Date(`${day}T00:00:00`);
  return [
    `${d.getMonth() + 1}/${d.getDate()}`,
    d.toLocaleDateString(language === "zh" ? "zh-CN" : "en", {
      weekday: "short",
    }),
  ];
}

export function isInternal(link: string): boolean {
  return link.startsWith("/");
}
