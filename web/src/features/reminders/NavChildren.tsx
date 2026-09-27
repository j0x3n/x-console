import NavChildLinks from "../../components/layout/NavChildLinks";
import { useLanguage } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { formatTime, relativeTime } from "../../lib/time";
import { useReminders } from "./api";

/** 侧边栏“提醒”下面：接下来要到的提醒和时间。 */
export default function RemindersNavChildren({ onNavigate }: NavChildrenProps) {
  const language = useLanguage();
  const today = useReminders("today");
  const upcoming = useReminders("upcoming");
  // 今天的和以后的合在一起，按时间排。
  const seen = new Set<number>();
  const list = [...(today.data ?? []), ...(upcoming.data ?? [])]
    .filter((r) => !seen.has(r.id) && seen.add(r.id))
    .filter((r) => r.nextAt)
    .sort((a, b) => a.nextAt!.localeCompare(b.nextAt!));
  const when = (at: string) => {
    const d = new Date(at);
    const today = new Date();
    return d.toDateString() === today.toDateString()
      ? formatTime(at, language)
      : relativeTime(at, language);
  };
  return (
    <NavChildLinks
      links={list.map((r) => ({
        key: r.id,
        to: "/reminders",
        label: r.title,
        hint: when(r.nextAt!),
      }))}
      allTo="/reminders"
      loading={today.isPending || upcoming.isPending}
      error={today.isError && upcoming.isError}
      empty="没有要到的提醒"
      onNavigate={onNavigate}
    />
  );
}
