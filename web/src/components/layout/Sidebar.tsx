import { useEffect, useRef, useState } from "react";
import { NavLink, useNavigate } from "react-router";
import {
  ChevronRight,
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
import { useNavChildren } from "../../lib/navChildren";

// 二级菜单展开了哪些，记在 localStorage。
const OPEN_KEY = "xc.nav.open";
function readOpen(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(OPEN_KEY) ?? "[]");
    return Array.isArray(v) ? v : [];
  } catch {
    return [];
  }
}

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
  const children = useNavChildren();
  const [open, setOpen] = useState<string[]>(readOpen);
  const toggle = (path: string) =>
    setOpen((prev) => {
      const next = prev.includes(path)
        ? prev.filter((p) => p !== path)
        : [...prev, path];
      try {
        localStorage.setItem(OPEN_KEY, JSON.stringify(next));
      } catch {
        /* 记不住就算了 */
      }
      return next;
    });
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
          {/* 点 Logo 发出事件。隐藏内容模块（B13）数连续点击次数。 */}
          <span
            className="brand-mark"
            onClick={() => window.dispatchEvent(new Event("xc:brand-tap"))}
          >
            X
          </span>
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
              {items.map((item) => {
                const Children = children[item.path];
                const isOpen = !!Children && open.includes(item.path);
                return (
                  <div key={item.path} className="nav-entry">
                    <NavLink
                      to={item.path}
                      end={item.path === "/"}
                      onClick={() => setMobileOpen(false)}
                      className={({ isActive }) =>
                        `nav-item ${isActive ? "selected" : ""}${Children ? " has-children" : ""}`
                      }
                    >
                      <item.icon size={17} strokeWidth={1.5} />
                      <span>{t(item.label)}</span>
                    </NavLink>
                    {Children && (
                      <button
                        type="button"
                        className={`nav-toggle${isOpen ? " open" : ""}`}
                        aria-expanded={isOpen}
                        aria-label={`${isOpen ? t("Collapse menu") : t("Expand menu")} ${t(item.label)}`}
                        onClick={() => toggle(item.path)}
                      >
                        <ChevronRight size={14} />
                      </button>
                    )}
                    {isOpen && (
                      <div className="nav-children">
                        <Children onNavigate={() => setMobileOpen(false)} />
                      </div>
                    )}
                  </div>
                );
              })}
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
  const navigate = useNavigate();
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
              onClick={() => {
                setOpen(false);
                navigate("/settings");
              }}
            >
              <Settings2 size={15} />
              <span>{t("Settings")}</span>
            </button>
          </div>
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
