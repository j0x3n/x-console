import { useLocation } from "react-router";
import { CalendarDays, CalendarCog, Timer } from "lucide-react";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";

/** 左栏“日程”的二级菜单（B102）：日历、专注、日历管理。早报在今日页右上角（B66）。 */
export default function CalendarNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const { pathname } = useLocation();
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/calendar"
          icon={CalendarDays}
          label={t("Calendar")}
          active={pathname === "/calendar"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/calendar/focus"
          icon={Timer}
          label={t("Focus")}
          active={pathname.startsWith("/calendar/focus")}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/calendar/calendars"
          icon={CalendarCog}
          label={t("Calendars")}
          active={pathname.startsWith("/calendar/calendars")}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
    </NavPanelStack>
  );
}
