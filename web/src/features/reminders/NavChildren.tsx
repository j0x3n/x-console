import { useLocation } from "react-router";
import { CalendarClock, CircleCheck, Sun } from "lucide-react";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useReminderCounts } from "./api";

/** 左栏“提醒”的二级菜单（B102）：今天、即将到来、已完成，和提醒页的标签一致。 */
export default function RemindersNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const counts = useReminderCounts();
  const location = useLocation();
  const tab =
    location.pathname === "/reminders"
      ? (new URLSearchParams(location.search).get("tab") ?? "today")
      : null;
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/reminders"
          icon={Sun}
          label={t("Today")}
          count={counts.loading ? null : counts.today}
          active={tab === "today"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/reminders?tab=upcoming"
          icon={CalendarClock}
          label={t("Upcoming")}
          count={counts.loading ? null : counts.upcoming}
          active={tab === "upcoming"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/reminders?tab=done"
          icon={CircleCheck}
          label={t("Done")}
          active={tab === "done"}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
    </NavPanelStack>
  );
}
