import { NavLink, Outlet } from "react-router";
import PageHeading from "../../components/ui/PageHeading";
import { useLanguage, useT } from "../../contexts/LanguageContext";

const tabs = [
  { to: "/calendar", label: "Schedule", end: true },
  { to: "/calendar/briefs", label: "Daily brief", end: false },
  { to: "/calendar/focus", label: "Focus", end: false },
  { to: "/calendar/calendars", label: "Calendars", end: false },
];

/** 日历模块的外框：标题和四个标签。 */
export default function CalendarShell() {
  const t = useT();
  const language = useLanguage();
  const today = new Date().toLocaleDateString(language === "zh" ? "zh-CN" : "en", {
    year: "numeric",
    month: "long",
    day: "numeric",
    weekday: "long",
  });
  return (
    <div className="xc-page calendar-page">
      <PageHeading title={t("Calendar")} subtitle={today} />
      <nav className="xc-tabs">
        {tabs.map((tab) => (
          <NavLink
            key={tab.to}
            to={tab.to}
            end={tab.end}
            className={({ isActive }) => (isActive ? "active" : "")}
          >
            {t(tab.label)}
          </NavLink>
        ))}
      </nav>
      <Outlet />
    </div>
  );
}
