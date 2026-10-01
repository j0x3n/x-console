import { useEffect, useRef, useState } from "react";
import { NavLink, useLocation, useNavigate } from "react-router";
import {
  ChevronRight,
  LogOut,
  Moon,
  Palette,
  PanelLeftClose,
  Search,
  Settings2,
  Languages,
} from "lucide-react";
import { navGroupLabels, navItems, type NavGroup } from "../../app/nav";
import { moduleOfPath, useModules } from "../../app/modules";
import { useAuthStatus, useLogout } from "../../api/core";
import { useT } from "../../contexts/LanguageContext";
import { accents, usePreferencesStore } from "../../stores/preferences-store";
import type { Accent } from "../../types/domain";
import {
  InstallHelpDialog,
  InstallMenuItem,
} from "../../features/pwa/InstallMenu";
import { useNavChildren } from "../../lib/navChildren";
import { useSidebar } from "../../stores/sidebar";

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
  const collapsed = useSidebar((s) => s.collapsed);
  const [open, setOpen] = useState<string[]>(readOpen);
  const location = useLocation();
  const atRoot = (path: string) => location.pathname === path;
  // 点一级菜单就展开它的二级菜单；只有点箭头才会收起。
  const setItemOpen = (path: string, value: boolean) =>
    setOpen((prev) => {
      if (prev.includes(path) === value) return prev;
      const next = value ? [...prev, path] : prev.filter((p) => p !== path);
      try {
        localStorage.setItem(OPEN_KEY, JSON.stringify(next));
      } catch {
        /* 记不住就算了 */
      }
      return next;
    });
  // B57：锁定时被隐藏的模块不出现，分组空了整组不显示
  const modules = useModules();
  const groups = (Object.keys(navGroupLabels) as NavGroup[])
    .map((group) => ({
      group,
      items: navItems.filter(
        (item) => item.group === group && modules.has(moduleOfPath(item.path)),
      ),
    }))
    .filter((g) => g.items.length > 0);
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
        className={`sidebar${mobileOpen ? " open" : ""}${collapsed ? " collapsed" : ""}`}
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
          {/* 隐藏内容解锁时，VaultPanel 把锁定按钮放到这里 */}
          <span id="brand-slot" className="brand-slot" />
          <button
            className="icon-button sidebar-collapse"
            title={t("Close navigation")}
            onClick={() => setMobileOpen(false)}
          >
            <PanelLeftClose size={15} />
          </button>
        </div>
        <button
          className="sidebar-search"
          onClick={openPalette}
          title={collapsed ? t("Search or run a command...") : undefined}
        >
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
                      title={collapsed ? t(item.label) : undefined}
                      onClick={() => {
                        // 从别的页面点过来时展开；已经在这一页、已经展开时再点一下收起。
                        if (Children)
                          setItemOpen(
                            item.path,
                            !(isOpen && atRoot(item.path)),
                          );
                        setMobileOpen(false);
                      }}
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
                        onClick={() => setItemOpen(item.path, !isOpen)}
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

const accentLabels: Record<Accent, string> = {
  ember: "Ember",
  violet: "Violet",
  mint: "Mint",
  ocean: "Ocean",
  rose: "Rose",
  graphite: "Graphite",
};

function ProfileMenu() {
  const t = useT();
  const navigate = useNavigate();
  const auth = useAuthStatus();
  const logout = useLogout();
  const { language, setLanguage, themeMode, setThemeMode, accent, setAccent } =
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
      <InstallHelpDialog />
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
            <div className="profile-menu-row">
              <Moon size={15} />
              <span>{t("Night mode")}</span>
            </div>
            <div
              className="profile-segmented"
              role="radiogroup"
              aria-label={t("Night mode")}
            >
              {(
                [
                  ["dark", "On"],
                  ["light", "Off"],
                  ["system", "Auto"],
                ] as const
              ).map(([mode, label]) => (
                <button
                  key={mode}
                  role="radio"
                  aria-checked={themeMode === mode}
                  className={themeMode === mode ? "active" : ""}
                  onClick={() => setThemeMode(mode)}
                >
                  {t(label)}
                </button>
              ))}
            </div>
            <div className="profile-menu-row">
              <Palette size={15} />
              <span>{t("Theme color")}</span>
            </div>
            <div
              className="profile-accents"
              role="radiogroup"
              aria-label={t("Theme color")}
            >
              {accents.map((a) => (
                <button
                  key={a}
                  role="radio"
                  aria-checked={accent === a}
                  aria-label={t(accentLabels[a])}
                  title={t(accentLabels[a])}
                  className={`profile-accent accent-${a}${accent === a ? " active" : ""}`}
                  onClick={() => setAccent(a)}
                />
              ))}
            </div>
            {themeMode === "dark" && (
              <small className="profile-menu-hint">
                {t("Theme colors apply in the daytime.")}
              </small>
            )}
          </div>
          <div className="profile-menu-section">
            <button
              className="profile-menu-item"
              role="menuitem"
              onClick={() => setLanguage(language === "zh" ? "en" : "zh")}
            >
              <Languages size={15} />
              <span>{t("Language")}</span>
              <small>{language === "zh" ? "中文" : "English"}</small>
            </button>
            <InstallMenuItem onDone={() => setOpen(false)} />
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
