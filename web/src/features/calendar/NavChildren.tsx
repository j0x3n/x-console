import NavChildLinks from "../../components/layout/NavChildLinks";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";

/** 侧边栏“日历”下面：四个标签页。 */
export default function CalendarNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  return (
    <NavChildLinks
      links={[
        { key: "s", to: "/calendar", label: t("Schedule") },
        { key: "b", to: "/calendar/briefs", label: t("Daily brief") },
        { key: "f", to: "/calendar/focus", label: t("Focus") },
        { key: "c", to: "/calendar/calendars", label: t("Calendars") },
      ]}
      allTo="/calendar"
      empty=""
      onNavigate={onNavigate}
    />
  );
}
