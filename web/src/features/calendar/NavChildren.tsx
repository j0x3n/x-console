import NavChildLinks from "../../components/layout/NavChildLinks";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";

/** 侧边栏“日程”下面：三个标签页。早报在今日页右上角（B66）。 */
export default function CalendarNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  return (
    <NavChildLinks
      links={[
        { key: "s", to: "/calendar", label: t("Calendar") },
        { key: "f", to: "/calendar/focus", label: t("Focus") },
        { key: "c", to: "/calendar/calendars", label: t("Calendars") },
      ]}
      allTo="/calendar"
      empty=""
      onNavigate={onNavigate}
    />
  );
}
