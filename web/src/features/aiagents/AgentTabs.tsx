import { Link, NavLink } from "react-router";
import { useT } from "../../contexts/LanguageContext";
import "./i18n";

const TABS = [
  { to: "/coding", label: "Agents", end: true },
  { to: "/coding/tasks", label: "Tasks", end: false },
  { to: "/coding/repos", label: "Repositories", end: false },
];

/** Agent 管理页的三个页签（B47）。放在各页 Toolbar 的 start。Git 账号在设置里（B62）。 */
export default function AgentTabs() {
  const t = useT();
  return (
    <nav className="xc-tabs" aria-label={t("Agents")}>
      {TABS.map((tab) => (
        <NavLink
          key={tab.to}
          to={tab.to}
          end={tab.end}
          className={({ isActive }) => (isActive ? "active" : undefined)}
        >
          {t(tab.label)}
        </NavLink>
      ))}
      <Link to="/settings/git" className="aiagent-tabs-link">
        {t("Git accounts are managed in settings")}
      </Link>
    </nav>
  );
}
