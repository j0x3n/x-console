import { useEffect, useRef } from "react";
import { NavLink, Navigate, useParams } from "react-router";
import PageHeading from "../../components/ui/PageHeading";
import { useT } from "../../contexts/LanguageContext";
import { settingsTabs } from "./tabs";

export default function SettingsPage() {
  const t = useT();
  const { tab } = useParams();
  const current = settingsTabs.find((item) => item.id === tab);
  const navRef = useRef<HTMLElement>(null);
  // 窄屏时标签横向滚动，让当前标签露出来。
  useEffect(() => {
    navRef.current
      ?.querySelector(".active")
      ?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
  }, [tab]);
  // B62：GitHub 设置并进了 Git 与 GitHub，旧地址跳过去
  if (tab === "github") return <Navigate to="/settings/git" replace />;
  if (!current)
    return <Navigate to={`/settings/${settingsTabs[0].id}`} replace />;
  const Component = current.component;
  return (
    <div className="xc-page">
      <PageHeading
        title={t("Settings")}
        subtitle={t("Security, devices and integrations")}
      />
      <nav
        ref={navRef}
        className="xc-tabs settings-tabs"
        aria-label={t("Settings")}
      >
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
      <Component />
    </div>
  );
}
