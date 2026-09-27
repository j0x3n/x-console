import { NavLink, Navigate, useParams } from "react-router";
import PageHeading from "../../components/ui/PageHeading";
import { useT } from "../../contexts/LanguageContext";
import { settingsTabs } from "./tabs";

export default function SettingsPage() {
  const t = useT();
  const { tab } = useParams();
  const current = settingsTabs.find((item) => item.id === tab);
  if (!current)
    return <Navigate to={`/settings/${settingsTabs[0].id}`} replace />;
  const Component = current.component;
  return (
    <div className="xc-page">
      <PageHeading
        title={t("Settings")}
        subtitle={t("Appearance, security, devices and integrations")}
      />
      <div className="settings-layout">
        <nav className="settings-nav" aria-label={t("Settings")}>
          {settingsTabs.map((item) => (
            <NavLink
              key={item.id}
              to={`/settings/${item.id}`}
              className={({ isActive }) => (isActive ? "active" : "")}
            >
              {t(item.label)}
            </NavLink>
          ))}
        </nav>
        <div className="settings-body">
          <Component />
        </div>
      </div>
    </div>
  );
}
