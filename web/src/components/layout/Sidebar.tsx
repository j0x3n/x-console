import { useEffect, useRef, useState } from "react";
import { NavLink } from "react-router";
import {
  LogOut,
  Moon,
  PanelLeftClose,
  Search,
  Settings2,
  Sun,
  Languages,
} from "lucide-react";
import { navGroupLabels, navItems, type NavGroup } from "../../app/nav";
import { useAuthStatus, useLogout } from "../../api/core";
import { useT } from "../../contexts/LanguageContext";
import { usePreferencesStore } from "../../stores/preferences-store";

interface SidebarProps {
  mobileOpen: boolean;
  setMobileOpen: (open: boolean) => void;
  openPalette: () => void;
}

export default function Sidebar({
  mobileOpen,
  setMobileOpen,
  openPalette,
}: SidebarProps) {
  const t = useT();
  const groups = (Object.keys(navGroupLabels) as NavGroup[]).map((group) => ({
    group,
    items: navItems.filter((item) => item.group === group),
  }));
  return (
    <>
      {mobileOpen && (
        <button
          className="mobile-scrim"
          aria-label={t("Close navigation")}
          onClick={() => setMobileOpen(false)}
        />
      )}
      <nav
        className={`sidebar ${mobileOpen ? "open" : ""}`}
        aria-label={t("Main")}
      >
        <div className="workspace-switch">
          <span className="brand-mark">X</span>
          <span>X Console</span>
          <button
            className="icon-button sidebar-collapse"
            title={t("Close navigation")}
            onClick={() => setMobileOpen(false)}
          >
            <PanelLeftClose size={15} />
          </button>
        </div>
        <button className="sidebar-search" onClick={openPalette}>
          <Search size={15} />
          <span>{t("Search or run a command...")}</span>
          <kbd>⌘K</kbd>
        </button>
        <div className="nav-groups">
          {groups.map(({ group, items }) => (
            <div key={group}>
              {navGroupLabels[group] && (
                <div className="side-label">{t(navGroupLabels[group])}</div>
              )}
              {items.map((item) => (
                <NavLink
                  key={item.path}
                  to={item.path}
                  end={item.path === "/"}
                  onClick={() => setMobileOpen(false)}
                  className={({ isActive }) =>
                    `nav-item ${isActive ? "selected" : ""}`
                  }
                >
                  <item.icon size={17} strokeWidth={1.5} />
                  <span>{t(item.label)}</span>
                </NavLink>
              ))}
            </div>
          ))}
        </div>
        <ProfileMenu />
      </nav>
    </>
  );
}

function ProfileMenu() {
  const t = useT();
  const auth = useAuthStatus();
  const logout = useLogout();
  const { language, setLanguage, themeMode, setThemeMode } =
    usePreferencesStore();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onDown = (event: PointerEvent) => {
      if (!ref.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open]);
  const username = auth.data?.username ?? "";
  return (
    <div className="profile-menu-wrap" ref={ref}>
      {open && (
        <div className="profile-popover" role="menu">
          <div className="profile-menu-section">
            <button
              className="profile-menu-item"
              role="menuitem"
              onClick={() =>
                setThemeMode(
                  themeMode === "dark"
                    ? "light"
                    : themeMode === "light"
                      ? "system"
                      : "dark",
                )
              }
            >
              {themeMode === "light" ? <Sun size={15} /> : <Moon size={15} />}
              <span>{t("Theme")}</span>
              <small>
                {t(
                  themeMode === "system"
                    ? "Follow system"
                    : themeMode === "dark"
                      ? "Dark"
                      : "Light",
                )}
              </small>
            </button>
            <button
              className="profile-menu-item"
              role="menuitem"
              onClick={() => setLanguage(language === "zh" ? "en" : "zh")}
            >
              <Languages size={15} />
              <span>{t("Language")}</span>
              <small>{language === "zh" ? "中文" : "English"}</small>
            </button>
          </div>
          <div className="profile-menu-section">
            <button
              className="profile-menu-item"
              role="menuitem"
              onClick={() => logout.mutate()}
            >
              <LogOut size={15} />
              <span>{t("Sign out")}</span>
            </button>
          </div>
        </div>
      )}
      <button
        className="profile"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <span className="profile-avatar">
          {username.slice(0, 2).toUpperCase()}
        </span>
        <span>
          <b>{username}</b>
          <small>X Console</small>
        </span>
        <Settings2 size={15} />
      </button>
    </div>
  );
}
