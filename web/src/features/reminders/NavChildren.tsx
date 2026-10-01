import { useLocation } from "react-router";
import NavChildLinks from "../../components/layout/NavChildLinks";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useReminderCounts } from "./api";

/**
 * 侧边栏“提醒”下面：今天几条、即将到来几条，和提醒页的标签一致。
 * 今天是 0 时不显示那一行。
 */
export default function RemindersNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const counts = useReminderCounts();
  const location = useLocation();
  const links = [
    ...(counts.today > 0
      ? [
          {
            key: "today",
            to: "/reminders",
            label: t("Today"),
            hint: String(counts.today),
            active:
              location.pathname === "/reminders" &&
              !location.search.includes("tab="),
          },
        ]
      : []),
    {
      key: "upcoming",
      to: "/reminders?tab=upcoming",
      label: t("Upcoming"),
      hint: String(counts.upcoming),
      active: location.search.includes("tab=upcoming"),
    },
  ];
  return (
    <NavChildLinks
      links={links}
      allTo="/reminders"
      loading={counts.loading}
      error={counts.error}
      empty={t("Nothing coming up")}
      onNavigate={onNavigate}
    />
  );
}
